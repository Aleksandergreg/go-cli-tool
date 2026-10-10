package sandbox

import (
	"fmt"
	"strings"
	"unicode"
)

// validateShellSyntax rejects shell language outside the teaching subset
// before one pipeline is lexed. Interactive lines and script lines share these
// rules, so syntax such as `cmd &` or `cmd 2>/dev/null` fails with a clear
// message instead of silently becoming literal arguments. Command-list
// operators (;, &&, ||) are split off earlier by splitCommandList.
func validateShellSyntax(line string) error {
	runes := []rune(line)
	var quote rune
	escaped := false
	tokenStarted := false
	tokenStart := 0
	digitsOnly := false
	for index, char := range runes {
		if escaped {
			escaped = false
			tokenStarted = true
			digitsOnly = false
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
			if char == '$' && isUnsupportedShellParameter(runes[index+1:]) {
				return unsupportedParameterError(runes[index+1:])
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
			digitsOnly = false
			continue
		}
		next := rune(0)
		if index+1 < len(runes) {
			next = runes[index+1]
		}
		switch char {
		case '&':
			if index > 0 && (runes[index-1] == '>' || runes[index-1] == '<') {
				return fdRedirectionError(string(runes[index-1 : index+1]))
			}
			if next == '>' {
				return fdRedirectionError("&>")
			}
			return fmt.Errorf("shell control syntax %q is not supported; background jobs are not available", "&")
		case '(', ')':
			return fmt.Errorf("shell control syntax %q is not supported", string(char))
		case '`':
			return fmt.Errorf("command substitution is not supported")
		case '|':
			tokenStarted = false
			continue
		case '<', '>':
			if tokenStarted && digitsOnly {
				return fdRedirectionError(string(runes[tokenStart : index+1]))
			}
			tokenStarted = false
			continue
		case '$':
			if next == '(' {
				return fmt.Errorf("command substitution is not supported")
			}
			if isUnsupportedShellParameter(runes[index+1:]) {
				return unsupportedParameterError(runes[index+1:])
			}
		}
		isDigit := char >= '0' && char <= '9'
		if !tokenStarted {
			tokenStart = index
			digitsOnly = isDigit
		} else {
			digitsOnly = digitsOnly && isDigit
		}
		tokenStarted = true
	}

	return nil
}

func fdRedirectionError(operator string) error {
	return fmt.Errorf("file-descriptor redirection such as %q is not supported; > and >> redirect standard output only", operator)
}

func isUnsupportedShellParameter(input []rune) bool {
	if len(input) == 0 {
		return false
	}
	if unicode.IsDigit(input[0]) || strings.ContainsRune("?$!#*@-", input[0]) {
		return true
	}
	return len(input) > 1 && input[0] == '{' && (unicode.IsDigit(input[1]) || strings.ContainsRune("?$!#*@-", input[1]))
}

func unsupportedParameterError(input []rune) error {
	name := string(input[0])
	if input[0] == '{' {
		name = string(input)
		if end := strings.IndexRune(name, '}'); end >= 0 {
			name = name[:end+1]
		}
	}
	return fmt.Errorf("positional and special parameters such as %q are not supported; use single quotes to pass a literal $", "$"+name)
}
