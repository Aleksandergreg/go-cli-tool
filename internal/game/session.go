package game

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
	"github.com/aleksandergregersen/opsquest/internal/ui"
)

type Session struct {
	Mission      mission.Mission
	Player       *profile.Profile
	Saver        ProfileSaver
	In           io.Reader
	Out          io.Writer
	ErrOut       io.Writer
	Reader       CommandLineReader
	Catalog      mission.Catalog
	ListMissions func([]string) error
	Now          func() time.Time
	Style        ui.Style
	ErrorStyle   ui.Style
	Context      context.Context
	Factory      Factory
}

// ProfileSaver is the narrow persistence boundary a mission attempt needs.
// Loading, reset, and profile-path concerns stay in the CLI layer.
type ProfileSaver interface {
	Save(profile.Profile) error
}

type SessionResult struct {
	Completed bool
	Quit      bool
	XPAwarded int
	HintsUsed int
	// SwitchMission contains a validated mission ID requested from inside the
	// lab. The CLI starts that mission with a fresh environment.
	SwitchMission string
}

func (s Session) Run() (returnResult SessionResult, returnErr error) {
	if s.Player == nil {
		return SessionResult{}, fmt.Errorf("mission session requires a player profile")
	}
	if s.Saver == nil {
		return SessionResult{}, fmt.Errorf("mission session requires profile persistence")
	}
	if s.In == nil {
		s.In = strings.NewReader("")
	}
	if s.Out == nil {
		s.Out = io.Discard
	}
	if s.ErrOut == nil {
		s.ErrOut = io.Discard
	}
	ctx := s.Context
	if ctx == nil {
		ctx = context.Background()
	}
	factory := s.Factory
	if factory == nil {
		factory = SandboxFactory{}
	}
	environment, err := createManagedEnvironment(ctx, factory, s.Mission)
	if err != nil {
		return SessionResult{}, fmt.Errorf("prepare mission: %w", err)
	}
	defer func() {
		if err := environment.close(); err != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("close mission environment: %w", err))
		}
	}()
	if s.Now == nil {
		s.Now = time.Now
	}

	hintsUsed := s.Player.MissionHints(s.Mission.ID)
	printMission(s.Out, s.Mission, hintsUsed, s.Style)
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
		if err := ctx.Err(); err != nil {
			return SessionResult{}, fmt.Errorf("mission context: %w", err)
		}
		line, readErr := reader.ReadLine(s.Style.Prompt(environment.PromptLabel()), environment.CompletionSource())
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
		if fields, navigation, navigationErr := missionNavigationFields(line); navigation {
			if navigationErr != nil {
				fmt.Fprintln(s.ErrOut, s.ErrorStyle.Failure("invalid mission navigation: "+navigationErr.Error()))
				continue
			}
			switch fields[0] {
			case "list", "missions":
				if s.ListMissions == nil {
					fmt.Fprintln(s.ErrOut, s.ErrorStyle.Failure("mission listing is unavailable in this session"))
					continue
				}
				if err := s.ListMissions(fields[1:]); err != nil {
					fmt.Fprintln(s.ErrOut, s.ErrorStyle.Failure(err.Error()))
				}
				continue
			case "play":
				if len(fields) != 2 {
					fmt.Fprintln(s.ErrOut, s.ErrorStyle.Failure("usage inside a mission: play MISSION"))
					continue
				}
				target, found := s.Catalog.Find(fields[1])
				if !found {
					message := fmt.Sprintf("mission %q not found; use list to see available missions", fields[1])
					fmt.Fprintln(s.ErrOut, s.ErrorStyle.Failure(message))
					continue
				}
				if target.ID == s.Mission.ID {
					message := fmt.Sprintf("Already playing Mission %02d: %s.", target.Number, target.Title)
					fmt.Fprintln(s.Out, s.Style.Accent(message))
					continue
				}
				printMissionSwitch(s.Out, target, s.Style)
				return SessionResult{SwitchMission: target.ID, HintsUsed: hintsUsed}, nil
			case "next", "previous", "prev":
				direction := 1
				if fields[0] != "next" {
					direction = -1
				}
				if len(fields) != 1 {
					message := fmt.Sprintf("%s does not accept arguments", fields[0])
					fmt.Fprintln(s.ErrOut, s.ErrorStyle.Failure(message))
					continue
				}
				target, found := s.Catalog.AdjacentInTrack(s.Mission.ID, direction)
				if !found {
					message := fmt.Sprintf("Mission %02d is already at this end of the catalog.", s.Mission.Number)
					fmt.Fprintln(s.Out, s.Style.Warning(message))
					continue
				}
				printMissionSwitch(s.Out, target, s.Style)
				return SessionResult{SwitchMission: target.ID, HintsUsed: hintsUsed}, nil
			}
		}

		switch line {
		case "quit", "exit", ":q":
			fmt.Fprintln(s.Out, s.Style.Muted("Mission paused. Your profile progress is safe."))
			return SessionResult{Quit: true, HintsUsed: hintsUsed}, nil
		case "hint":
			if hintsUsed >= len(s.Mission.Hints) {
				fmt.Fprintln(s.Out, s.Style.Warning("No more hints. ByteWorks has exhausted its documentation budget."))
				continue
			}
			hintsUsed = s.Player.RecordHint(s.Mission.ID)
			if err := s.Saver.Save(*s.Player); err != nil {
				return SessionResult{}, err
			}
			penalty := s.Mission.Rewards.HintPenalty
			message := fmt.Sprintf("Hint %d/%d (-%d XP): %s", hintsUsed, len(s.Mission.Hints), penalty, s.Mission.Hints[hintsUsed-1])
			fmt.Fprintln(s.Out, s.Style.Warning(message))
			continue
		case "objective":
			fmt.Fprintf(s.Out, "%s\n\n%s\n", s.Mission.Objective, s.Style.CommandGuide(s.Mission.SuggestedCommands))
			continue
		case "status":
			outcomes, err := evaluateOutcomes(ctx, s.Mission.Validation, environment.Environment, lastOutput)
			if err != nil {
				return SessionResult{}, fmt.Errorf("check mission status: %w", err)
			}
			printOutcomeStatus(s.Out, outcomes, s.Style)
			continue
		case "restart":
			if err := environment.close(); err != nil {
				return SessionResult{}, fmt.Errorf("restart mission: close current environment: %w", err)
			}
			environment, err = createManagedEnvironment(ctx, factory, s.Mission)
			if err != nil {
				return SessionResult{}, fmt.Errorf("restart mission: %w", err)
			}
			lastOutput = ""
			fmt.Fprintln(s.Out, s.Style.Accent("Mission environment restarted. Hints and command mastery are retained."))
			continue
		case "?":
			printMissionControls(s.Out, s.Style)
			continue
		}

		result, executeErr := environment.Execute(ctx, line)
		if executeErr != nil {
			if err := ctx.Err(); err != nil {
				return SessionResult{}, fmt.Errorf("execute command: %w", err)
			}
			fmt.Fprintln(s.ErrOut, s.ErrorStyle.Failure(executeErr.Error()))
			continue
		}
		if err := ctx.Err(); err != nil {
			return SessionResult{}, fmt.Errorf("execute command: %w", err)
		}
		if result.Interactive != nil {
			if result.Interactive.Run == nil {
				return SessionResult{}, fmt.Errorf("run %s: interactive action has no runner", result.Interactive.Command)
			}
			if err := result.Interactive.Run(reader); err != nil {
				if errors.Is(err, ErrInteractiveEditor) || errors.Is(err, ErrUnsupportedEditorFile) {
					message := fmt.Sprintf("%s: %v", result.Interactive.Command, err)
					fmt.Fprintln(s.ErrOut, s.ErrorStyle.Failure(message))
					continue
				}
				return SessionResult{}, fmt.Errorf("run %s: %w", result.Interactive.Command, err)
			}
		}
		if result.Output != "" {
			fmt.Fprint(s.Out, result.Output)
			if !strings.HasSuffix(result.Output, "\n") {
				fmt.Fprintln(s.Out)
			}
		}
		lastOutput = result.Output
		for _, command := range result.PracticedCommands {
			if !practicedSet[command] {
				practiced = append(practiced, command)
				practicedSet[command] = true
			}
			if s.Player.Commands[command] == 0 && !discoveredSet[command] {
				discovered = append(discovered, command)
				discoveredSet[command] = true
			}
		}
		s.Player.RecordCommands(result.PracticedCommands)
		unlocked := make([]profile.Achievement, 0)
		achievementTime := s.Now()
		if result.PipelineWidth >= 3 {
			unlocked = unlock(s.Player, profile.AchievementPipeDream, achievementTime, unlocked)
		}
		unlocked = append(unlocked, ReconcileCommandAchievements(s.Player, achievementTime)...)
		if err := s.Saver.Save(*s.Player); err != nil {
			return SessionResult{}, err
		}

		outcomes, err := evaluateOutcomes(ctx, s.Mission.Validation, environment.Environment, result.Output)
		if err != nil {
			return SessionResult{}, fmt.Errorf("validate mission: %w", err)
		}
		if !allOutcomesSatisfied(outcomes) {
			printAchievements(s.Out, unlocked, s.Style)
			message := fmt.Sprintf("Not complete yet — %d/%d outcome checks satisfied. Type status to see what remains.", satisfiedOutcomeCount(outcomes), len(outcomes))
			fmt.Fprintln(s.Out, s.Style.Warning(message))
			continue
		}
		if err := environment.close(); err != nil {
			return SessionResult{}, fmt.Errorf("complete mission: close environment before awarding XP: %w", err)
		}

		xp := currentReward(s.Mission, hintsUsed)
		completedAt := s.Now()
		firstCompletion := s.Player.Complete(s.Mission.ID, xp, hintsUsed, completedAt)
		if !firstCompletion {
			xp = 0
		} else {
			unlocked = append(unlocked, ReconcileAchievements(s.Player, s.Catalog, completedAt)...)
		}
		if err := s.Saver.Save(*s.Player); err != nil {
			return SessionResult{}, err
		}
		printCompletion(s.Out, s.Mission, xp, firstCompletion, practiced, discovered, unlocked, s.Style)
		return SessionResult{Completed: true, XPAwarded: xp, HintsUsed: hintsUsed}, nil
	}
}

