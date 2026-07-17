package sandbox

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func (s *Sandbox) readInputs(args []string, stdin string) ([]namedText, error) {
	if len(args) == 0 {
		return []namedText{{text: stdin}}, nil
	}
	inputs := make([]namedText, 0, len(args))
	for _, name := range args {
		content, err := s.FS.ReadFile(s.Resolve(name))
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, namedText{name: name, text: content})
	}
	return inputs, nil
}

type namedText struct {
	name string
	text string
}

func (s *Sandbox) cmdCat(args []string, stdin string) (string, error) {
	inputs, err := s.readInputs(args, stdin)
	if err != nil {
		return "", err
	}
	var output strings.Builder
	for _, input := range inputs {
		output.WriteString(input.text)
	}
	return output.String(), nil
}

func (s *Sandbox) cmdHeadTail(args []string, stdin string, head bool) (string, error) {
	count := 10
	var files []string
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "-n":
			if index+1 >= len(args) {
				return "", fmt.Errorf("-n requires a line count")
			}
			index++
			value, err := strconv.Atoi(args[index])
			if err != nil || value < 0 {
				return "", fmt.Errorf("invalid line count %q", args[index])
			}
			count = value
		case strings.HasPrefix(arg, "-") && len(arg) > 1:
			value, err := strconv.Atoi(strings.TrimPrefix(arg, "-"))
			if err != nil || value < 0 {
				return "", fmt.Errorf("unknown option %s", arg)
			}
			count = value
		default:
			files = append(files, arg)
		}
	}
	inputs, err := s.readInputs(files, stdin)
	if err != nil {
		return "", err
	}
	var output strings.Builder
	for index, input := range inputs {
		if len(inputs) > 1 {
			if index > 0 {
				output.WriteByte('\n')
			}
			output.WriteString("==> " + input.name + " <==\n")
		}
		lines := textLines(input.text)
		limit := count
		if limit > len(lines) {
			limit = len(lines)
		}
		if head {
			lines = lines[:limit]
		} else {
			lines = lines[len(lines)-limit:]
		}
		if len(lines) > 0 {
			output.WriteString(strings.Join(lines, "\n") + "\n")
		}
	}
	return output.String(), nil
}

func (s *Sandbox) cmdGrep(args []string, stdin string) (string, error) {
	recursive, namesOnly, lineNumbers := false, false, false
	insensitive, invert, fixed, countOnly, wholeWord := false, false, false, false, false
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
					// Go regular expressions already use extended-style syntax.
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
	var output strings.Builder
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
	return output.String(), nil
}

func (s *Sandbox) cmdSort(args []string, stdin string) (string, error) {
	reverse, unique, numeric := false, false, false
	var files []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			for _, option := range strings.TrimPrefix(arg, "-") {
				switch option {
				case 'r':
					reverse = true
				case 'u':
					unique = true
				case 'n':
					numeric = true
				default:
					return "", fmt.Errorf("unknown option -%c", option)
				}
			}
		} else {
			files = append(files, arg)
		}
	}
	inputs, err := s.readInputs(files, stdin)
	if err != nil {
		return "", err
	}
	lines := make([]string, 0)
	for _, input := range inputs {
		lines = append(lines, textLines(input.text)...)
	}
	if numeric {
		sort.SliceStable(lines, func(i, j int) bool {
			left, right := leadingNumber(lines[i]), leadingNumber(lines[j])
			if left == right {
				return lines[i] < lines[j]
			}
			return left < right
		})
	} else {
		sort.Strings(lines)
	}
	if reverse {
		for left, right := 0, len(lines)-1; left < right; left, right = left+1, right-1 {
			lines[left], lines[right] = lines[right], lines[left]
		}
	}
	if unique {
		lines = uniqueLines(lines)
	}
	return joinOutputLines(lines), nil
}

func leadingNumber(value string) float64 {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return 0
	}
	number, _ := strconv.ParseFloat(fields[0], 64)
	return number
}

func (s *Sandbox) cmdUniq(args []string, stdin string) (string, error) {
	count := false
	var files []string
	for _, arg := range args {
		if arg == "-c" || arg == "--count" {
			count = true
		} else if strings.HasPrefix(arg, "-") {
			return "", fmt.Errorf("unknown option %s", arg)
		} else {
			files = append(files, arg)
		}
	}
	if len(files) > 1 {
		return "", fmt.Errorf("too many files")
	}
	inputs, err := s.readInputs(files, stdin)
	if err != nil {
		return "", err
	}
	lines := textLines(inputs[0].text)
	if len(lines) == 0 {
		return "", nil
	}
	var output strings.Builder
	for index := 0; index < len(lines); {
		next := index + 1
		for next < len(lines) && lines[next] == lines[index] {
			next++
		}
		if count {
			output.WriteString(fmt.Sprintf("%7d %s\n", next-index, lines[index]))
		} else {
			output.WriteString(lines[index] + "\n")
		}
		index = next
	}
	return output.String(), nil
}

