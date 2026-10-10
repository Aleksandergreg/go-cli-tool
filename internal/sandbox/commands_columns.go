package sandbox

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

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
	return selectField(inputs, field, strings.Fields)
}

func (s *Sandbox) cmdCut(args []string, stdin string) (string, error) {
	delimiter := "\t"
	fieldList := ""
	onlyDelimited := false
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
				return "", fmt.Errorf("-f requires a field list")
			}
			index++
			fieldList = args[index]
		case strings.HasPrefix(args[index], "-f"):
			fieldList = strings.TrimPrefix(args[index], "-f")
		case args[index] == "-s":
			onlyDelimited = true
		default:
			if strings.HasPrefix(args[index], "-") {
				return "", fmt.Errorf("unknown option %s", args[index])
			}
			files = append(files, args[index])
		}
	}
	if fieldList == "" {
		return "", fmt.Errorf("select fields with -f, such as -f 2, -f 1,3, or -f 2-")
	}
	fields, err := parseFieldList(fieldList)
	if err != nil {
		return "", err
	}
	if utf8.RuneCountInString(delimiter) != 1 {
		return "", fmt.Errorf("the delimiter must be a single character")
	}
	inputs, err := s.readInputs(files, stdin)
	if err != nil {
		return "", err
	}
	var output commandOutputBuffer
	for _, input := range inputs {
		for _, line := range textLines(input.text) {
			// Like GNU cut, a line without the delimiter passes through whole
			// unless -s asks for delimited lines only.
			if !strings.Contains(line, delimiter) {
				if !onlyDelimited {
					output.WriteString(line + "\n")
				}
				continue
			}
			var selected []string
			for position, field := range strings.Split(line, delimiter) {
				if fields.contains(position + 1) {
					selected = append(selected, field)
				}
			}
			output.WriteString(strings.Join(selected, delimiter) + "\n")
		}
	}
	return output.Result()
}

// fieldRange is an inclusive range of 1-based fields; high 0 is open-ended.
type fieldRange struct{ low, high int }

type fieldList []fieldRange

func (l fieldList) contains(field int) bool {
	for _, item := range l {
		if field >= item.low && (item.high == 0 || field <= item.high) {
			return true
		}
	}
	return false
}

// parseFieldList parses cut's LIST: comma-separated N, N-M, N-, or -M.
// Ranges are kept as bounds, so -f 1-999999999 allocates nothing per field.
func parseFieldList(list string) (fieldList, error) {
	invalid := fmt.Errorf("invalid field list %q; use N, N-M, N-, or -M separated by commas", list)
	number := func(value string) (int, error) {
		if value == "" || strings.Trim(value, "0123456789") != "" {
			return 0, invalid
		}
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return 0, invalid
		}
		if parsed < 1 {
			return 0, fmt.Errorf("fields are numbered from 1")
		}
		return parsed, nil
	}
	var fields fieldList
	for _, item := range strings.Split(list, ",") {
		lowText, highText, isRange := strings.Cut(item, "-")
		if !isRange {
			field, err := number(item)
			if err != nil {
				return nil, err
			}
			fields = append(fields, fieldRange{low: field, high: field})
			continue
		}
		if lowText == "" && highText == "" {
			return nil, invalid
		}
		bounds := fieldRange{low: 1}
		var err error
		if lowText != "" {
			if bounds.low, err = number(lowText); err != nil {
				return nil, err
			}
		}
		if highText != "" {
			if bounds.high, err = number(highText); err != nil {
				return nil, err
			}
			if bounds.high < bounds.low {
				return nil, fmt.Errorf("invalid decreasing range %q in field list", item)
			}
		}
		fields = append(fields, bounds)
	}
	return fields, nil
}

func selectField(inputs []namedText, field int, split func(string) []string) (string, error) {
	var output commandOutputBuffer
	for _, input := range inputs {
		for _, line := range textLines(input.text) {
			fields := split(line)
			if field <= len(fields) {
				output.WriteString(fields[field-1])
			}
			output.WriteByte('\n')
		}
	}
	return output.Result()
}
