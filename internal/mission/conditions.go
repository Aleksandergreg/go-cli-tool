package mission

import (
	"encoding/json"
	"fmt"
	"strings"
)

type conditionFields uint8

const (
	conditionPath conditionFields = 1 << iota
	conditionValue
	conditionValues
	conditionPID
	conditionContainer
	conditionCount
	conditionNetwork
	conditionContainers
)

var allowedConditionFields = map[ConditionType]conditionFields{
	ConditionOutputEquals:              conditionValue,
	ConditionOutputContains:            conditionValue,
	ConditionOutputContainsAll:         conditionValues,
	ConditionOutputNotContains:         conditionValue,
	ConditionCWDEquals:                 conditionValue,
	ConditionFileExists:                conditionPath,
	ConditionDirectoryExists:           conditionPath,
	ConditionPathMissing:               conditionPath,
	ConditionFileContentEquals:         conditionPath | conditionValue,
	ConditionFileContentContains:       conditionPath | conditionValue,
	ConditionFileLinesEqual:            conditionPath | conditionValues,
	ConditionFileModeEquals:            conditionPath | conditionValue,
	ConditionFileOwnerEquals:           conditionPath | conditionValue,
	ConditionProcessStopped:            conditionPID,
	ConditionProcessRunning:            conditionPID,
	ConditionEnvironmentEquals:         conditionValue,
	ConditionDockerContainerRunning:    conditionContainer,
	ConditionDockerContainerStopped:    conditionContainer,
	ConditionDockerContainerCountEqual: conditionCount,
	ConditionDockerContainerAbsent:     conditionContainer,
	ConditionDockerNetworkShared:       conditionContainers,
	ConditionDockerNetworkIsolated:     conditionContainers,
	ConditionDockerNetworkAbsent:       conditionNetwork,
}

var conditionFieldFlags = map[string]conditionFields{
	"path":       conditionPath,
	"value":      conditionValue,
	"values":     conditionValues,
	"pid":        conditionPID,
	"container":  conditionContainer,
	"count":      conditionCount,
	"network":    conditionNetwork,
	"containers": conditionContainers,
}

func validateCondition(condition Condition, environment string) error {
	allowed, known := allowedConditionFields[condition.Type]
	if !known {
		return fmt.Errorf("unknown type %q", condition.Type)
	}
	if err := checkConditionFields(condition, allowed); err != nil {
		return err
	}
	if allowed&conditionPath != 0 {
		if environment != EnvironmentSimulated {
			return fmt.Errorf("type %q requires a simulated environment", condition.Type)
		}
		if err := validateAbsolutePath(string(condition.Type), condition.Path, condition.Type == ConditionDirectoryExists); err != nil {
			return err
		}
	}
	return validateConditionValues(condition, environment)
}

// checkConditionFields rejects any field, even an explicit zero value in
// JSON, that the condition type does not support.
func checkConditionFields(condition Condition, allowed conditionFields) error {
	present := condition.present
	for _, field := range []struct {
		name    string
		present bool
		flag    conditionFields
	}{
		{name: "path", present: condition.Path != "", flag: conditionPath},
		{name: "value", present: condition.Value != "", flag: conditionValue},
		{name: "values", present: len(condition.Values) != 0, flag: conditionValues},
		{name: "pid", present: condition.PID != 0, flag: conditionPID},
		{name: "container", present: condition.Container != "", flag: conditionContainer},
		{name: "count", present: condition.Count != nil, flag: conditionCount},
		{name: "network", present: condition.Network != "", flag: conditionNetwork},
		{name: "containers", present: len(condition.Containers) != 0, flag: conditionContainers},
	} {
		if field.present {
			present |= field.flag
		}
		if present&field.flag != 0 && allowed&field.flag == 0 {
			return fmt.Errorf("type %q does not support %s", condition.Type, field.name)
		}
	}
	return nil
}

