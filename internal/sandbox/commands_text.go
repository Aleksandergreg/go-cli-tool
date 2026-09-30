package sandbox

import (
	"fmt"
	"slices"
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
	var output commandOutputBuffer
	for _, input := range inputs {
		output.WriteString(input.text)
	}
	return output.Result()
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
	var output commandOutputBuffer
	for index, input := range inputs {
		if len(inputs) > 1 {
			if index > 0 {
				output.WriteByte('\n')
			}
			output.WriteString("==> " + input.name + " <==\n")
		}
		lines := textLines(input.text)
		limit := min(count, len(lines))
		if head {
			lines = lines[:limit]
		} else {
			lines = lines[len(lines)-limit:]
		}
		if len(lines) > 0 {
			output.WriteString(strings.Join(lines, "\n") + "\n")
		}
	}
	return output.Result()
}

func (s *Sandbox) cmdSort(args []string, stdin string) (string, error) {
	options, files, err := parseShortOptions(args, "run", false)
	if err != nil {
		return "", err
	}
	reverse := strings.ContainsRune(options, 'r')
	unique := strings.ContainsRune(options, 'u')
	numeric := strings.ContainsRune(options, 'n')
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
		slices.Reverse(lines)
	}
	if unique {
		lines = slices.Compact(lines)
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
	var output commandOutputBuffer
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
	return output.Result()
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
	var output commandOutputBuffer
	for _, input := range inputs {
		var count int
		switch mode {
		case "lines":
			count = strings.Count(input.text, "\n")
		case "words":
			count = len(strings.Fields(input.text))
		case "bytes":
			count = len(input.text)
		}
		output.WriteString(strconv.Itoa(count))
		if input.name != "" {
			output.WriteString(" " + input.name)
		}
		output.WriteByte('\n')
	}
	return output.Result()
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
	var output commandOutputBuffer
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
	return output.Result()
}

func textLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

func joinOutputLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}
