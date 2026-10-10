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

// operandsOnly returns the operands of a command that accepts no options in
// this lab. As in GNU tools, -- ends option parsing and a lone - remains an
// operand; any other dash-prefixed argument is reported as an unknown option
// instead of being treated as a file name.
func operandsOnly(args []string) ([]string, error) {
	operands := make([]string, 0, len(args))
	for index, arg := range args {
		if arg == "--" {
			return append(operands, args[index+1:]...), nil
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			return nil, fmt.Errorf("unknown option %s", arg)
		}
		operands = append(operands, arg)
	}
	return operands, nil
}
