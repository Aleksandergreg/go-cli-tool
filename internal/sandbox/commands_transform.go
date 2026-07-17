package sandbox

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

type sedSubstitution struct {
	pattern     *regexp.Regexp
	replacement string
	global      bool
	printMatch  bool
}

func (s *Sandbox) cmdSed(args []string, stdin string) (string, error) {
	inPlace, quiet := false, false
	expression := ""
	files := make([]string, 0)
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "-i":
			inPlace = true
		case "-n":
			quiet = true
		case "-e":
			if index+1 >= len(args) {
				return "", fmt.Errorf("-e requires an expression")
			}
			index++
			if expression != "" {
				return "", fmt.Errorf("this lab supports one sed expression at a time")
			}
			expression = args[index]
		default:
			if strings.HasPrefix(args[index], "-") && expression == "" {
				return "", fmt.Errorf("unknown option %s", args[index])
			}
			if expression == "" {
				expression = args[index]
			} else {
				files = append(files, args[index])
			}
		}
	}
	if expression == "" {
		return "", fmt.Errorf("missing sed expression")
	}
	if inPlace && len(files) == 0 {
		return "", fmt.Errorf("-i requires at least one file")
	}
	substitution, err := parseSedSubstitution(expression)
	if err != nil {
		return "", err
	}
	inputs, err := s.readInputs(files, stdin)
	if err != nil {
		return "", err
	}
	var output commandOutputBuffer
	for _, input := range inputs {
		transformed, selected, err := applySedSubstitution(input.text, substitution)
		if err != nil {
			return "", err
		}
		if inPlace {
			if err := s.FS.WriteFile(s.Resolve(input.name), transformed, 0); err != nil {
				return "", err
			}
			s.removeArchiveMetadata(s.Resolve(input.name))
		}
		if quiet {
			if substitution.printMatch {
				output.WriteString(selected)
			}
		} else if !inPlace {
			output.WriteString(transformed)
		} else if substitution.printMatch {
			output.WriteString(selected)
		}
	}
	return output.Result()
}

func parseSedSubstitution(expression string) (sedSubstitution, error) {
	if len(expression) < 4 || expression[0] != 's' {
		return sedSubstitution{}, fmt.Errorf("this lab supports substitutions like s/old/new/g")
	}
	delimiter := expression[1]
	if delimiter == '\\' || delimiter == '\n' {
		return sedSubstitution{}, fmt.Errorf("invalid substitution delimiter")
	}
	pattern, next, err := readSedSection(expression, 2, delimiter)
	if err != nil {
		return sedSubstitution{}, err
	}
	replacement, next, err := readSedSection(expression, next, delimiter)
	if err != nil {
		return sedSubstitution{}, err
	}
	matcher, err := regexp.Compile(pattern)
	if err != nil {
		return sedSubstitution{}, fmt.Errorf("invalid regular expression: %w", err)
	}
	result := sedSubstitution{pattern: matcher, replacement: replacement}
	for _, flag := range expression[next:] {
		switch flag {
		case 'g':
			result.global = true
		case 'p':
			result.printMatch = true
		default:
			return sedSubstitution{}, fmt.Errorf("unsupported substitution flag %q", flag)
		}
	}
	return result, nil
}

func readSedSection(expression string, start int, delimiter byte) (string, int, error) {
	var section strings.Builder
	for index := start; index < len(expression); index++ {
		if expression[index] == delimiter {
			return section.String(), index + 1, nil
		}
		if expression[index] == '\\' && index+1 < len(expression) {
			if expression[index+1] == delimiter {
				section.WriteByte(delimiter)
				index++
				continue
			}
			section.WriteByte('\\')
			continue
		}
		section.WriteByte(expression[index])
	}
	return "", 0, fmt.Errorf("unterminated substitution")
}

