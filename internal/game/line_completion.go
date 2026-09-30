package game

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

func terminalCompleter(completions CompletionSource) func(string, int, rune) (string, int, bool) {
	return func(line string, position int, key rune) (string, int, bool) {
		switch key {
		case terminalKeyDeleteForward:
			return deleteRuneAtCursor(line, position)
		case '\t':
			return completeLine(line, position, completions)
		default:
			return line, position, false
		}
	}
}

func deleteRuneAtCursor(line string, position int) (string, int, bool) {
	if invalidCursorPosition(line, position) {
		return line, position, true
	}
	if position == len(line) {
		return line, position, true
	}
	_, size := utf8.DecodeRuneInString(line[position:])
	return line[:position] + line[position+size:], position, true
}

func completeLine(line string, position int, completions CompletionSource) (string, int, bool) {
	if invalidCursorPosition(line, position) {
		return line, position, true
	}

	start, end := completionTokenRange(line, position)
	typed, quote, valid := completionToken(line[start:position])
	if !valid {
		return line, position, true
	}

	var candidates []CompletionCandidate
	if commandPosition(line[:start]) && !scriptPathCompletion(typed) {
		candidates = commandCandidates(completions, typed)
	} else if completions != nil {
		candidates = completions.PathCandidates(typed)
	}
	if len(candidates) == 0 {
		return line, position, true
	}

	replacement := ""
	if len(candidates) == 1 {
		candidate := candidates[0]
		replacement = formatCompletion(candidate.Value, quote, !candidate.Directory)
		if !candidate.Directory && completionNeedsSpace(line[end:]) {
			replacement += " "
		}
	} else {
		common := longestCommonPrefix(candidates)
		if common == typed {
			return line, position, true
		}
		replacement = formatCompletion(common, quote, false)
	}

	completed := line[:start] + replacement + line[end:]
	return completed, start + len(replacement), true
}

func invalidCursorPosition(line string, position int) bool {
	return position < 0 || position > len(line) || !utf8.ValidString(line) || position < len(line) && !utf8.RuneStart(line[position])
}

func scriptPathCompletion(value string) bool {
	return strings.Contains(value, "/") || strings.HasPrefix(value, ".") || strings.HasPrefix(value, "~")
}

func completionTokenRange(line string, position int) (int, int) {
	start, end := 0, len(line)
	var quote byte
	escaped := false
	for index := 0; index < len(line); index++ {
		char := line[index]
		if escaped {
			escaped = false
			continue
		}
		if char == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			continue
		}
		if completionBoundary(char) {
			if index < position {
				start = index + 1
				continue
			}
			end = index
			break
		}
	}
	return start, end
}

func completionBoundary(char byte) bool {
	return strings.ContainsRune(" \t\r\n|<>", rune(char))
}

func completionToken(raw string) (string, byte, bool) {
	var quote byte
	if raw != "" && (raw[0] == '\'' || raw[0] == '"') {
		quote = raw[0]
		raw = raw[1:]
		if strings.ContainsRune(raw, rune(quote)) {
			return "", 0, false
		}
	}
	if quote == '\'' {
		return raw, quote, true
	}

	var value strings.Builder
	escaped := false
	for _, char := range raw {
		if escaped {
			value.WriteRune(char)
			escaped = false
			continue
		}
		if char == '\\' {
			escaped = true
			continue
		}
		value.WriteRune(char)
	}
	if escaped {
		return "", 0, false
	}
	return value.String(), quote, true
}

func commandPosition(beforeToken string) bool {
	trimmed := strings.TrimSpace(beforeToken)
	return trimmed == "" || strings.HasSuffix(trimmed, "|")
}

func commandCandidates(completions CompletionSource, prefix string) []CompletionCandidate {
	commands := make([]string, 0)
	if completions != nil {
		commands = append(commands, completions.CommandNames()...)
	}
	commands = append(commands, "?", ":q", "exit", "guide", "hint", "list", "map", "missions", "next", "objective", "opsquest", "play", "prev", "previous", "quit", "restart", "status", "world", "worlds")
	sort.Strings(commands)
	candidates := make([]CompletionCandidate, 0)
	last := ""
	for _, command := range commands {
		if command == last || !strings.HasPrefix(command, prefix) {
			continue
		}
		candidates = append(candidates, CompletionCandidate{Value: command})
		last = command
	}
	return candidates
}

func longestCommonPrefix(candidates []CompletionCandidate) string {
	prefix := []rune(candidates[0].Value)
	for _, candidate := range candidates[1:] {
		value := []rune(candidate.Value)
		limit := len(prefix)
		if len(value) < limit {
			limit = len(value)
		}
		index := 0
		for index < limit && prefix[index] == value[index] {
			index++
		}
		prefix = prefix[:index]
	}
	return string(prefix)
}

func formatCompletion(value string, quote byte, closeQuote bool) string {
	if quote == '\'' {
		result := "'" + strings.ReplaceAll(value, "'", `'\''`)
		if closeQuote {
			result += "'"
		}
		return result
	}
	if quote == '"' {
		replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`)
		result := `"` + replacer.Replace(value)
		if closeQuote {
			result += `"`
		}
		return result
	}

	var escaped strings.Builder
	for _, char := range value {
		if unicode.IsSpace(char) || strings.ContainsRune("\\'\"|<>#$*?[", char) {
			escaped.WriteRune('\\')
		}
		escaped.WriteRune(char)
	}
	return escaped.String()
}

func completionNeedsSpace(afterToken string) bool {
	return afterToken == "" || strings.ContainsRune("|<>", rune(afterToken[0]))
}
