package sandbox

import (
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

	namePattern := ""
	caseInsensitive := false
	entryType := ""
	var execArgs []string
	for index < len(args) {
		switch args[index] {
		case "-name", "-iname":
			caseInsensitive = args[index] == "-iname"
			if index+1 >= len(args) {
				return "", fmt.Errorf("%s requires a pattern", args[index])
			}
			index++
			namePattern = args[index]
		case "-type":
			if index+1 >= len(args) {
				return "", fmt.Errorf("-type requires f or d")
			}
			index++
			entryType = args[index]
			if entryType != "f" && entryType != "d" {
				return "", fmt.Errorf("unsupported type %q", entryType)
			}
		case "-print":
			// Printing is the default action.
		case "-exec":
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
			return "", fmt.Errorf("unsupported expression %s", args[index])
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
		for _, candidate := range items {
			entry, _ := s.FS.Entry(candidate)
			if entryType == "f" && entry.Kind != Regular {
				continue
			}
			if entryType == "d" && entry.Kind != Directory {
				continue
			}
			if namePattern != "" {
				candidateName, pattern := path.Base(candidate), namePattern
				if caseInsensitive {
					candidateName, pattern = strings.ToLower(candidateName), strings.ToLower(pattern)
				}
				matched, err := matchShellPattern(pattern, candidateName)
				if err != nil {
					return "", fmt.Errorf("invalid name pattern: %w", err)
				}
				if !matched {
					continue
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
			result, err := s.run(context, command, "")
			if err != nil {
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
