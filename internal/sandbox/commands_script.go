package sandbox

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxScriptBytes       = 64 * 1024
	maxScriptLineBytes   = 8 * 1024
	maxScriptDepth       = 8
	maxScriptSteps       = 256
	maxScriptOutputBytes = 1024 * 1024
)

func isScriptInvocation(name string) bool {
	return name == "sh" || isExecutableScriptPath(name)
}

func isExecutableScriptPath(name string) bool {
	return strings.Contains(name, "/")
}

func (s *Sandbox) cmdSh(context *executionContext, args []string, stdin string) (string, error) {
	if stdin != "" {
		return "", fmt.Errorf("script input from pipelines or redirection is not supported")
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			return "", fmt.Errorf("options such as %s are not supported; usage: sh FILE", arg)
		}
	}
	if len(args) != 1 || args[0] == "" {
		return "", fmt.Errorf("usage: sh FILE; positional arguments are not supported")
	}
	return s.executeScript(context, s.Resolve(args[0]), false)
}

func (s *Sandbox) cmdExecutableScript(context *executionContext, args []string, stdin string) (string, error) {
	if stdin != "" {
		return "", fmt.Errorf("script input from pipelines or redirection is not supported")
	}
	if len(args) != 1 {
		return "", fmt.Errorf("positional arguments are not supported; run sh FILE without arguments")
	}
	resolved := s.Resolve(args[0])
	entry, exists := s.FS.Entry(resolved)
	if !exists {
		return "", fmt.Errorf("%s: no such file or directory", args[0])
	}
	if entry.Kind != Regular {
		return "", fmt.Errorf("%s: is a directory", args[0])
	}
	if entry.Mode&0o111 == 0 {
		return "", fmt.Errorf("%s: permission denied; set an executable mode with chmod", args[0])
	}
	return s.executeScript(context, resolved, true)
}

