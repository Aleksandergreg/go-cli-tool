package sandbox

import (
	"fmt"
	"slices"
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
		return "", withExitStatus(statusCommandNotFound, fmt.Errorf("%s: no such file or directory", args[0]))
	}
	if entry.Kind != Regular {
		return "", withExitStatus(statusCannotExecute, fmt.Errorf("%s: is a directory", args[0]))
	}
	if entry.Mode&0o111 == 0 {
		return "", withExitStatus(statusCannotExecute, fmt.Errorf("%s: permission denied; set an executable mode with chmod", args[0]))
	}
	return s.executeScript(context, resolved, true)
}

func (s *Sandbox) executeScript(context *executionContext, scriptPath string, requireShebang bool) (string, error) {
	// As in sh, a missing script exits 127 and one that cannot run exits 126.
	entry, exists := s.FS.Entry(scriptPath)
	if !exists {
		return "", withExitStatus(statusCommandNotFound, fmt.Errorf("%s: no such file", scriptPath))
	}
	if entry.Kind != Regular {
		return "", withExitStatus(statusCannotExecute, fmt.Errorf("%s: is a directory", scriptPath))
	}
	content := entry.Content
	if len(content) > maxScriptBytes {
		return "", fmt.Errorf("%s: script exceeds the %d KiB limit", scriptPath, maxScriptBytes/1024)
	}
	if !utf8.ValidString(content) || strings.ContainsRune(content, '\x00') {
		return "", fmt.Errorf("%s: scripts must be UTF-8 text without NUL bytes", scriptPath)
	}
	lines := strings.Split(content, "\n")
	if requireShebang {
		firstLine := strings.TrimSuffix(lines[0], "\r")
		if firstLine != "#!/bin/sh" && firstLine != "#!/usr/bin/env sh" {
			return "", withExitStatus(statusCannotExecute, fmt.Errorf("%s: executable scripts require #!/bin/sh or #!/usr/bin/env sh", scriptPath))
		}
	}
	if len(context.scriptStack) >= maxScriptDepth {
		return "", fmt.Errorf("script nesting limit of %d exceeded", maxScriptDepth)
	}
	if slices.Contains(context.scriptStack, scriptPath) {
		return "", fmt.Errorf("recursive script invocation of %s is not supported", scriptPath)
	}

	savedCWD := s.CWD
	savedEnv := cloneEnvironment(s.Env)
	context.scriptStack = append(context.scriptStack, scriptPath)
	defer func() {
		context.scriptStack = context.scriptStack[:len(context.scriptStack)-1]
		s.CWD = savedCWD
		s.Env = savedEnv
	}()

	// A script runs in a new shell, where $? starts at 0.
	s.lastStatus = 0
	var output strings.Builder
	statusFailed := false
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
		stderrBefore := len(context.stderr)
		result, err := s.executeLine(line, context, false)
		// Failures that a command list handled (`a || b`), on this line or in a
		// nested script, still reach the caller's terminal ahead of the
		// script's output, located like the errors that stop a script.
		diagnostics := slices.Clone(context.stderr[stderrBefore:])
		context.stderr = context.stderr[:stderrBefore]
		for _, chunk := range result.Transcript {
			if chunk.Error {
				diagnostics = append(diagnostics, chunk.Text)
			}
		}
		for _, message := range diagnostics {
			context.stderr = append(context.stderr, fmt.Sprintf("%s:%d: %s", scriptPath, lineNumber, message))
		}
		if err != nil {
			return "", fmt.Errorf("%s:%d: %w", scriptPath, lineNumber, err)
		}
		if output.Len()+len(result.Output) > maxScriptOutputBytes {
			return "", fmt.Errorf("%s:%d: script output exceeds the %d KiB limit", scriptPath, lineNumber, maxScriptOutputBytes/1024)
		}
		output.WriteString(result.Output)
		statusFailed = result.statusFailed
	}
	if statusFailed {
		// Like sh, a script's exit status is that of its last command.
		return output.String(), errFailureStatus
	}
	return output.String(), nil
}

func validateScriptCommands(parsed commandLine) error {
	for _, stage := range parsed.stages {
		first := stage.args[0].value
		if _, unsupported := unsupportedScriptKeywords[first]; unsupported {
			return fmt.Errorf("shell language keyword %q is not supported", first)
		}
		if name, _, found := strings.Cut(first, "="); found && validScriptVariableName(name) {
			return fmt.Errorf("standalone assignments are not supported; use export NAME=value")
		}
	}
	return nil
}

var unsupportedScriptKeywords = map[string]struct{}{
	"if": {}, "then": {}, "elif": {}, "else": {}, "fi": {},
	"for": {}, "while": {}, "until": {}, "do": {}, "done": {},
	"case": {}, "esac": {}, "select": {}, "function": {},
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
