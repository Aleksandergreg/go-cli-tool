package game

import (
	"context"
	"fmt"
	"strings"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/sandbox"
)

type outcomeResult struct {
	Satisfied   bool
	Description string
}

func Validate(validation mission.Validation, box *sandbox.Sandbox, output string) (bool, error) {
	outcomes, err := evaluateOutcomes(context.Background(), validation, newSandboxEnvironment(box), output)
	if err != nil {
		return false, err
	}
	return allOutcomesSatisfied(outcomes), nil
}

func Progress(validation mission.Validation, box *sandbox.Sandbox, output string) (int, int, error) {
	outcomes, err := evaluateOutcomes(context.Background(), validation, newSandboxEnvironment(box), output)
	if err != nil {
		return 0, len(validation.All), err
	}
	return satisfiedOutcomeCount(outcomes), len(outcomes), nil
}

func evaluateOutcomes(ctx context.Context, validation mission.Validation, environment Environment, output string) ([]outcomeResult, error) {
	outcomes := make([]outcomeResult, 0, len(validation.All))
	for _, condition := range validation.All {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		satisfied, err := validateCondition(ctx, condition, environment, output)
		if err != nil {
			return nil, err
		}
		outcomes = append(outcomes, outcomeResult{
			Satisfied:   satisfied,
			Description: describeCondition(condition),
		})
	}
	return outcomes, nil
}

func allOutcomesSatisfied(outcomes []outcomeResult) bool {
	for _, outcome := range outcomes {
		if !outcome.Satisfied {
			return false
		}
	}
	return true
}

func satisfiedOutcomeCount(outcomes []outcomeResult) int {
	count := 0
	for _, outcome := range outcomes {
		if outcome.Satisfied {
			count++
		}
	}
	return count
}

func describeCondition(condition mission.Condition) string {
	switch condition.Type {
	case mission.ConditionOutputEquals:
		return "Command output matches the required result"
	case mission.ConditionOutputContains:
		return fmt.Sprintf("Output contains %q", condition.Value)
	case mission.ConditionOutputContainsAll:
		return "Output contains every required match"
	case mission.ConditionOutputNotContains:
		return fmt.Sprintf("Output excludes %q", condition.Value)
	case mission.ConditionCWDEquals:
		return fmt.Sprintf("Current directory is %s", condition.Value)
	case mission.ConditionFileExists:
		return fmt.Sprintf("File exists: %s", condition.Path)
	case mission.ConditionDirectoryExists:
		return fmt.Sprintf("Directory exists: %s", condition.Path)
	case mission.ConditionPathMissing:
		return fmt.Sprintf("Path no longer exists: %s", condition.Path)
	case mission.ConditionFileContentEquals:
		return fmt.Sprintf("File has the required content: %s", condition.Path)
	case mission.ConditionFileContentContains:
		return fmt.Sprintf("File contains the required text: %s", condition.Path)
	case mission.ConditionFileLinesEqual:
		return fmt.Sprintf("File has the required lines: %s", condition.Path)
	case mission.ConditionFileModeEquals:
		return fmt.Sprintf("File mode is %s: %s", condition.Value, condition.Path)
	case mission.ConditionFileOwnerEquals:
		return fmt.Sprintf("File owner is %s: %s", condition.Value, condition.Path)
	case mission.ConditionProcessStopped:
		return fmt.Sprintf("Process %d is stopped", condition.PID)
	case mission.ConditionProcessRunning:
		return fmt.Sprintf("Process %d is still running", condition.PID)
	case mission.ConditionEnvironmentEquals:
		return fmt.Sprintf("Environment contains %s", condition.Value)
	case mission.ConditionDockerContainerRunning:
		return fmt.Sprintf("Container %s is running", condition.Container)
	case mission.ConditionDockerContainerStopped:
		return fmt.Sprintf("Container %s is stopped", condition.Container)
	case mission.ConditionDockerContainerCountEqual:
		if condition.Count != nil {
			return fmt.Sprintf("Exactly %d mission containers exist", *condition.Count)
		}
		return "The required number of mission containers exist"
	default:
		return fmt.Sprintf("Outcome condition %s is satisfied", condition.Type)
	}
}

func validateCondition(ctx context.Context, condition mission.Condition, environment Environment, output string) (bool, error) {
	switch condition.Type {
	case mission.ConditionOutputEquals:
		return normalizeText(output) == normalizeText(condition.Value), nil
	case mission.ConditionOutputContains:
		return strings.Contains(output, condition.Value), nil
	case mission.ConditionOutputContainsAll:
		for _, value := range condition.Values {
			if !strings.Contains(output, value) {
				return false, nil
			}
		}
		return true, nil
	case mission.ConditionOutputNotContains:
		return !strings.Contains(output, condition.Value), nil
	default:
		if environment == nil {
			return false, fmt.Errorf("validation type %q requires an environment observer", condition.Type)
		}
		return environment.Observe(ctx, condition)
	}
}

func normalizeText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	return strings.TrimSpace(value)
}

func normalizedLines(value string) []string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	lines := strings.Split(value, "\n")
	for index := range lines {
		lines[index] = strings.Join(strings.Fields(lines[index]), " ")
	}
	return lines
}
