package sandbox

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

func (s *Sandbox) cmdGrep(args []string, stdin string) (string, error) {
	recursive, namesOnly, lineNumbers := false, false, false
	insensitive, invert, fixed, countOnly, wholeWord, extended := false, false, false, false, false, false
	var operands []string
	optionsDone := false
	for _, arg := range args {
		if !optionsDone && arg == "--" {
			optionsDone = true
			continue
		}
		if !optionsDone && strings.HasPrefix(arg, "-") && arg != "-" {
			for _, option := range strings.TrimPrefix(arg, "-") {
				switch option {
				case 'r', 'R':
					recursive = true
				case 'l':
					namesOnly = true
				case 'n':
					lineNumbers = true
				case 'i':
					insensitive = true
				case 'v':
					invert = true
				case 'F':
					fixed = true
				case 'c':
					countOnly = true
				case 'w':
					wholeWord = true
				case 'E':
					extended = true
				default:
					return "", fmt.Errorf("unknown option -%c", option)
				}
			}
			continue
		}
		optionsDone = true
		operands = append(operands, arg)
	}
	if len(operands) == 0 {
		return "", fmt.Errorf("missing search pattern")
	}
	pattern := operands[0]
	if fixed {
		pattern = regexp.QuoteMeta(pattern)
	} else if !extended {
		translated, err := translateBasicRegex(pattern)
		if err != nil {
			return "", fmt.Errorf("invalid pattern: %w", err)
		}
		pattern = translated
	}
	if wholeWord {
		pattern = `\b(?:` + pattern + `)\b`
	}
	if insensitive {
		pattern = "(?i)" + pattern
	}
	matcher, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid pattern: %w", err)
	}

	fileArgs := operands[1:]
	inputs := make([]namedText, 0)
	if len(fileArgs) == 0 {
		inputs = append(inputs, namedText{text: stdin})
	} else {
		for _, name := range fileArgs {
			resolved := s.Resolve(name)
			entry, exists := s.FS.Entry(resolved)
			if !exists {
				return "", fmt.Errorf("%s: no such file or directory", name)
			}
			if entry.Kind == Directory {
				if !recursive {
					return "", fmt.Errorf("%s: is a directory", name)
				}
				paths, _ := s.FS.Descendants(resolved, false)
				for _, candidate := range paths {
					item, _ := s.FS.Entry(candidate)
					if item.Kind != Regular {
						continue
					}
					display := candidate
					if !strings.HasPrefix(name, "/") {
						rel := strings.TrimPrefix(candidate, resolved)
						display = strings.TrimSuffix(name, "/") + rel
					}
					inputs = append(inputs, namedText{name: display, text: item.Content})
				}
			} else {
				inputs = append(inputs, namedText{name: name, text: entry.Content})
			}
		}
	}

	showNames := len(inputs) > 1 || recursive
	var output commandOutputBuffer
	for _, input := range inputs {
		matchedFile := false
		matchCount := 0
		for index, line := range textLines(input.text) {
			matched := matcher.MatchString(line)
			if invert {
				matched = !matched
			}
			if !matched {
				continue
			}
			matchCount++
			if namesOnly {
				if input.name != "" && !matchedFile {
					output.WriteString(input.name + "\n")
					matchedFile = true
				}
				continue
			}
			if countOnly {
				continue
			}
			if showNames && input.name != "" {
				output.WriteString(input.name + ":")
			}
			if lineNumbers {
				output.WriteString(strconv.Itoa(index+1) + ":")
			}
			output.WriteString(line + "\n")
		}
		if countOnly && !namesOnly {
			if showNames && input.name != "" {
				output.WriteString(input.name + ":")
			}
			output.WriteString(strconv.Itoa(matchCount) + "\n")
		}
	}
	return output.Result()
}