func printMission(out io.Writer, item mission.Mission, hintsUsed int, style ui.Style) {
	heading := fmt.Sprintf("MISSION %02d: %s", item.Number, item.Title)
	fmt.Fprintf(out, "\n%s\n", style.Header(heading))
	fmt.Fprintln(out, style.Muted(strings.Repeat("=", len(item.Title)+12)))
	fmt.Fprintf(out, "Campaign: %s · Difficulty: %s · Reward: %s\n",
		style.Accent(item.Campaign),
		style.Difficulty(item.Difficulty),
		style.Reward(fmt.Sprintf("%d XP", currentReward(item, hintsUsed))),
	)
	if hintsUsed > 0 {
		fmt.Fprintln(out, style.Warning(fmt.Sprintf("Hints already used: %d/%d", hintsUsed, len(item.Hints))))
	}
	fmt.Fprintf(out, "\n%s\n\n%s\n\n", item.Story, item.Objective)
	fmt.Fprintf(out, "%s\n\n", style.CommandGuide(item.SuggestedCommands))
	printMissionControls(out, style)
}

func printMissionControls(out io.Writer, style ui.Style) {
	fmt.Fprintf(out, "Mission controls: %s, %s, %s, %s. Type %s to see completed and missing outcomes.\n",
		style.Accent("hint"), style.Accent("objective"), style.Accent("restart"), style.Accent("quit"), style.Accent("status"))
	fmt.Fprintf(out, "Navigation: %s, %s, %s, %s. An optional %s prefix also works.\n",
		style.Accent("list --completed"), style.Accent("play NUMBER/ID"), style.Accent("next"), style.Accent("previous"), style.Accent("opsquest"))
	fmt.Fprintf(out, "Type %s for lab commands; valid solutions are judged by their result, not by one command sequence.\n", style.Accent("help"))
	fmt.Fprintf(out, "Prompt editing keys: arrows move and recall history; %s or %s jump across the line.\n",
		style.Accent("Home/End"), style.Accent("Ctrl-A/E"))
	fmt.Fprintf(out, "%s move by word; %s completes; %s, %s, and %s remove text.\n",
		style.Accent("Option/Ctrl-Left/Right"), style.Accent("Tab"), style.Accent("Backspace"), style.Accent("Delete"), style.Accent("Ctrl-W"))
}

