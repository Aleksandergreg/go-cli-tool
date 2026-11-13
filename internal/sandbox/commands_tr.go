package sandbox

import (
	"fmt"
	"strings"
)

func (s *Sandbox) cmdTr(args []string, stdin string) (string, error) {
	options, operands, err := parseShortOptions(args, "ds", true)
	if err != nil {
		return "", err
	}
	deleteSet, squeeze := strings.ContainsRune(options, 'd'), strings.ContainsRune(options, 's')
	// GNU tr takes one set for -d or -s alone, two sets to translate, and two
	// sets for -ds (delete SET1, then squeeze SET2).
	wantOperands := 2
	if deleteSet != squeeze {
		wantOperands = 1
	}
	if len(operands) < wantOperands || len(operands) > 2 || deleteSet && !squeeze && len(operands) > 1 {
		return "", fmt.Errorf("usage: tr SET1 SET2, tr -d SET1, tr -s SET1 [SET2], or tr -ds SET1 SET2")
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
			targetIndex := min(index, len(set2)-1)
			translations[char] = set2[targetIndex]
		}
	}
	squeezeSet := set2
	if len(squeezeSet) == 0 {
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
		if class, length, found := characterClass(runes[index:]); found {
			if class == nil {
				return nil, fmt.Errorf("unknown character class %s", string(runes[index:index+length]))
			}
			result = append(result, class...)
			index += length - 1
			continue
		}
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

// characterClass expands a POSIX [:name:] class at the start of input. found
// reports whether input starts with a complete [:...:] token; class is nil for
// an unknown name. Classes cover ASCII, matching the C locale.
func characterClass(input []rune) (class []rune, length int, found bool) {
	if len(input) < 4 || input[0] != '[' || input[1] != ':' {
		return nil, 0, false
	}
	for end := 2; end+1 < len(input); end++ {
		if input[end] != ':' || input[end+1] != ']' {
			continue
		}
		name := string(input[2:end])
		for char := rune(0); char < 128; char++ {
			if asciiClassContains(name, char) {
				class = append(class, char)
			}
		}
		return class, end + 2, true
	}
	return nil, 0, false
}

func asciiClassContains(name string, char rune) bool {
	lower, upper, digit := 'a' <= char && char <= 'z', 'A' <= char && char <= 'Z', '0' <= char && char <= '9'
	switch name {
	case "alnum":
		return lower || upper || digit
	case "alpha":
		return lower || upper
	case "blank":
		return char == ' ' || char == '\t'
	case "digit":
		return digit
	case "lower":
		return lower
	case "upper":
		return upper
	case "punct":
		return char > ' ' && char < 127 && !lower && !upper && !digit
	case "space":
		return strings.ContainsRune(" \t\n\v\f\r", char)
	case "xdigit":
		return digit || 'a' <= char && char <= 'f' || 'A' <= char && char <= 'F'
	}
	return false
}
