package game

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/sandbox"
)

func Validate(validation mission.Validation, box *sandbox.Sandbox, output string) (bool, error) {
	for _, condition := range validation.All {
		valid, err := validateCondition(condition, box, output)
		if err != nil {
			return false, err
		}
		if !valid {
			return false, nil
		}
	}
	return true, nil
}

func validateCondition(condition mission.Condition, box *sandbox.Sandbox, output string) (bool, error) {
	switch condition.Type {
	case "output_equals":
		return normalizeText(output) == normalizeText(condition.Value), nil
	case "output_contains":
		return strings.Contains(output, condition.Value), nil
	case "output_contains_all":
		for _, value := range condition.Values {
			if !strings.Contains(output, value) {
				return false, nil
			}
		}
		return true, nil
	case "output_not_contains":
		return !strings.Contains(output, condition.Value), nil
	case "cwd_equals":
		return box.CWD == condition.Value, nil
	case "file_exists":
		entry, exists := box.FS.Entry(condition.Path)
		return exists && entry.Kind == sandbox.Regular, nil
	case "dir_exists":
		return box.FS.IsDir(condition.Path), nil
	case "path_missing":
		return !box.FS.Exists(condition.Path), nil
	case "file_content_equals":
		content, err := box.FS.ReadFile(condition.Path)
		if err != nil {
			return false, nil
		}
		return normalizeText(content) == normalizeText(condition.Value), nil
	case "file_content_contains":
		content, err := box.FS.ReadFile(condition.Path)
		if err != nil {
			return false, nil
		}
		return strings.Contains(content, condition.Value), nil
	case "file_mode_equals":
		entry, exists := box.FS.Entry(condition.Path)
		if !exists || entry.Kind != sandbox.Regular {
			return false, nil
		}
		expected, err := strconv.ParseUint(condition.Value, 8, 12)
		if err != nil {
			return false, fmt.Errorf("invalid validation mode %q", condition.Value)
		}
		return entry.Mode == uint32(expected), nil
	case "process_stopped":
		process, exists := box.Processes[condition.PID]
		return exists && !process.Running, nil
	case "process_running":
		process, exists := box.Processes[condition.PID]
		return exists && process.Running, nil
	case "env_equals":
		key, expected, found := strings.Cut(condition.Value, "=")
		if !found {
			return false, fmt.Errorf("env_equals value must be NAME=value")
		}
		actual, exists := box.Env[key]
		return exists && actual == expected, nil
	default:
		return false, fmt.Errorf("unknown validation type %q", condition.Type)
	}
}

func normalizeText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	return strings.TrimSpace(value)
}
