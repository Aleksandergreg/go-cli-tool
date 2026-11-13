package cli

import (
	"fmt"

	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
)

func (a *App) runShow(args []string) error {
	flags := a.newFlagSet("show", func() { fmt.Fprintln(a.errOut, "Usage: opsquest show [MISSION]") })
	positionals, help, err := parseInterspersedFlags(flags, args)
	if help || err != nil {
		return err
	}
	if len(positionals) > 1 {
		return fmt.Errorf("usage: opsquest show [MISSION]")
	}
	player, err := a.loadPlayer()
	if err != nil {
		return err
	}
	var item mission.Mission
	var found bool
	if len(positionals) == 1 {
		item, found = a.catalog.Find(positionals[0])
		if !found {
			return fmt.Errorf("mission %q not found; run 'opsquest list'", positionals[0])
		}
	} else {
		item, found = a.catalog.NextInTrack(mission.TrackLinux, player.IsComplete)
		if !found {
			item, found = a.catalog.LastInTrack(mission.TrackLinux)
		}
	}
	if !found {
		return fmt.Errorf("no missions are available")
	}
	completion, complete := player.Completed[item.ID]
	status := "not completed"
	if complete {
		status = "completed"
	}
	hintsUsed := player.MissionHints(item.ID)
	fmt.Fprintln(a.out, a.style.Header(fmt.Sprintf("MISSION %02d: %s", item.Number, item.Title)))
	if placement, exists := a.catalog.Placement(item.ID); exists {
		fmt.Fprintf(a.out, "Track: %s · World %d/%d: %s · Stage %d/%d\n",
			trackDisplayName(placement.Track), placement.WorldNumber, placement.WorldTotal,
			a.style.World(placement.WorldName), placement.StageNumber, placement.StageTotal)
	} else {
		fmt.Fprintf(a.out, "Track: %s · Campaign: %s\n", trackDisplayName(item.EffectiveTrack()), a.style.World(item.Campaign))
	}
	reward := a.style.Reward(fmt.Sprintf("%d XP", game.AdjustedReward(item, hintsUsed)))
	if complete {
		reward = a.style.Accent(fmt.Sprintf("claimed · %d XP earned", completion.XP))
	}
	fmt.Fprintf(a.out, "Difficulty: %s · Reward: %s\n", a.style.Difficulty(item.Difficulty), reward)
	styledStatus := a.style.Accent(status)
	if complete {
		styledStatus = a.style.Success(status)
	}
	remainingHints := max(len(item.Hints)-hintsUsed, 0)
	hintStatus := fmt.Sprintf("%d used · %d remaining · %d total", hintsUsed, remainingHints, len(item.Hints))
	if complete {
		hintStatus = fmt.Sprintf("%d used on first completion · replay hints do not change XP", completion.HintsUsed)
	}
	fmt.Fprintf(a.out, "Status: %s · Outcome checks: %d · Hints: %s\n\n", styledStatus, len(item.Validation.All), hintStatus)
	fmt.Fprintf(a.out, "%s\n%s\n\n", a.style.Section("INCIDENT"), item.Story)
	fmt.Fprintf(a.out, "%s\n%s\n\n%s\n", a.style.Section("OBJECTIVE"), item.Objective, a.style.CommandGuide(item.SuggestedCommands))
	if item.EffectiveEnvironment() == mission.EnvironmentDocker {
		availability := game.EnvironmentAvailability(a.ctx, a.factory, item)
		fmt.Fprintln(a.out)
		if availability.Available {
			fmt.Fprintln(a.out, a.style.Success("Docker lab ready."))
		} else {
			fmt.Fprintln(a.out, a.style.Warning(availability.Detail))
		}
	}
	fmt.Fprintf(a.out, "\nPlay with: opsquest play %d\n", item.Number)
	return nil
}
