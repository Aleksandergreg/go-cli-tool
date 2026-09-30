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
			targetIndex := min(index, len(set2)-1)
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
