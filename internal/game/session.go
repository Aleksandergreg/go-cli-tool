package game

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
	"github.com/aleksandergregersen/opsquest/internal/sandbox"
)

type Session struct {
	Mission      mission.Mission
	MissionCount int
	Player       *profile.Profile
	Store        profile.Store
	In           io.Reader
	Out          io.Writer
	Err          io.Writer
	Reader       CommandLineReader
	Catalog      mission.Catalog
	ListMissions func([]string) error
	Now          func() time.Time
}

type SessionResult struct {
	Completed bool
	Quit      bool
	XPAwarded int
	HintsUsed int
	// SwitchMission contains a validated mission ID requested from inside the
	// lab. The CLI starts that mission with a fresh sandbox.
	SwitchMission string
}

func (s Session) Run() (SessionResult, error) {
	box, err := sandbox.New(s.Mission.Setup, s.Mission.StartDir)
	if err != nil {
		return SessionResult{}, fmt.Errorf("prepare mission: %w", err)
	}
	if s.Now == nil {
		s.Now = time.Now
	}

	hintsUsed := s.Player.MissionHints(s.Mission.ID)
	printMission(s.Out, s.Mission, hintsUsed)
	reader := s.Reader
	if reader == nil {
		reader = NewCommandLineReader(s.In, s.Out)
	}
	discovered := make([]string, 0)
	discoveredSet := make(map[string]bool)
	practiced := make([]string, 0)
	practicedSet := make(map[string]bool)
	lastOutput := ""

	for {
		line, readErr := reader.ReadLine(fmt.Sprintf("opsquest:%s$ ", box.CWD), box)
		if errors.Is(readErr, io.EOF) {
			fmt.Fprintln(s.Out)
			return SessionResult{Quit: true, HintsUsed: hintsUsed}, nil
		}
		if readErr != nil {
			return SessionResult{}, fmt.Errorf("read command: %w", readErr)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if fields, navigation := missionNavigationFields(line); navigation {
			switch fields[0] {
			case "list", "missions":
				if s.ListMissions == nil {
					fmt.Fprintln(s.Err, "mission listing is unavailable in this session")
					continue
				}
				if err := s.ListMissions(fields[1:]); err != nil {
					fmt.Fprintf(s.Err, "%v\n", err)
				}
				continue
			case "play":
				if len(fields) != 2 {
					fmt.Fprintln(s.Err, "usage inside a mission: play MISSION")
					continue
				}
				target, found := s.Catalog.Find(fields[1])
				if !found {
					fmt.Fprintf(s.Err, "mission %q not found; use list to see available missions\n", fields[1])
					continue
				}
				if target.ID == s.Mission.ID {
					fmt.Fprintf(s.Out, "Already playing Mission %02d: %s.\n", target.Number, target.Title)
					continue
				}
				printMissionSwitch(s.Out, target)
				return SessionResult{SwitchMission: target.ID, HintsUsed: hintsUsed}, nil
			case "next", "previous", "prev":
				direction := 1
				if fields[0] != "next" {
					direction = -1
				}
				if len(fields) != 1 {
					fmt.Fprintf(s.Err, "%s does not accept arguments\n", fields[0])
					continue
				}
				target, found := adjacentMission(s.Catalog, s.Mission.ID, direction)
				if !found {
					fmt.Fprintf(s.Out, "Mission %02d is already at this end of the catalog.\n", s.Mission.Number)
					continue
				}
				printMissionSwitch(s.Out, target)
				return SessionResult{SwitchMission: target.ID, HintsUsed: hintsUsed}, nil
			}
		}

		switch line {
		case "quit", "exit", ":q":
			fmt.Fprintln(s.Out, "Mission paused. Your profile progress is safe.")
			return SessionResult{Quit: true, HintsUsed: hintsUsed}, nil
		case "hint":
			if hintsUsed >= len(s.Mission.Hints) {
				fmt.Fprintln(s.Out, "No more hints. ByteWorks has exhausted its documentation budget.")
				continue
			}
			hintsUsed = s.Player.RecordHint(s.Mission.ID)
			if err := s.Store.Save(*s.Player); err != nil {
				return SessionResult{}, err
			}
			penalty := s.Mission.Rewards.HintPenalty
			fmt.Fprintf(s.Out, "Hint %d/%d (-%d XP): %s\n", hintsUsed, len(s.Mission.Hints), penalty, s.Mission.Hints[hintsUsed-1])
			continue
		case "objective":
			fmt.Fprintln(s.Out, s.Mission.Objective)
			continue
		case "status":
			outcomes, err := evaluateOutcomes(s.Mission.Validation, box, lastOutput)
			if err != nil {
				return SessionResult{}, fmt.Errorf("check mission status: %w", err)
			}
			printOutcomeStatus(s.Out, outcomes)
			continue
		case "restart":
			box, err = sandbox.New(s.Mission.Setup, s.Mission.StartDir)
			if err != nil {
				return SessionResult{}, fmt.Errorf("restart mission: %w", err)
			}
			lastOutput = ""
			fmt.Fprintln(s.Out, "Mission environment restarted. Hints and command mastery are retained.")
			continue
		case "?":
			printMissionControls(s.Out)
			continue
		}

		result, executeErr := box.Execute(line)
		if executeErr != nil {
			fmt.Fprintf(s.Err, "%v\n", executeErr)
			continue
		}
		if result.Editor != nil {
			if err := reader.Edit(*result.Editor, box.SaveEditorFile); err != nil {
				if errors.Is(err, ErrInteractiveEditor) || errors.Is(err, ErrUnsupportedEditorFile) {
					fmt.Fprintf(s.Err, "%s: %v\n", result.Editor.Command, err)
					continue
				}
				return SessionResult{}, fmt.Errorf("run %s: %w", result.Editor.Command, err)
			}
		}
		if result.Output != "" {
			fmt.Fprint(s.Out, result.Output)
			if !strings.HasSuffix(result.Output, "\n") {
				fmt.Fprintln(s.Out)
			}
		}
		lastOutput = result.Output
		for _, command := range result.Commands {
			if !practicedSet[command] {
				practiced = append(practiced, command)
				practicedSet[command] = true
			}
			if s.Player.Commands[command] == 0 && !discoveredSet[command] {
				discovered = append(discovered, command)
				discoveredSet[command] = true
			}
		}
		s.Player.RecordCommands(result.Commands)
		unlocked := make([]profile.Achievement, 0)
		if result.PipelineWidth >= 3 {
			unlocked = unlock(s.Player, "pipe-dream", s.Now(), unlocked)
		}
		if len(s.Player.Commands) >= 10 {
			unlocked = unlock(s.Player, "command-collector", s.Now(), unlocked)
		}
		if err := s.Store.Save(*s.Player); err != nil {
			return SessionResult{}, err
		}

		outcomes, err := evaluateOutcomes(s.Mission.Validation, box, result.Output)
		if err != nil {
			return SessionResult{}, fmt.Errorf("validate mission: %w", err)
		}
		if !allOutcomesSatisfied(outcomes) {
			printAchievements(s.Out, unlocked)
			fmt.Fprintf(s.Out, "Not complete yet — %d/%d outcome checks satisfied. Type status to see what remains.\n", satisfiedOutcomeCount(outcomes), len(outcomes))
			continue
		}

		xp := currentReward(s.Mission, hintsUsed)
		completedAt := s.Now()
		firstCompletion := s.Player.Complete(s.Mission.ID, xp, hintsUsed, completedAt)
		if !firstCompletion {
			xp = 0
		} else {
			if len(s.Player.Completed) == 1 {
				unlocked = unlock(s.Player, "first-fix", completedAt, unlocked)
			}
			if s.Player.HintFreeCompletions() >= 5 {
				unlocked = unlock(s.Player, "self-reliant", completedAt, unlocked)
			}
			if s.Mission.Difficulty == "advanced" {
				unlocked = unlock(s.Player, "boss-slayer", completedAt, unlocked)
			}
			if s.MissionCount > 0 && len(s.Player.Completed) >= s.MissionCount {
				unlocked = unlock(s.Player, "linux-completionist", completedAt, unlocked)
			}
		}
		if err := s.Store.Save(*s.Player); err != nil {
			return SessionResult{}, err
		}
		printCompletion(s.Out, s.Mission, xp, firstCompletion, practiced, discovered, unlocked)
		return SessionResult{Completed: true, XPAwarded: xp, HintsUsed: hintsUsed}, nil
	}
}

func printMission(out io.Writer, item mission.Mission, hintsUsed int) {
	fmt.Fprintf(out, "\nMISSION %02d: %s\n", item.Number, item.Title)
	fmt.Fprintln(out, strings.Repeat("=", len(item.Title)+12))
	fmt.Fprintf(out, "Campaign: %s · Difficulty: %s · Reward: %d XP\n", item.Campaign, item.Difficulty, currentReward(item, hintsUsed))
	if hintsUsed > 0 {
		fmt.Fprintf(out, "Hints already used: %d/%d\n", hintsUsed, len(item.Hints))
	}
	fmt.Fprintf(out, "\n%s\n\n%s\n\n", item.Story, item.Objective)
	printMissionControls(out)
}

func printMissionControls(out io.Writer) {
	fmt.Fprintln(out, "Mission controls: hint, objective, status, restart, quit. Type status to see completed and missing outcomes.")
	fmt.Fprintln(out, "Navigation: list --completed, play NUMBER/ID, next, previous. An optional opsquest prefix also works.")
	fmt.Fprintln(out, "Type help for lab commands; valid solutions are judged by their result, not by one command sequence.")
	fmt.Fprintln(out, "Prompt editing keys: arrows move and recall history; Home/End or Ctrl-A/E jump across the line.")
	fmt.Fprintln(out, "Option/Ctrl-Left/Right move by word; Tab completes; Backspace, Delete, and Ctrl-W remove text.")
}

func missionNavigationFields(line string) ([]string, bool) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil, false
	}
	if fields[0] == "opsquest" {
		fields = fields[1:]
		if len(fields) == 0 {
			return []string{"missions"}, true
		}
	}
	switch fields[0] {
	case "list", "missions", "play", "next", "previous", "prev":
		return fields, true
	default:
		return nil, false
	}
}