// validateConditionValues applies each condition type's own value and
// environment rules.
func validateConditionValues(condition Condition, environment string) error {
	switch condition.Type {
	case ConditionOutputEquals, ConditionFileExists, ConditionDirectoryExists, ConditionPathMissing, ConditionFileContentEquals:
		return nil
	case ConditionOutputContains, ConditionOutputNotContains, ConditionFileContentContains:
		if condition.Value == "" {
			return fmt.Errorf("value cannot be empty")
		}
	case ConditionOutputContainsAll:
		if len(condition.Values) == 0 {
			return fmt.Errorf("values cannot be empty")
		}
		for _, value := range condition.Values {
			if value == "" {
				return fmt.Errorf("values cannot contain an empty string")
			}
		}
	case ConditionCWDEquals:
		if environment != EnvironmentSimulated {
			return fmt.Errorf("cwd_equals requires a simulated environment")
		}
		return validateAbsolutePath("cwd_equals", condition.Value, true)
	case ConditionFileLinesEqual:
		if len(condition.Values) == 0 {
			return fmt.Errorf("values cannot be empty")
		}
		for _, value := range condition.Values {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("line values cannot be empty")
			}
		}
	case ConditionFileModeEquals:
		return validateMode(condition.Value)
	case ConditionFileOwnerEquals:
		if strings.TrimSpace(condition.Value) == "" {
			return fmt.Errorf("owner cannot be empty")
		}
	case ConditionProcessStopped, ConditionProcessRunning:
		if environment != EnvironmentSimulated {
			return fmt.Errorf("%s requires a simulated environment", condition.Type)
		}
		if condition.PID <= 0 {
			return fmt.Errorf("PID must be positive")
		}
	case ConditionEnvironmentEquals:
		if environment != EnvironmentSimulated {
			return fmt.Errorf("env_equals requires a simulated environment")
		}
		name, _, found := strings.Cut(condition.Value, "=")
		if !found || !variablePattern.MatchString(name) {
			return fmt.Errorf("value must be NAME=value")
		}
	case ConditionDockerContainerRunning, ConditionDockerContainerStopped, ConditionDockerContainerAbsent:
		if environment != EnvironmentDocker {
			return fmt.Errorf("%s requires a docker environment", condition.Type)
		}
		if !ValidDockerLogicalName(condition.Container) {
			return fmt.Errorf("container %q must be a lowercase logical name", condition.Container)
		}
	case ConditionDockerNetworkShared, ConditionDockerNetworkIsolated:
		if environment != EnvironmentDocker {
			return fmt.Errorf("%s requires a docker environment", condition.Type)
		}
		if len(condition.Containers) != 2 || condition.Containers[0] == condition.Containers[1] {
			return fmt.Errorf("containers must name exactly two different containers")
		}
		for _, container := range condition.Containers {
			if !ValidDockerLogicalName(container) {
				return fmt.Errorf("container %q must be a lowercase logical name", container)
			}
		}
	case ConditionDockerNetworkAbsent:
		if environment != EnvironmentDocker {
			return fmt.Errorf("%s requires a docker environment", condition.Type)
		}
		if !ValidDockerNetworkName(condition.Network) {
			return fmt.Errorf("network %q must be a lowercase logical name other than bridge, default, host, or none", condition.Network)
		}
	case ConditionDockerContainerCountEqual:
		if environment != EnvironmentDocker {
			return fmt.Errorf("docker_container_count_equals requires a docker environment")
		}
		if condition.Count == nil || *condition.Count < 0 {
			return fmt.Errorf("count must be a non-negative integer")
		}
	}
	return nil
}

// UnmarshalJSON retains field presence so validation can reject an unsupported
// field even when its explicit JSON value is empty or zero. Condition owns a
// custom decoder, so it also preserves the catalog's unknown-field rejection.
func (c *Condition) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	present := conditionFields(0)
	for name := range fields {
		if name == "type" {
			continue
		}
		flag, known := conditionFieldFlags[name]
		if !known {
			return fmt.Errorf("json: unknown field %q", name)
		}
		present |= flag
	}
	type wireCondition Condition
	var decoded wireCondition
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*c = Condition(decoded)
	c.present = present
	return nil
}