func missionNavigationFields(line string) ([]string, bool, error) {
	fields, err := splitMissionNavigation(line)
	if len(fields) == 0 {
		return nil, false, nil
	}
	prefixed := fields[0] == "opsquest"
	if prefixed {
		fields = fields[1:]
		if len(fields) == 0 {
			if err != nil {
				return nil, true, err
			}
			return []string{"missions"}, true, nil
		}
	}
	switch fields[0] {
	case "list", "missions", "play", "next", "previous", "prev":
		return fields, true, err
	default:
		if prefixed && err != nil {
			return nil, true, err
		}
		// Non-navigation input belongs to the active environment, including its
		// own quote and escape errors.
		return nil, false, nil
	}
}

// splitMissionNavigation parses the small, host-independent argument syntax
// accepted by in-mission navigation. It deliberately performs no expansion or
// command execution: whitespace separates fields, quotes group text, and a
// backslash escapes the next character outside single quotes.
func splitMissionNavigation(line string) ([]string, error) {
	fields := make([]string, 0)
	var field strings.Builder
	var quote rune
	escaped := false
	started := false

	flush := func() {
		if !started {
			return
		}
		fields = append(fields, field.String())
		field.Reset()
		started = false
	}

	for _, char := range line {
		if escaped {
			field.WriteRune(char)
			escaped = false
			started = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
				continue
			}
			if quote == '"' && char == '\\' {
				escaped = true
				continue
			}
			field.WriteRune(char)
			continue
		}

		switch {
		case unicode.IsSpace(char):
			flush()
		case char == '\'' || char == '"':
			quote = char
			started = true
		case char == '\\':
			escaped = true
			started = true
		default:
			field.WriteRune(char)
			started = true
		}
	}

	if escaped {
		flush()
		return fields, fmt.Errorf("unfinished escape")
	}
	if quote != 0 {
		flush()
		name := "single"
		if quote == '"' {
			name = "double"
		}
		return fields, fmt.Errorf("unterminated %s quote", name)
	}
	flush()
	return fields, nil
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

