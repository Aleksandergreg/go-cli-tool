package sandbox

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
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
	return selectField(inputs, field, func(line string) []string { return strings.Split(line, delimiter) })
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
