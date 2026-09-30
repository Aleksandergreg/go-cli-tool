package sandbox

import (
	"fmt"
	"sort"
	"strings"
)

func sortedKeys[V any](items map[string]V) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// parseShortOptions handles the shared, deliberately small combined-short-flag
// syntax used by teaching commands such as ls, du, sort, and tr.
func parseShortOptions(args []string, valid string, dashIsOperand bool) (string, []string, error) {
	var options strings.Builder
	operands := make([]string, 0, len(args))
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") || dashIsOperand && arg == "-" {
			operands = append(operands, arg)
			continue
		}
		for _, option := range strings.TrimPrefix(arg, "-") {
			if !strings.ContainsRune(valid, option) {
				return "", nil, fmt.Errorf("unknown option -%c", option)
			}
			options.WriteRune(option)
		}
	}
	return options.String(), operands, nil
}
