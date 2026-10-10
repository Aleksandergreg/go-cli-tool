package sandbox

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
)

func (s *Sandbox) cmdFind(context *executionContext, args []string) (string, error) {
	var roots []string
	index := 0
	for index < len(args) && !strings.HasPrefix(args[index], "-") {
		roots = append(roots, args[index])
		index++
	}
	if len(roots) == 0 {
		roots = []string{"."}
	}

	// Tests combine with an implicit AND, as in find: every -name, -iname,
	// and -type must hold for an entry to be printed or passed to -exec.
	var tests []func(candidate string, entry *Entry) (bool, error)
	var execArgs []string
	for index < len(args) {
		switch args[index] {
		case "-name", "-iname":
			caseInsensitive := args[index] == "-iname"
			if index+1 >= len(args) {
				return "", fmt.Errorf("%s requires a pattern", args[index])
			}
			index++
			pattern := args[index]
			if caseInsensitive {
				pattern = strings.ToLower(pattern)
			}
			tests = append(tests, func(candidate string, _ *Entry) (bool, error) {
				name := path.Base(candidate)
				if caseInsensitive {
					name = strings.ToLower(name)
				}
				matched, err := matchShellPattern(pattern, name)
				if err != nil {
					return false, fmt.Errorf("invalid name pattern: %w", err)
				}
				return matched, nil
			})
		case "-type":
			if index+1 >= len(args) {
				return "", fmt.Errorf("-type requires f or d")
			}
			index++
			kind := Regular
			switch args[index] {
			case "f":
			case "d":
				kind = Directory
			default:
				return "", fmt.Errorf("unsupported type %q", args[index])
			}
			tests = append(tests, func(_ string, entry *Entry) (bool, error) {
				return entry.Kind == kind, nil
			})
		case "-a", "-and", "-print":
			// AND is already implicit, and printing is the default action.
		case "-exec":
			if execArgs != nil {
				return "", fmt.Errorf("this lab supports one -exec action per find")
			}
			index++
			start := index
			for index < len(args) && args[index] != ";" {
				index++
			}
			if index >= len(args) {
				return "", fmt.Errorf("-exec must end with \\;")
			}
			execArgs = slices.Clone(args[start:index])
			if len(execArgs) == 0 {
				return "", fmt.Errorf("-exec requires a command")
			}
		default:
			return "", fmt.Errorf("unsupported expression %s; tests combine with an implicit AND, and -o, !, and parentheses are not supported", args[index])
		}
		index++
	}

	var output commandOutputBuffer
	for _, root := range roots {
		rootAbs := s.Resolve(root)
		items, err := s.FS.Descendants(rootAbs, true)
		if err != nil {
			return "", err
		}
	candidates:
		for _, candidate := range items {
			entry, _ := s.FS.Entry(candidate)
			for _, test := range tests {
				matched, err := test(candidate, entry)
				if err != nil {
					return "", err
				}
				if !matched {
					continue candidates
				}
			}
			display := displayFindPath(root, rootAbs, candidate)
			if len(execArgs) == 0 {
				output.WriteString(display + "\n")
				if err := output.Err(); err != nil {
					return "", err
				}
				continue
			}
			command := slices.Clone(execArgs)
			for position, arg := range command {
				if arg == "{}" {
					command[position] = display
				}
			}
			// A failing status (grep without a match) only makes this -exec
			// test false; find continues with the next entry.
			result, err := s.run(context, command, "")
			if err != nil && !errors.Is(err, errFailureStatus) {
				return "", fmt.Errorf("-exec %s: %w", command[0], err)
			}
			output.WriteString(result)
			if err := output.Err(); err != nil {
				return "", err
			}
		}
	}
	return output.Result()
}

func displayFindPath(root, rootAbs, candidate string) string {
	if strings.HasPrefix(root, "/") {
		return candidate
	}
	rel := strings.TrimPrefix(candidate, rootAbs)
	if rel == "" {
		return path.Clean(root)
	}
	if root == "." || root == "./" {
		return "." + rel
	}
	return strings.TrimSuffix(root, "/") + rel
}
