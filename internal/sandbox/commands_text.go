package sandbox

import (
	"fmt"
	"regexp"
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
	files, err := operandsOnly(args)
	if err != nil {
		return "", err
	}
	inputs, err := s.readInputs(files, stdin)
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
	// fromStart selects GNU's signed forms: tail -n +N starts at line N and
	// head -n -N prints everything except the last N lines.
	fromStart := false
	var files []string
	setCount := func(value string) error {
		sign := byte(0)
		if value != "" && (value[0] == '+' || value[0] == '-') {
			sign = value[0]
			value = value[1:]
		}
		number, err := strconv.Atoi(value)
		if err != nil || number < 0 || strings.ContainsAny(value, "+-") {
			return fmt.Errorf("invalid line count %q", string(sign)+value)
		}
		count = number
		fromStart = head && sign == '-' || !head && sign == '+'
		return nil
	}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "-n":
			if index+1 >= len(args) {
				return "", fmt.Errorf("-n requires a line count")
			}
			index++
			if err := setCount(args[index]); err != nil {
				return "", err
			}
		case strings.HasPrefix(arg, "-n"):
			if err := setCount(strings.TrimPrefix(arg, "-n")); err != nil {
				return "", err
			}
		case strings.HasPrefix(arg, "-") && len(arg) > 1:
			value, err := strconv.Atoi(strings.TrimPrefix(arg, "-"))
			if err != nil || value < 0 {
				return "", fmt.Errorf("unknown option %s", arg)
			}
			count = value
			fromStart = false
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
		switch {
		case head && fromStart:
			lines = lines[:len(lines)-min(count, len(lines))]
		case head:
			lines = lines[:min(count, len(lines))]
		case fromStart:
			lines = lines[min(max(count-1, 0), len(lines)):]
		default:
			lines = lines[len(lines)-min(count, len(lines)):]
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
	if unique && numeric {
		// Like GNU sort, -nu keeps the first line of each equal numeric key.
		lines = slices.CompactFunc(lines, func(left, right string) bool {
			return leadingNumber(left) == leadingNumber(right)
		})
	} else if unique {
		lines = slices.Compact(lines)
	}
	return joinOutputLines(lines), nil
}

// leadingNumber reads the numeric prefix GNU sort -n compares: optional
// leading blanks, an optional minus sign, digits, and an optional decimal
// fraction. Text after the prefix is ignored, so "10ms" sorts as 10 and a
// line without a numeric prefix sorts as 0.
func leadingNumber(value string) float64 {
	value = strings.TrimLeft(value, " \t")
	end := 0
	if end < len(value) && value[end] == '-' {
		end++
	}
	digits := 0
	for end < len(value) && value[end] >= '0' && value[end] <= '9' {
		end++
		digits++
	}
	if end < len(value) && value[end] == '.' {
		end++
		for end < len(value) && value[end] >= '0' && value[end] <= '9' {
			end++
			digits++
		}
	}
	if digits == 0 {
		return 0
	}
	number, _ := strconv.ParseFloat(strings.TrimSuffix(value[:end], "."), 64)
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
	options, files, err := parseShortOptions(args, "lwc", false)
	if err != nil {
		return "", err
	}
	if options == "" {
		options = "lwc"
	}
	selected := []bool{strings.ContainsRune(options, 'l'), strings.ContainsRune(options, 'w'), strings.ContainsRune(options, 'c')}
	inputs, err := s.readInputs(files, stdin)
	if err != nil {
		return "", err
	}
	rows := make([]wcRow, 0, len(inputs)+1)
	total := wcRow{name: "total"}
	for _, input := range inputs {
		row := wcRow{name: input.name, counts: [3]int{strings.Count(input.text, "\n"), len(strings.Fields(input.text)), len(input.text)}}
		for index := range total.counts {
			total.counts[index] += row.counts[index]
		}
		rows = append(rows, row)
	}
	if len(rows) > 1 {
		rows = append(rows, total)
	}

	// GNU wc right-aligns every count to one width. A single count for a
	// single input is unpadded; otherwise the width fits the total byte count
	// of named files, with a minimum of 7 when reading standard input.
	selectedCount := 0
	for _, enabled := range selected {
		if enabled {
			selectedCount++
		}
	}
	width := 1
	if selectedCount > 1 || len(inputs) > 1 {
		width = len(strconv.Itoa(total.counts[2]))
		if len(files) == 0 {
			width = max(width, 7)
		}
	}
	var output commandOutputBuffer
	for _, row := range rows {
		separator := ""
		for index, enabled := range selected {
			if enabled {
				output.WriteString(fmt.Sprintf("%s%*d", separator, width, row.counts[index]))
				separator = " "
			}
		}
		if row.name != "" {
			output.WriteString(" " + row.name)
		}
		output.WriteByte('\n')
	}
	return output.Result()
}

type wcRow struct {
	name   string
	counts [3]int
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

// maxPrintfField bounds printf widths and precisions so a directive such as
// %999999999s fails before formatting allocates beyond the output budget.
const maxPrintfField = 4096

var printfSpecPattern = regexp.MustCompile(`^[-+0]*([0-9]*)(?:\.([0-9]*))?$`)

func cmdPrintf(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("missing format string")
	}
	format := strings.NewReplacer(`\n`, "\n", `\t`, "\t", `\\`, "\\").Replace(args[0])
	values := args[1:]
	var output commandOutputBuffer
	// Like POSIX printf, the format is reused while values remain, so
	// printf '%s\n' a b c prints three lines.
	for {
		consumed, err := writePrintfFormat(&output, format, values)
		if err != nil {
			return "", err
		}
		values = values[consumed:]
		if consumed == 0 || len(values) == 0 {
			break
		}
	}
	return output.Result()
}

func writePrintfFormat(output *commandOutputBuffer, format string, values []string) (int, error) {
	used := 0
	nextValue := func() (string, bool) {
		if used >= len(values) {
			return "", false
		}
		used++
		return values[used-1], true
	}
	for index := 0; index < len(format); index++ {
		if format[index] != '%' {
			output.WriteByte(format[index])
			continue
		}
		end := index + 1
		for end < len(format) && strings.IndexByte("-+0123456789.", format[end]) >= 0 {
			end++
		}
		if end >= len(format) {
			return used, fmt.Errorf("missing conversion character after %q", format[index:])
		}
		spec, verb := format[index+1:end], format[end]
		directive := format[index : end+1]
		index = end
		if verb == '%' && spec == "" {
			output.WriteByte('%')
			continue
		}
		match := printfSpecPattern.FindStringSubmatch(spec)
		if verb != 's' && verb != 'd' && verb != 'i' || match == nil {
			return used, fmt.Errorf("unsupported directive %q; use %%s, %%d, or %%%% with optional - or 0 flags, width, and precision", directive)
		}
		for _, field := range match[1:] {
			if size, _ := strconv.Atoi(field); size > maxPrintfField {
				return used, fmt.Errorf("%q exceeds the %d-character field limit", directive, maxPrintfField)
			}
		}
		value, _ := nextValue()
		if verb == 's' {
			output.WriteString(fmt.Sprintf("%"+spec+"s", value))
			continue
		}
		number := int64(0)
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			parsed, err := strconv.ParseInt(trimmed, 10, 64)
			if err != nil {
				return used, fmt.Errorf("%q: invalid number", value)
			}
			number = parsed
		}
		output.WriteString(fmt.Sprintf("%"+spec+"d", number))
	}
	return used, output.Err()
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