func currentReward(item mission.Mission, hintsUsed int) int {
	xp := item.Rewards.XP - hintsUsed*item.Rewards.HintPenalty
	minimum := item.Rewards.XP / 4
	if xp < minimum {
		return minimum
	}
	return xp
}

func printCompletion(out io.Writer, item mission.Mission, xp int, first bool, practiced, discovered []string, unlocked []profile.Achievement, style ui.Style) {
	fmt.Fprintf(out, "\n%s\n", style.Success("✓ Mission complete!"))
	if first {
		fmt.Fprintln(out, style.Reward(fmt.Sprintf("+%d XP", xp)))
	} else {
		fmt.Fprintln(out, style.Muted("Replay complete — XP was already claimed."))
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

func unlock(player *profile.Profile, id string, now time.Time, unlocked []profile.Achievement) []profile.Achievement {
	if achievement, added := player.UnlockAchievement(id, now); added {
		return append(unlocked, achievement)
	}
	return unlocked
}

func printAchievements(out io.Writer, unlocked []profile.Achievement, style ui.Style) {
	for _, achievement := range unlocked {
		message := fmt.Sprintf("★ Achievement unlocked: %s — %s", achievement.Title, achievement.Description)
		fmt.Fprintln(out, style.Achievement(message))
	}
}