func applySedSubstitution(content string, substitution sedSubstitution) (string, string, error) {
	hadFinalNewline := strings.HasSuffix(content, "\n")
	trimmed := strings.TrimSuffix(content, "\n")
	if trimmed == "" && content == "" {
		return "", "", nil
	}
	lines := strings.Split(trimmed, "\n")
	projectedBytes := 0
	for index, line := range lines {
		lineBytes, err := sedSubstitutedLineBytes(line, substitution)
		if err != nil {
			return "", "", err
		}
		additional := lineBytes
		if index < len(lines)-1 || index == len(lines)-1 && hadFinalNewline {
			additional++
		}
		if additional > maxVirtualFileBytes-projectedBytes {
			return "", "", fmt.Errorf("substitution result exceeds the %d KiB limit", maxVirtualFileBytes/1024)
		}
		projectedBytes += additional
	}
	selected := make([]string, 0)
	for index, line := range lines {
		matched := substitution.pattern.MatchString(line)
		if substitution.global {
			updated := substitution.pattern.ReplaceAllString(line, substitution.replacement)
			lines[index] = updated
		} else if match := substitution.pattern.FindStringSubmatchIndex(line); match != nil {
			replacement := substitution.pattern.ExpandString(nil, substitution.replacement, line, match)
			lines[index] = line[:match[0]] + string(replacement) + line[match[1]:]
		}
		if matched && substitution.printMatch {
			selected = append(selected, lines[index])
		}
	}
	transformed := strings.Join(lines, "\n")
	if hadFinalNewline {
		transformed += "\n"
	}
	return transformed, joinOutputLines(selected), nil
}

func sedSubstitutedLineBytes(line string, substitution sedSubstitution) (int, error) {
	if !substitution.global {
		match := substitution.pattern.FindStringSubmatchIndex(line)
		if match == nil {
			return len(line), nil
		}
		replacementBytes, err := sedReplacementUpperBound(substitution.pattern, substitution.replacement, line, match)
		if err != nil {
			return 0, err
		}
		base := len(line) - (match[1] - match[0])
		return addSedResultBytes(base, replacementBytes)
	}

	total, lastMatchEnd, searchPosition := 0, 0, 0
	for searchPosition <= len(line) {
		relativeMatch := substitution.pattern.FindStringSubmatchIndex(line[searchPosition:])
		if relativeMatch == nil {
			break
		}
		match := make([]int, len(relativeMatch))
		for index, position := range relativeMatch {
			if position < 0 {
				match[index] = position
			} else {
				match[index] = searchPosition + position
			}
		}
		start, end := match[0], match[1]
		// Match regexp.ReplaceAllString's treatment of empty matches next to
		// a previous match so the preflight remains a safe upper bound.
		if end > lastMatchEnd || start == 0 {
			var err error
			total, err = addSedResultBytes(total, start-lastMatchEnd)
			if err != nil {
				return 0, err
			}
			replacementBytes, err := sedReplacementUpperBound(substitution.pattern, substitution.replacement, line, match)
			if err != nil {
				return 0, err
			}
			total, err = addSedResultBytes(total, replacementBytes)
			if err != nil {
				return 0, err
			}
			lastMatchEnd = end
		}
		if end == searchPosition {
			if searchPosition == len(line) {
				break
			}
			_, width := utf8.DecodeRuneInString(line[searchPosition:])
			searchPosition = end + width
		} else {
			searchPosition = end
		}
	}
	return addSedResultBytes(total, len(line)-lastMatchEnd)
}