func adjacentMission(catalog mission.Catalog, currentID string, direction int) (mission.Mission, bool) {
	items := catalog.All()
	for index, item := range items {
		if item.ID != currentID {
			continue
		}
		target := index + direction
		if target < 0 || target >= len(items) {
			return mission.Mission{}, false
		}
		return items[target], true
	}
	return mission.Mission{}, false
}

func printMissionSwitch(out io.Writer, target mission.Mission) {
	fmt.Fprintf(out, "Switching to Mission %02d: %s. The current mission sandbox will reset.\n", target.Number, target.Title)
}

func printOutcomeStatus(out io.Writer, outcomes []outcomeResult) {
	fmt.Fprintf(out, "Outcome checks satisfied: %d/%d.\n", satisfiedOutcomeCount(outcomes), len(outcomes))
	for _, outcome := range outcomes {
		marker := "○"
		if outcome.Satisfied {
			marker = "✓"
		}
		fmt.Fprintf(out, "  %s %s\n", marker, outcome.Description)
	}
}

func currentReward(item mission.Mission, hintsUsed int) int {
	xp := item.Rewards.XP - hintsUsed*item.Rewards.HintPenalty
	minimum := item.Rewards.XP / 4
	if xp < minimum {
		return minimum
	}
	return xp
}

