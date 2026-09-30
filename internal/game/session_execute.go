package game

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/aleksandergregersen/opsquest/internal/profile"
)

// execute runs one command in the mission environment, records practice, and
// completes the mission once every outcome holds.
func (a *attempt) execute(line string) (step, error) {
	result, ran, err := a.runCommand(line)
	if err != nil || !ran {
		return step{}, err
	}
	if result.Output != "" {
		fmt.Fprint(a.Out, result.Output)
		if !strings.HasSuffix(result.Output, "\n") {
			fmt.Fprintln(a.Out)
		}
	}
	a.lastOutput = result.Output
	unlocked, err := a.recordPractice(result)
	if err != nil {
		return step{}, err
	}
	outcomes, err := evaluateOutcomes(a.ctx, a.Mission.Validation, a.environment.Environment, result.Output)
	if err != nil {
		return step{}, fmt.Errorf("validate mission: %w", err)
	}
	a.outcomes = outcomes
	a.report(AttemptProgress, AttemptStateActive, unlocked)
	if !allOutcomesSatisfied(outcomes) {
		printAchievements(a.Out, unlocked, a.Style)
		if !a.Companion {
			message := fmt.Sprintf("Progress — %d/%d outcome checks satisfied. Type status to see what remains.", satisfiedOutcomeCount(outcomes), len(outcomes))
			fmt.Fprintln(a.Out, a.Style.Accent(message))
		}
		return step{}, nil
	}
	completed, err := a.complete(unlocked)
	if err != nil {
		return step{}, err
	}
	return finish(completed), nil
}

// runCommand executes line and any interactive follow-up. ran is false when
// a recoverable command error was already shown to the player.
func (a *attempt) runCommand(line string) (Execution, bool, error) {
	result, executeErr := a.environment.Execute(a.ctx, line)
	if executeErr != nil {
		if err := a.ctx.Err(); err != nil {
			return Execution{}, false, fmt.Errorf("execute command: %w", err)
		}
		a.fail(executeErr.Error())
		return Execution{}, false, nil
	}
	if err := a.ctx.Err(); err != nil {
		return Execution{}, false, fmt.Errorf("execute command: %w", err)
	}
	if result.Interactive == nil {
		return result, true, nil
	}
	if result.Interactive.Run == nil {
		return Execution{}, false, fmt.Errorf("run %s: interactive action has no runner", result.Interactive.Command)
	}
	if err := result.Interactive.Run(a.reader); err != nil {
		if errors.Is(err, ErrInteractiveEditor) || errors.Is(err, ErrUnsupportedEditorFile) {
			a.fail(fmt.Sprintf("%s: %v", result.Interactive.Command, err))
			return Execution{}, false, nil
		}
		return Execution{}, false, fmt.Errorf("run %s: %w", result.Interactive.Command, err)
	}
	return result, true, nil
}

// recordPractice credits the commands a successful execution used, unlocks
// command achievements, and saves the profile.
func (a *attempt) recordPractice(result Execution) ([]profile.Achievement, error) {
	for _, command := range result.PracticedCommands {
		if !slices.Contains(a.practiced, command) {
			a.practiced = append(a.practiced, command)
		}
		if a.Player.Commands[command] == 0 && !slices.Contains(a.discovered, command) {
			a.discovered = append(a.discovered, command)
		}
	}
	a.Player.RecordCommands(result.PracticedCommands)
	unlocked := make([]profile.Achievement, 0)
	achievementTime := a.Now()
	if result.PipelineWidth >= 3 {
		if achievement, added := a.Player.UnlockAchievement(profile.AchievementPipeDream, achievementTime); added {
			unlocked = append(unlocked, achievement)
		}
	}
	unlocked = append(unlocked, ReconcileCommandAchievements(a.Player, achievementTime)...)
	if err := a.Saver.Save(*a.Player); err != nil {
		return nil, err
	}
	return unlocked, nil
}

// complete closes the environment before awarding XP, so an attempt whose
// cleanup failed is never recorded as complete.
func (a *attempt) complete(unlocked []profile.Achievement) (SessionResult, error) {
	if err := a.environment.close(); err != nil {
		return SessionResult{}, fmt.Errorf("complete mission: close environment before awarding XP: %w", err)
	}
	xp := AdjustedReward(a.Mission, a.hintsUsed)
	completedAt := a.Now()
	firstCompletion := a.Player.Complete(a.Mission.ID, xp, a.hintsUsed, completedAt)
	if !firstCompletion {
		xp = 0
	} else {
		unlocked = append(unlocked, ReconcileAchievements(a.Player, a.Catalog, completedAt)...)
	}
	if err := a.Saver.Save(*a.Player); err != nil {
		return SessionResult{}, err
	}
	a.reportAttempt(AttemptCompleted, AttemptStateCompleted, a.outcomes, a.hintsUsed, a.replaying, xp, firstCompletion, a.practiced, a.discovered, unlocked)
	if a.Companion {
		printCompanionCompletion(a.Out, xp, firstCompletion, unlocked, a.Style)
	} else {
		printCompletion(a.Out, a.Mission, xp, firstCompletion, a.practiced, a.discovered, unlocked, a.Style)
	}
	return SessionResult{Completed: true, XPAwarded: xp, HintsUsed: a.hintsUsed, WorldRoute: a.worldRoute}, nil
}