func sedReplacementUpperBound(pattern *regexp.Regexp, replacement, source string, match []int) (int, error) {
	total := 0
	for index := 0; index < len(replacement); {
		if replacement[index] != '$' {
			var err error
			total, err = addSedResultBytes(total, 1)
			if err != nil {
				return 0, err
			}
			index++
			continue
		}
		if index+1 < len(replacement) && replacement[index+1] == '$' {
			var err error
			total, err = addSedResultBytes(total, 1)
			if err != nil {
				return 0, err
			}
			index += 2
			continue
		}

		start := index
		index++
		name := ""
		if index < len(replacement) && replacement[index] == '{' {
			closing := strings.IndexByte(replacement[index+1:], '}')
			if closing >= 0 {
				closing += index + 1
				name = replacement[index+1 : closing]
				index = closing + 1
			}
		} else {
			nameStart := index
			for index < len(replacement) && isSedReplacementNameByte(replacement[index]) {
				index++
			}
			name = replacement[nameStart:index]
		}
		if name == "" {
			var err error
			total, err = addSedResultBytes(total, index-start)
			if err != nil {
				return 0, err
			}
			continue
		}

		group := pattern.SubexpIndex(name)
		if numeric, err := strconv.Atoi(name); err == nil {
			group = numeric
		}
		position := group * 2
		if group >= 0 && position+1 < len(match) && match[position] >= 0 {
			var err error
			total, err = addSedResultBytes(total, match[position+1]-match[position])
			if err != nil {
				return 0, err
			}
		} else if group < 0 {
			// Unknown references expand to empty in Go's regexp package. Count
			// their source spelling anyway so this remains a conservative bound.
			var err error
			total, err = addSedResultBytes(total, index-start)
			if err != nil {
				return 0, err
			}
		}
	}
	return total, nil
}

func addSedResultBytes(total, additional int) (int, error) {
	if additional > maxVirtualFileBytes-total {
		return 0, fmt.Errorf("substitution result exceeds the %d KiB limit", maxVirtualFileBytes/1024)
	}
	return total + additional, nil
}

func isSedReplacementNameByte(value byte) bool {
	return value == '_' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}

func (s *Sandbox) cmdTr(args []string, stdin string) (string, error) {
	deleteSet, squeeze := false, false
	var operands []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && arg != "-" {
			for _, option := range strings.TrimPrefix(arg, "-") {
				switch option {
				case 'd':
					deleteSet = true
				case 's':
					squeeze = true
				default:
					return "", fmt.Errorf("unknown option -%c", option)
				}
			}
		} else {
			operands = append(operands, arg)
		}
	}
	if len(operands) == 0 || (!deleteSet && !squeeze && len(operands) < 2) || len(operands) > 2 {
		return "", fmt.Errorf("usage: tr [-ds] SET1 [SET2]")
	}
	set1, err := expandCharacterSet(operands[0])
	if err != nil {
		return "", err
	}
	set2 := []rune(nil)
	if len(operands) == 2 {
		set2, err = expandCharacterSet(operands[1])
		if err != nil {
			return "", err
		}
	}
	deleteMap := make(map[rune]bool, len(set1))
	translations := make(map[rune]rune, len(set1))
	for index, char := range set1 {
		deleteMap[char] = true
		if !deleteSet && len(set2) > 0 {
			targetIndex := index
			if targetIndex >= len(set2) {
				targetIndex = len(set2) - 1
			}
			translations[char] = set2[targetIndex]
		}
	}
	squeezeSet := set2
	if deleteSet || len(squeezeSet) == 0 {
		squeezeSet = set1
	}
	squeezeMap := make(map[rune]bool, len(squeezeSet))
	for _, char := range squeezeSet {
		squeezeMap[char] = true
	}
	var output commandOutputBuffer
	var previous rune
	havePrevious := false
	for _, char := range stdin {
		if deleteSet && deleteMap[char] {
			continue
		}
		if translated, exists := translations[char]; exists {
			char = translated
		}
		if squeeze && havePrevious && char == previous && squeezeMap[char] {
			continue
		}
		output.WriteRune(char)
		previous, havePrevious = char, true
	}
	return output.Result()
}

func expandCharacterSet(value string) ([]rune, error) {
	value = strings.NewReplacer(`\n`, "\n", `\t`, "\t", `\r`, "\r").Replace(value)
	runes := []rune(value)
	result := make([]rune, 0, len(runes))
	for index := 0; index < len(runes); index++ {
		if index+2 < len(runes) && runes[index+1] == '-' {
			if runes[index] > runes[index+2] {
				return nil, fmt.Errorf("descending character range %c-%c", runes[index], runes[index+2])
			}
			for char := runes[index]; char <= runes[index+2]; char++ {
				result = append(result, char)
			}
			index += 2
			continue
		}
		result = append(result, runes[index])
	}
	return result, nil
}