func printCompletion(out io.Writer, item mission.Mission, xp int, first bool, practiced, discovered []string, unlocked []profile.Achievement) {
	fmt.Fprintln(out, "\n✓ Mission complete!")
	if first {
		fmt.Fprintf(out, "+%d XP\n", xp)
	} else {
		fmt.Fprintln(out, "Replay complete — XP was already claimed.")
	}
	if len(discovered) == 1 {
		fmt.Fprintf(out, "New command discovered: %s\n", discovered[0])
	} else if len(discovered) > 1 {
		fmt.Fprintf(out, "New commands discovered: %s\n", strings.Join(discovered, ", "))
	} else if len(practiced) == 1 {
		fmt.Fprintf(out, "Command practiced: %s\n", practiced[0])
	} else if len(practiced) > 1 {
		fmt.Fprintf(out, "Commands practiced: %s\n", strings.Join(practiced, ", "))
	}
	printAchievements(out, unlocked)
	fmt.Fprintf(out, "\n%s\n", item.Explanation)
}

func unlock(player *profile.Profile, id string, now time.Time, unlocked []profile.Achievement) []profile.Achievement {
	if achievement, added := player.UnlockAchievement(id, now); added {
		return append(unlocked, achievement)
	}
	return unlocked
}

func printAchievements(out io.Writer, unlocked []profile.Achievement) {
	for _, achievement := range unlocked {
		fmt.Fprintf(out, "★ Achievement unlocked: %s — %s\n", achievement.Title, achievement.Description)
	}
}
