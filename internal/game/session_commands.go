package game

import "fmt"

// revealHint shows the next hint and, on a first attempt, records it so the
// reduced reward survives a pause.
func (a *attempt) revealHint() error {
	if a.hintsUsed >= len(a.Mission.Hints) {
		fmt.Fprintln(a.Out, a.Style.Warning("No more hints. ByteWorks has exhausted its documentation budget."))
		return nil
	}
	cost := 0
	if a.replaying {
		a.hintsUsed++
	} else {
		before := AdjustedReward(a.Mission, a.hintsUsed)
		a.hintsUsed = a.Player.RecordHint(a.Mission.ID)
		if err := a.Saver.Save(*a.Player); err != nil {
			return err
		}
		cost = before - AdjustedReward(a.Mission, a.hintsUsed)
	}
	costLabel := fmt.Sprintf("-%d XP", cost)
	if cost == 0 {
		costLabel = "no XP cost"
	}
	a.report(AttemptHint, AttemptStateActive, nil)
	if a.Companion {
		message := fmt.Sprintf("Hint %d/%d revealed in the web companion (%s).", a.hintsUsed, len(a.Mission.Hints), costLabel)
		fmt.Fprintln(a.Out, a.Style.Warning(message))
	} else {
		prefix := fmt.Sprintf("Hint %d/%d (%s):", a.hintsUsed, len(a.Mission.Hints), costLabel)
		fmt.Fprintf(a.Out, "%s %s\n", a.Style.Warning(prefix), a.Mission.Hints[a.hintsUsed-1])
	}
	return nil
}

func (a *attempt) showObjective() {
	if a.Companion {
		fmt.Fprintln(a.Out, a.Style.Accent("The objective and suggested commands are shown in the web companion."))
		return
	}
	fmt.Fprintf(a.Out, "%s\n%s\n\n%s\n", a.Style.Section("OBJECTIVE"), a.Mission.Objective, a.Style.CommandGuide(a.Mission.SuggestedCommands))
}

func (a *attempt) refreshStatus() error {
	outcomes, err := evaluateOutcomes(a.ctx, a.Mission.Validation, a.environment.Environment, a.lastOutput)
	if err != nil {
		return fmt.Errorf("check mission status: %w", err)
	}
	a.outcomes = outcomes
	a.report(AttemptProgress, AttemptStateActive, nil)
	if a.Companion {
		fmt.Fprintln(a.Out, a.Style.Accent("Mission status refreshed in the web companion."))
	} else {
		printOutcomeStatus(a.Out, outcomes, a.Style)
	}
	return nil
}

// restart replaces the environment with a fresh one. Hints and command
// practice belong to the player, so they are kept.
func (a *attempt) restart() error {
	if err := a.environment.close(); err != nil {
		return fmt.Errorf("restart mission: close current environment: %w", err)
	}
	environment, err := createManagedEnvironment(a.ctx, a.factory, a.Mission)
	a.environment = environment
	if err != nil {
		return fmt.Errorf("restart mission: %w", err)
	}
	a.lastOutput = ""
	if a.Reporter != nil {
		outcomes, err := evaluateOutcomes(a.ctx, a.Mission.Validation, a.environment.Environment, "")
		if err != nil {
			return fmt.Errorf("refresh companion mission status: %w", err)
		}
		a.outcomes = outcomes
		a.report(AttemptRestarted, AttemptStateActive, nil)
	}
	fmt.Fprintln(a.Out, a.Style.Accent("Mission environment restarted. Hints and command mastery are retained."))
	return nil
}

func (a *attempt) showControls() {
	if a.Companion {
		fmt.Fprintln(a.Out, a.Style.Accent("Mission controls and guidance are shown in the web companion."))
		return
	}
	printMissionControls(a.Out, a.Style)
}
