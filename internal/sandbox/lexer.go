package sandbox

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type tokenKind int

type token struct {
	kind  tokenKind
	value string
	glob  bool
}

// lex splits one pipeline into tokens, expanding variables from env and $?
// from status.
func lex(line string, env map[string]string, status int) ([]token, error) {
	var tokens []token
	var current strings.Builder
	var quote rune
	tokenStarted := false
	wordGlob := false
	runes := []rune(line)
	expandedBytes := 0

	reserve := func(size int) error {
		if size > maxExpandedTokenBytes-expandedBytes {
			return fmt.Errorf("expanded command exceeds the %d KiB token limit", maxExpandedTokenBytes/1024)
		}
		expandedBytes += size
		return nil
	}
	writeString := func(value string) error {
		if err := reserve(len(value)); err != nil {
			return err
		}
		current.WriteString(value)
		return nil
	}
	writeRune := func(value rune) error {
		size := utf8.RuneLen(value)
		if size < 0 {
			size = utf8.RuneLen(utf8.RuneError)
		}
		if err := reserve(size); err != nil {
			return err
		}
		current.WriteRune(value)
		return nil
	}

	flush := func() {
		if tokenStarted {
			tokens = append(tokens, token{kind: wordToken, value: current.String(), glob: wordGlob})
			current.Reset()
			tokenStarted = false
			wordGlob = false
		}
	}

	for i := 0; i < len(runes); i++ {
		char := runes[i]
		if quote == 0 {
			switch {
			case unicode.IsSpace(char):
				flush()
				continue
			case char == '\'' || char == '"':
				quote = char
				tokenStarted = true
				continue
			case char == '\\':
				if i+1 >= len(runes) {
					return nil, fmt.Errorf("unfinished escape")
				}
				i++
				if err := writeRune(runes[i]); err != nil {
					return nil, err
				}
				tokenStarted = true
				continue
			case char == '#' && !tokenStarted:
				flush()
				return tokens, nil
			case char == '|':
				flush()
				tokens = append(tokens, token{kind: pipeToken, value: "|"})
				continue
			case char == '<':
				flush()
				tokens = append(tokens, token{kind: inputToken, value: "<"})
				continue
			case char == '>':
				flush()
				if i+1 < len(runes) && runes[i+1] == '>' {
					i++
					tokens = append(tokens, token{kind: appendToken, value: ">>"})
				} else {
					tokens = append(tokens, token{kind: redirectToken, value: ">"})
				}
				continue
			case char == '$':
				value, consumed := expandVariable(runes[i:], env, status)
				if err := writeString(value); err != nil {
					return nil, err
				}
				wordGlob = wordGlob || strings.ContainsAny(value, "*?[")
				i += consumed
				tokenStarted = true
				continue
			default:
				if err := writeRune(char); err != nil {
					return nil, err
				}
				wordGlob = wordGlob || strings.ContainsRune("*?[", char)
				tokenStarted = true
				continue
			}
		}

		if char == quote {
			quote = 0
			continue
		}
		// Inside double quotes a backslash escapes only $, `, ", \, and newline;
		// before any other character it stays literal, so "a\nb" reaches
		// printf with its \n intact.
		if quote == '"' && char == '\\' {
			if i+1 >= len(runes) {
				return nil, fmt.Errorf("unfinished escape")
			}
			if !strings.ContainsRune("$`\"\\\n", runes[i+1]) {
				if err := writeRune(char); err != nil {
					return nil, err
				}
				continue
			}
			i++
			if err := writeRune(runes[i]); err != nil {
				return nil, err
			}
			continue
		}
		if quote == '"' && char == '$' {
			value, consumed := expandVariable(runes[i:], env, status)
			if err := writeString(value); err != nil {
				return nil, err
			}
			i += consumed
			continue
		}
		if err := writeRune(char); err != nil {
			return nil, err
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote")
	}
	flush()
	return tokens, nil
}

func expandVariable(input []rune, env map[string]string, status int) (string, int) {
	if len(input) < 2 {
		return "$", 0
	}
	if input[1] == '?' {
		return strconv.Itoa(status), 1
	}
	if input[1] == '{' {
		for i := 2; i < len(input); i++ {
			if input[i] == '}' {
				if name := string(input[2:i]); name == "?" {
					return strconv.Itoa(status), i
				}
				return env[string(input[2:i])], i
			}
		}
		return "$", 0
	}
	i := 1
	for i < len(input) && (unicode.IsLetter(input[i]) || unicode.IsDigit(input[i]) || input[i] == '_') {
		i++
	}
	if i == 1 {
		return "$", 0
	}
	return env[string(input[1:i])], i - 1
}