func (s *Sandbox) executeScript(context *executionContext, scriptPath string, requireShebang bool) (string, error) {
	content, err := s.FS.ReadFile(scriptPath)
	if err != nil {
		return "", err
	}
	if len(content) > maxScriptBytes {
		return "", fmt.Errorf("%s: script exceeds the %d KiB limit", scriptPath, maxScriptBytes/1024)
	}
	if !utf8.ValidString(content) || strings.ContainsRune(content, '\x00') {
		return "", fmt.Errorf("%s: scripts must be UTF-8 text without NUL bytes", scriptPath)
	}
	lines := strings.Split(content, "\n")
	if requireShebang {
		firstLine := ""
		if len(lines) > 0 {
			firstLine = strings.TrimSuffix(lines[0], "\r")
		}
		if firstLine != "#!/bin/sh" && firstLine != "#!/usr/bin/env sh" {
			return "", fmt.Errorf("%s: executable scripts require #!/bin/sh or #!/usr/bin/env sh", scriptPath)
		}
	}
	if len(context.scriptStack) >= maxScriptDepth {
		return "", fmt.Errorf("script nesting limit of %d exceeded", maxScriptDepth)
	}
	for _, activePath := range context.scriptStack {
		if activePath == scriptPath {
			return "", fmt.Errorf("recursive script invocation of %s is not supported", scriptPath)
		}
	}

	savedCWD := s.CWD
	savedPrevious := s.Previous
	savedEnv := cloneScriptEnv(s.Env)
	context.scriptStack = append(context.scriptStack, scriptPath)
	defer func() {
		context.scriptStack = context.scriptStack[:len(context.scriptStack)-1]
		s.CWD = savedCWD
		s.Previous = savedPrevious
		s.Env = savedEnv
	}()

	var output strings.Builder
	for index, rawLine := range lines {
		lineNumber := index + 1
		line := strings.TrimSuffix(rawLine, "\r")
		if len(line) > maxScriptLineBytes {
			return "", fmt.Errorf("%s:%d: line exceeds the %d KiB limit", scriptPath, lineNumber, maxScriptLineBytes/1024)
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if err := validateScriptLine(line, s.Env); err != nil {
			return "", fmt.Errorf("%s:%d: %w", scriptPath, lineNumber, err)
		}
		result, err := s.executeLine(line, context, false)
		if err != nil {
			return "", fmt.Errorf("%s:%d: %w", scriptPath, lineNumber, err)
		}
		if output.Len()+len(result.Output) > maxScriptOutputBytes {
			return "", fmt.Errorf("%s:%d: script output exceeds the %d KiB limit", scriptPath, lineNumber, maxScriptOutputBytes/1024)
		}
		output.WriteString(result.Output)
	}
	return output.String(), nil
}

func cloneScriptEnv(source map[string]string) map[string]string {
	clone := make(map[string]string, len(source))
	for name, value := range source {
		clone[name] = value
	}
	return clone
}

func validateScriptLine(line string, env map[string]string) error {
	runes := []rune(line)
	var quote rune
	escaped := false
	tokenStarted := false
	for index, char := range runes {
		if escaped {
			escaped = false
			tokenStarted = true
			continue
		}
		if quote == '\'' {
			if char == quote {
				quote = 0
			}
			continue
		}
		if char == '\\' {
			escaped = true
			continue
		}
		if quote == '"' {
			if char == quote {
				quote = 0
				continue
			}
			if char == '`' || char == '$' && index+1 < len(runes) && runes[index+1] == '(' {
				return fmt.Errorf("command substitution is not supported")
			}
			if char == '$' && isUnsupportedScriptParameter(runes[index+1:]) {
				return fmt.Errorf("positional and special parameters are not supported")
			}
			continue
		}

		if unicode.IsSpace(char) {
			tokenStarted = false
			continue
		}
		if char == '#' && !tokenStarted {
			break
		}
		if char == '\'' || char == '"' {
			quote = char
			tokenStarted = true
			continue
		}
		switch char {
		case ';', '&', '(', ')':
			return fmt.Errorf("shell control syntax %q is not supported", char)
		case '`':
			return fmt.Errorf("command substitution is not supported")
		case '|':
			if index+1 < len(runes) && runes[index+1] == '|' {
				return fmt.Errorf("control operator || is not supported")
			}
		case '$':
			if index+1 < len(runes) && runes[index+1] == '(' {
				return fmt.Errorf("command substitution is not supported")
			}
			if isUnsupportedScriptParameter(runes[index+1:]) {
				return fmt.Errorf("positional and special parameters are not supported")
			}
		}
		tokenStarted = true
	}

	tokens, err := lex(line, env)
	if err != nil || len(tokens) == 0 || tokens[0].kind != wordToken {
		return nil
	}
	first := tokens[0].value
	unsupportedKeywords := map[string]bool{
		"if": true, "then": true, "elif": true, "else": true, "fi": true,
		"for": true, "while": true, "until": true, "do": true, "done": true,
		"case": true, "esac": true, "select": true, "function": true,
	}
	if unsupportedKeywords[first] {
		return fmt.Errorf("shell language keyword %q is not supported", first)
	}
	if name, _, found := strings.Cut(first, "="); found && validScriptVariableName(name) {
		return fmt.Errorf("standalone assignments are not supported; use export NAME=value")
	}
	return nil
}

func isUnsupportedScriptParameter(input []rune) bool {
	if len(input) == 0 {
		return false
	}
	if unicode.IsDigit(input[0]) || strings.ContainsRune("?$!#*@-", input[0]) {
		return true
	}
	return len(input) > 1 && input[0] == '{' && (unicode.IsDigit(input[1]) || strings.ContainsRune("?$!#*@-", input[1]))
}

func validScriptVariableName(name string) bool {
	if name == "" {
		return false
	}
	for index, char := range name {
		if index == 0 && char != '_' && !unicode.IsLetter(char) {
			return false
		}
		if index > 0 && char != '_' && !unicode.IsLetter(char) && !unicode.IsDigit(char) {
			return false
		}
	}
	return true
}
