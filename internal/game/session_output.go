package game

import (
	"fmt"
	"io"
	"strings"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
	"github.com/aleksandergregersen/opsquest/internal/ui"
)

func printMission(out io.Writer, item mission.Mission, hintsUsed int, completed bool, catalog mission.Catalog, style ui.Style) {
	heading := fmt.Sprintf("MISSION %02d: %s", item.Number, item.Title)
	fmt.Fprintf(out, "\n%s\n", style.Header(heading))
	fmt.Fprintln(out, style.Muted(strings.Repeat("=", len(item.Title)+12)))
	if placement, found := catalog.Placement(item.ID); found {
		location := fmt.Sprintf("%s · World %d/%d: %s · Stage %d/%d",
			displayTrackName(placement.Track), placement.WorldNumber, placement.WorldTotal,
			placement.WorldName, placement.StageNumber, placement.StageTotal)
		fmt.Fprintln(out, style.World(location))
	} else {
		fmt.Fprintf(out, "Campaign: %s\n", style.World(item.Campaign))
	}
	reward := style.Reward(fmt.Sprintf("%d XP", AdjustedReward(item, hintsUsed)))
	if completed {
		reward = style.Accent(fmt.Sprintf("already claimed · %d XP base", item.Rewards.XP))
	}
	fmt.Fprintf(out, "Difficulty: %s · Reward: %s\n", style.Difficulty(item.Difficulty), reward)
	if hintsUsed > 0 {
		fmt.Fprintln(out, style.Warning(fmt.Sprintf("Hints already used: %d/%d", hintsUsed, len(item.Hints))))
	}
	fmt.Fprintf(out, "\n%s\n%s\n\n", style.Section("INCIDENT"), item.Story)
	fmt.Fprintf(out, "%s\n%s\n\n", style.Section("OBJECTIVE"), item.Objective)
	fmt.Fprintf(out, "%s\n\n", style.CommandGuide(item.SuggestedCommands))
	printCompactMissionControls(out, style)
}

func displayTrackName(track string) string {
	switch track {
	case mission.TrackDocker:
		return "Docker"
	case mission.TrackLinux:
		return "Linux"
	default:
		return track
	}
}

func printCompactMissionControls(out io.Writer, style ui.Style) {
	fmt.Fprintf(out, "Controls: %s for the full guide\n", accented(style, " · ", "hint", "objective", "status", "restart", "quit", "?"))
	fmt.Fprintf(out, "Navigate: %s\n", accented(style, " · ", "map", "world N", "play STAGE/ID", "next", "previous"))
	fmt.Fprintf(out, "Lab: %s lists commands · %s completes · arrows edit and recall history\n",
		style.Accent("help"), style.Accent("Tab"))
}

func printMissionControls(out io.Writer, style ui.Style) {
	fmt.Fprintln(out, style.Section("MISSION GUIDE"))
	fmt.Fprintf(out, "  Controls: %s\n", accented(style, ", ", "hint", "objective", "status", "restart", "quit"))
	fmt.Fprintf(out, "  Navigate: %s\n", accented(style, ", ", "map", "world N", "play STAGE/ID", "next", "previous"))
	fmt.Fprintf(out, "  Lab commands: %s or %s. Valid solutions are judged by their result.\n", style.Accent("help"), style.Accent("help COMMAND"))
	fmt.Fprintf(out, "  Line editing: arrows move and recall history; %s or %s jump across the line.\n",
		style.Accent("Home/End"), style.Accent("Ctrl-A/E"))
	fmt.Fprintf(out, "  %s move by word; %s completes; %s, %s, and %s remove text.\n",
		style.Accent("Option/Ctrl-Left/Right"), style.Accent("Tab"), style.Accent("Backspace"), style.Accent("Delete"), style.Accent("Ctrl-W"))
	fmt.Fprintln(out, "  Prefix navigation with opsquest if you prefer; both forms are equivalent inside a mission.")
}

func accented(style ui.Style, separator string, values ...string) string {
	styled := make([]string, len(values))
	for index, value := range values {
		styled[index] = style.Accent(value)
	}
	return strings.Join(styled, separator)
}

func printMissionSwitch(out io.Writer, target mission.Mission, style ui.Style) {
	message := fmt.Sprintf("Switching to Mission %02d: %s. The current mission environment will reset.", target.Number, target.Title)
	fmt.Fprintln(out, style.Accent(message))
}

func printOutcomeStatus(out io.Writer, outcomes []outcomeResult, style ui.Style) {
	message := fmt.Sprintf("Outcome checks satisfied: %d/%d.", satisfiedOutcomeCount(outcomes), len(outcomes))
	fmt.Fprintln(out, style.Accent(message))
	for _, outcome := range outcomes {
		marker := style.Progress("", "○")
		if outcome.Satisfied {
			marker = style.Progress("✓", "")
		}
		fmt.Fprintf(out, "  %s %s\n", marker, outcome.Description)
	}
}

func printCompletion(out io.Writer, item mission.Mission, xp int, first bool, practiced, discovered []string, unlocked []profile.Achievement, style ui.Style) {
	fmt.Fprintf(out, "\n%s\n", style.Success("✓ Mission complete!"))
	if first {
		fmt.Fprintln(out, style.Reward(fmt.Sprintf("+%d XP", xp)))
	} else {
		fmt.Fprintln(out, style.Accent("Replay complete — XP was already claimed."))
	}
	if len(discovered) == 1 {
		fmt.Fprintln(out, style.Accent(fmt.Sprintf("New command discovered: %s", discovered[0])))
	} else if len(discovered) > 1 {
		fmt.Fprintln(out, style.Accent(fmt.Sprintf("New commands discovered: %s", strings.Join(discovered, ", "))))
	} else if len(practiced) == 1 {
		fmt.Fprintln(out, style.Accent(fmt.Sprintf("Command practiced: %s", practiced[0])))
	} else if len(practiced) > 1 {
		fmt.Fprintln(out, style.Accent(fmt.Sprintf("Commands practiced: %s", strings.Join(practiced, ", "))))
	}
	printAchievements(out, unlocked, style)
	fmt.Fprintf(out, "\n%s\n", item.Explanation)
}

func printCompanionCompletion(out io.Writer, xp int, first bool, unlocked []profile.Achievement, style ui.Style) {
	fmt.Fprintf(out, "\n%s\n", style.Success("✓ Mission complete!"))
	if first {
		fmt.Fprintln(out, style.Reward(fmt.Sprintf("+%d XP", xp)))
	} else {
		fmt.Fprintln(out, style.Accent("Replay complete — XP was already claimed."))
	}
	printAchievements(out, unlocked, style)
	fmt.Fprintln(out, style.Accent("The explanation and completed objective are shown in the web companion."))
}

func printAchievements(out io.Writer, unlocked []profile.Achievement, style ui.Style) {
	for _, achievement := range unlocked {
		fmt.Fprintf(out, "%s %s — %s\n", style.Achievement("★ Achievement unlocked:"), achievement.Title, achievement.Description)
	}
}
