package mission

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	missionIDPattern   = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	commandNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	variablePattern    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// validateMission checks one decoded mission in a fixed order, so the first
// reported problem is stable.
func validateMission(item Mission) error {
	for _, check := range []func(Mission) error{validateIdentity, validateGuidance, validateLab, validateOutcomes} {
		if err := check(item); err != nil {
			return err
		}
	}
	return nil
}

// validateIdentity checks placement, narrative, difficulty, and rewards.
func validateIdentity(item Mission) error {
	if !missionIDPattern.MatchString(item.ID) {
		return fmt.Errorf("id %q must use lowercase words separated by hyphens", item.ID)
	}
	track := item.EffectiveTrack()
	switch track {
	case TrackLinux, TrackDocker:
	default:
		return fmt.Errorf("unknown track %q", track)
	}
	environment := item.EffectiveEnvironment()
	switch environment {
	case EnvironmentSimulated, EnvironmentDocker:
	default:
		return fmt.Errorf("unknown environment %q", environment)
	}
	if (track == TrackDocker) != (environment == EnvironmentDocker) {
		return fmt.Errorf("track %q cannot use environment %q", track, environment)
	}
	for name, value := range map[string]string{
		"title": item.Title, "campaign": item.Campaign, "difficulty": item.Difficulty,
		"story": item.Story, "objective": item.Objective, "explanation": item.Explanation,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if item.Number < 1 {
		return fmt.Errorf("number must be positive")
	}
	switch item.Difficulty {
	case DifficultyBeginner, DifficultyIntermediate, DifficultyAdvanced:
	default:
		return fmt.Errorf("unknown difficulty %q", item.Difficulty)
	}
	if item.Rewards.XP <= 0 || item.Rewards.HintPenalty < 0 {
		return fmt.Errorf("XP must be positive and hint penalty cannot be negative")
	}
	return nil
}

// validateGuidance checks suggested commands and hints.
func validateGuidance(item Mission) error {
	if len(item.SuggestedCommands) == 0 {
		return fmt.Errorf("at least one suggested command is required")
	}
	seenCommands := make(map[string]bool, len(item.SuggestedCommands))
	for index, command := range item.SuggestedCommands {
		if !commandNamePattern.MatchString(command) {
			return fmt.Errorf("suggested command %d %q must be a lowercase command name", index+1, command)
		}
		if seenCommands[command] {
			return fmt.Errorf("duplicate suggested command %q", command)
		}
		seenCommands[command] = true
	}
	if len(item.Hints) < 1 || len(item.Hints) > 5 {
		return fmt.Errorf("missions require between 1 and 5 hints")
	}
	for index, hint := range item.Hints {
		if strings.TrimSpace(hint) == "" {
			return fmt.Errorf("hint %d is empty", index+1)
		}
	}
	return nil
}

// validateLab checks that the setup matches the mission's environment:
// simulated missions declare a virtual lab, Docker missions a Docker setup.
func validateLab(item Mission) error {
	if item.EffectiveEnvironment() == EnvironmentSimulated {
		if item.Docker != nil {
			return fmt.Errorf("simulated mission cannot define docker setup")
		}
		if err := validateAbsolutePath("start_dir", item.StartDir, true); err != nil {
			return err
		}
		return validateSetup(item.Setup, item.StartDir)
	}
	if item.StartDir != "" {
		return fmt.Errorf("docker mission cannot define start_dir")
	}
	if !setupEmpty(item.Setup) {
		return fmt.Errorf("docker mission cannot define simulated setup")
	}
	if item.Docker == nil {
		return fmt.Errorf("docker mission requires docker setup")
	}
	return ValidateDockerSetup(*item.Docker)
}

// validateOutcomes checks every condition and that each container or network
// it names is declared by the setup.
func validateOutcomes(item Mission) error {
	if len(item.Validation.All) == 0 {
		return fmt.Errorf("at least one validation condition is required")
	}
	for index, condition := range item.Validation.All {
		if err := validateCondition(condition, item.EffectiveEnvironment()); err != nil {
			return fmt.Errorf("validation condition %d: %w", index+1, err)
		}
		if err := validateConditionReferences(condition, item.Docker); err != nil {
			return fmt.Errorf("validation condition %d: %w", index+1, err)
		}
	}
	return nil
}

func validateConditionReferences(condition Condition, setup *DockerSetup) error {
	for _, container := range append([]string{condition.Container}, condition.Containers...) {
		if container != "" && !dockerSetupHasContainer(setup, container) {
			return fmt.Errorf("unknown docker container %q", container)
		}
	}
	if condition.Network != "" && !dockerSetupHasNetwork(setup, condition.Network) {
		return fmt.Errorf("unknown docker network %q", condition.Network)
	}
	return nil
}