func (s *Sandbox) cmdWC(args []string, stdin string) (string, error) {
	mode := "lines"
	var files []string
	for _, arg := range args {
		switch arg {
		case "-l":
			mode = "lines"
		case "-w":
			mode = "words"
		case "-c":
			mode = "bytes"
		default:
			if strings.HasPrefix(arg, "-") {
				return "", fmt.Errorf("unknown option %s", arg)
			}
			files = append(files, arg)
		}
	}
	inputs, err := s.readInputs(files, stdin)
	if err != nil {
		return "", err
	}
	var output strings.Builder
	for _, input := range inputs {
		var count int
		switch mode {
		case "lines":
			count = strings.Count(input.text, "\n")
			if input.text != "" && !strings.HasSuffix(input.text, "\n") {
				count++
			}
		case "words":
			count = len(strings.Fields(input.text))
		case "bytes":
			count = len([]byte(input.text))
		}
		output.WriteString(strconv.Itoa(count))
		if input.name != "" {
			output.WriteString(" " + input.name)
		}
		output.WriteByte('\n')
	}
	return output.String(), nil
}

var awkPrintField = regexp.MustCompile(`^\{\s*print\s+\$([0-9]+)\s*\}$`)

func (s *Sandbox) cmdAwk(args []string, stdin string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("missing program")
	}
	match := awkPrintField.FindStringSubmatch(args[0])
	if match == nil {
		return "", fmt.Errorf("this lab supports awk programs like '{print $3}'")
	}
	field, _ := strconv.Atoi(match[1])
	if field < 1 {
		return "", fmt.Errorf("field numbers start at 1")
	}
	inputs, err := s.readInputs(args[1:], stdin)
	if err != nil {
		return "", err
	}
	var output strings.Builder
	for _, input := range inputs {
		for _, line := range textLines(input.text) {
			fields := strings.Fields(line)
			if field <= len(fields) {
				output.WriteString(fields[field-1])
			}
			output.WriteByte('\n')
		}
	}
	return output.String(), nil
}

func (s *Sandbox) cmdCut(args []string, stdin string) (string, error) {
	delimiter := "\t"
	field := 0
	var files []string
	for index := 0; index < len(args); index++ {
		switch {
		case args[index] == "-d":
			if index+1 >= len(args) {
				return "", fmt.Errorf("-d requires a delimiter")
			}
			index++
			delimiter = args[index]
		case strings.HasPrefix(args[index], "-d") && len(args[index]) > 2:
			delimiter = strings.TrimPrefix(args[index], "-d")
		case args[index] == "-f":
			if index+1 >= len(args) {
				return "", fmt.Errorf("-f requires a field")
			}
			index++
			field, _ = strconv.Atoi(args[index])
		case strings.HasPrefix(args[index], "-f"):
			field, _ = strconv.Atoi(strings.TrimPrefix(args[index], "-f"))
		default:
			if strings.HasPrefix(args[index], "-") {
				return "", fmt.Errorf("unknown option %s", args[index])
			}
			files = append(files, args[index])
		}
	}
	if field < 1 {
		return "", fmt.Errorf("select a positive field with -f")
	}
	if delimiter == "" {
		return "", fmt.Errorf("delimiter cannot be empty")
	}
	inputs, err := s.readInputs(files, stdin)
	if err != nil {
		return "", err
	}
	var output strings.Builder
	for _, input := range inputs {
		for _, line := range textLines(input.text) {
			fields := strings.Split(line, delimiter)
			if field <= len(fields) {
				output.WriteString(fields[field-1])
			}
			output.WriteByte('\n')
		}
	}
	return output.String(), nil
}

func cmdEcho(args []string) (string, error) {
	newline := true
	if len(args) > 0 && args[0] == "-n" {
		newline = false
		args = args[1:]
	}
	result := strings.Join(args, " ")
	if newline {
		result += "\n"
	}
	return result, nil
}

func cmdPrintf(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("missing format string")
	}
	format := strings.NewReplacer(`\n`, "\n", `\t`, "\t", `\\`, "\\").Replace(args[0])
	values := args[1:]
	var output strings.Builder
	valueIndex := 0
	for index := 0; index < len(format); index++ {
		if format[index] == '%' && index+1 < len(format) {
			next := format[index+1]
			if next == '%' {
				output.WriteByte('%')
				index++
				continue
			}
			if next == 's' {
				if valueIndex < len(values) {
					output.WriteString(values[valueIndex])
					valueIndex++
				}
				index++
				continue
			}
		}
		output.WriteByte(format[index])
	}
	return output.String(), nil
}

func textLines(text string) []string {
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func joinOutputLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func uniqueLines(lines []string) []string {
	if len(lines) == 0 {
		return lines
	}
	result := []string{lines[0]}
	for _, line := range lines[1:] {
		if line != result[len(result)-1] {
			result = append(result, line)
		}
	}
	return result
}
