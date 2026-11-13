package sandbox

import (
	"fmt"
	"strconv"
	"strings"
)

// translateBasicRegex converts a POSIX basic regular expression, the default
// syntax of grep and sed, into Go RE2 syntax. In basic syntax ( ) { } | + ?
// are literal and their backslash-escaped forms are operators (GNU's \| \+ \?
// extensions included), so 'ERROR\|WARN' alternates while 'ERROR|WARN'
// matches a literal bar. Extended syntax (-E) is passed to RE2 unchanged.
func translateBasicRegex(pattern string) (string, error) {
	runes := []rune(pattern)
	var out strings.Builder
	// caretAnchors is true where ^ is an anchor: the start of the pattern, a
	// group, or an alternative. starLiteral is true where * has nothing to
	// repeat and is therefore literal; that also includes just after ^.
	caretAnchors, starLiteral := true, true
	for index := 0; index < len(runes); index++ {
		char := runes[index]
		nextCaretAnchors, nextStarLiteral := false, false
		switch {
		case char == '[':
			end := bracketExpressionEnd(runes, index)
			if end < 0 {
				return "", fmt.Errorf("unterminated bracket expression")
			}
			// Backslash is an ordinary character inside POSIX brackets.
			out.WriteString(strings.ReplaceAll(string(runes[index:end+1]), `\`, `\\`))
			index = end
		case char == '\\':
			if index+1 >= len(runes) {
				return "", fmt.Errorf("trailing backslash")
			}
			index++
			escaped := runes[index]
			switch {
			case escaped == '(' || escaped == '|':
				out.WriteRune(escaped)
				nextCaretAnchors, nextStarLiteral = true, true
			case strings.ContainsRune("){}+?", escaped):
				out.WriteRune(escaped)
			case escaped == '<' || escaped == '>':
				out.WriteString(`\b`)
			case escaped >= '1' && escaped <= '9':
				return "", fmt.Errorf("backreferences such as \\%c are not supported in patterns", escaped)
			default:
				out.WriteRune('\\')
				out.WriteRune(escaped)
			}
		case strings.ContainsRune("(){}|+?", char):
			out.WriteRune('\\')
			out.WriteRune(char)
		case char == '*' && starLiteral:
			out.WriteString(`\*`)
		case char == '^' && caretAnchors:
			out.WriteRune(char)
			nextStarLiteral = true
		case char == '^':
			out.WriteString(`\^`)
		case char == '$' && !basicDollarAnchors(runes, index):
			out.WriteString(`\$`)
		default:
			out.WriteRune(char)
		}
		caretAnchors, starLiteral = nextCaretAnchors, nextStarLiteral
	}
	return out.String(), nil
}

// basicDollarAnchors reports whether $ at index ends the pattern, a group, or
// an alternative; elsewhere a basic regular expression treats it literally.
func basicDollarAnchors(runes []rune, index int) bool {
	rest := runes[index+1:]
	return len(rest) == 0 || len(rest) >= 2 && rest[0] == '\\' && (rest[1] == ')' || rest[1] == '|')
}

// bracketExpressionEnd returns the index of the ] closing the bracket
// expression opening at start, or -1. A ] immediately after [ or [^ is a
// member, and [:class:], [.coll.], and [=equiv=] spans are skipped whole.
func bracketExpressionEnd(runes []rune, start int) int {
	index := start + 1
	if index < len(runes) && runes[index] == '^' {
		index++
	}
	if index < len(runes) && runes[index] == ']' {
		index++
	}
	for ; index < len(runes); index++ {
		if runes[index] == ']' {
			return index
		}
		if runes[index] != '[' || index+1 >= len(runes) || !strings.ContainsRune(":.=", runes[index+1]) {
			continue
		}
		delimiter := runes[index+1]
		for closing := index + 2; closing+1 < len(runes); closing++ {
			if runes[closing] == delimiter && runes[closing+1] == ']' {
				index = closing + 1
				break
			}
		}
	}
	return -1
}

// translateSedReplacement converts sed replacement text into a Go
// regexp.Expand template: & is the whole match, \1-\9 are groups, \n and \t
// are newline and tab, any other escaped character is literal, and a literal
// $ is doubled so Go does not read it as a group reference.
func translateSedReplacement(replacement string, groups int) (string, error) {
	var out strings.Builder
	for index := 0; index < len(replacement); index++ {
		char := replacement[index]
		switch {
		case char == '&':
			out.WriteString("${0}")
		case char == '$':
			out.WriteString("$$")
		case char == '\\' && index+1 < len(replacement):
			index++
			escaped := replacement[index]
			switch {
			case escaped >= '0' && escaped <= '9':
				group, _ := strconv.Atoi(string(escaped))
				if group > groups {
					return "", fmt.Errorf("invalid reference \\%d on s command's replacement: the pattern has %d group(s)", group, groups)
				}
				out.WriteString("${" + string(escaped) + "}")
			case escaped == 'n':
				out.WriteByte('\n')
			case escaped == 't':
				out.WriteByte('\t')
			case escaped == '$':
				out.WriteString("$$")
			default:
				out.WriteByte(escaped)
			}
		default:
			out.WriteByte(char)
		}
	}
	return out.String(), nil
}
