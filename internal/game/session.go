package game

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
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
	Reporter     AttemptReporter
	Companion    bool
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
	// WorldRoute preserves a world-scoped route requested with `world N`.
	// It may accompany either a mission switch or completion of the current lab.
	WorldRoute int
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
	ctx := defaultContext(s.Context)
	factory := defaultFactory(s.Factory)
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

	replaying := s.Player.IsComplete(s.Mission.ID)
	hintsUsed := s.Player.MissionHints(s.Mission.ID)
	currentOutcomes := []outcomeResult(nil)
	if s.Reporter != nil {
		currentOutcomes, err = evaluateOutcomes(ctx, s.Mission.Validation, environment.Environment, "")
		if err != nil {
			return SessionResult{}, fmt.Errorf("prepare companion mission status: %w", err)
		}
		s.reportAttempt(AttemptStarted, AttemptStateActive, currentOutcomes, hintsUsed, replaying, 0, false, nil, nil, nil)
	}
	if s.Companion {
		fmt.Fprintf(s.Out, "\nMission %02d ready in the web companion. Enter lab commands below.\n", s.Mission.Number)
	} else {
		printMission(s.Out, s.Mission, hintsUsed, replaying, s.Catalog, s.Style)
	}
	reader := s.Reader
	if reader == nil {
		reader = NewCommandLineReader(s.In, s.Out)
	}
	discovered := make([]string, 0)
	practiced := make([]string, 0)
	lastOutput := ""
	worldRoute := 0

	for {
		if err := ctx.Err(); err != nil {
			return SessionResult{}, fmt.Errorf("mission context: %w", err)
		}
		line, readErr := reader.ReadLine(s.Style.Prompt(environment.PromptLabel()), environment.CompletionSource())
		if errors.Is(readErr, io.EOF) {
			fmt.Fprintln(s.Out)
			s.reportAttempt(AttemptPaused, AttemptStatePaused, currentOutcomes, hintsUsed, replaying, 0, false, practiced, discovered, nil)
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
			case "list", "missions", "map", "worlds":
				if s.ListMissions == nil {
					fmt.Fprintln(s.ErrOut, s.ErrorStyle.Failure("mission listing is unavailable in this session"))
					continue
				}
				listArgs := fields[1:]
				if fields[0] == "map" || fields[0] == "worlds" {
					if len(fields) != 1 {
						fmt.Fprintln(s.ErrOut, s.ErrorStyle.Failure(fields[0]+" does not accept arguments"))
						continue
					}
					listArgs = nil
				}
				if err := s.ListMissions(listArgs); err != nil {
					fmt.Fprintln(s.ErrOut, s.ErrorStyle.Failure(err.Error()))
				}
				continue
			case "world":
				if len(fields) != 2 {
					fmt.Fprintln(s.ErrOut, s.ErrorStyle.Failure("usage inside a mission: world NUMBER"))
					continue
				}
				worldNumber, err := strconv.Atoi(fields[1])
				if err != nil || worldNumber < 1 {
					fmt.Fprintln(s.ErrOut, s.ErrorStyle.Failure("world number must be a positive integer; use map to see worlds"))
					continue
				}
				target, found := s.Catalog.NextInWorld(s.Mission.EffectiveTrack(), worldNumber, s.Player.IsComplete)
				replayingCompletedWorld := false
				if !found {
					if world, exists := s.Catalog.World(s.Mission.EffectiveTrack(), worldNumber); exists && len(world.Missions) > 0 {
						target, found = world.Missions[0], true
						replayingCompletedWorld = true
					}
				}
				if !found {
					message := fmt.Sprintf("world %d does not exist in the %s track; use map to see worlds", worldNumber, s.Mission.EffectiveTrack())
					fmt.Fprintln(s.ErrOut, s.ErrorStyle.Failure(message))
					continue
				}
				if target.ID == s.Mission.ID {
					worldRoute = worldNumber
					message := fmt.Sprintf("World %d route selected; already playing the recommended stage.", worldNumber)
					if replayingCompletedWorld {
						message = fmt.Sprintf("World %d replay selected; already playing Stage 1.", worldNumber)
					}
					fmt.Fprintln(s.Out, s.Style.Accent(message))
					continue
				}
				if replayingCompletedWorld {
					fmt.Fprintln(s.Out, s.Style.Accent(fmt.Sprintf("World %d is complete; replaying Stage 1.", worldNumber)))
				}
				printMissionSwitch(s.Out, target, s.Style)
				return SessionResult{SwitchMission: target.ID, WorldRoute: worldNumber, HintsUsed: hintsUsed}, nil
			case "play":
				if len(fields) != 2 {
					fmt.Fprintln(s.ErrOut, s.ErrorStyle.Failure("usage inside a mission: play STAGE_OR_ID"))
					continue
				}
				ref := fields[1]
				target, found := mission.Mission{}, false
				if stageNumber, err := strconv.Atoi(ref); err == nil {
					placement, placed := s.Catalog.Placement(s.Mission.ID)
					if placed {
						world, worldFound := s.Catalog.World(placement.Track, placement.WorldNumber)
						if worldFound && stageNumber >= 1 && stageNumber <= len(world.Missions) {
							target, found = world.Missions[stageNumber-1], true
						}
						if !found {
							message := fmt.Sprintf("stage %q does not exist in World %d; choose a stage from 1 to %d or use map", ref, placement.WorldNumber, placement.StageTotal)
							fmt.Fprintln(s.ErrOut, s.ErrorStyle.Failure(message))
							continue
						}
					}
				} else {
					target, found = s.Catalog.Find(ref)
				}
				if !found {
					message := fmt.Sprintf("mission %q not found; use list --ids to see available mission IDs", ref)
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
			fmt.Fprintln(s.Out, s.Style.Accent("Mission paused. Your profile progress is safe."))
			s.reportAttempt(AttemptPaused, AttemptStatePaused, currentOutcomes, hintsUsed, replaying, 0, false, practiced, discovered, nil)
			return SessionResult{Quit: true, HintsUsed: hintsUsed}, nil
		case "hint":
			if hintsUsed >= len(s.Mission.Hints) {
				fmt.Fprintln(s.Out, s.Style.Warning("No more hints. ByteWorks has exhausted its documentation budget."))
				continue
			}
			cost := 0
			if replaying {
				hintsUsed++
			} else {
				before := AdjustedReward(s.Mission, hintsUsed)
				hintsUsed = s.Player.RecordHint(s.Mission.ID)
				if err := s.Saver.Save(*s.Player); err != nil {
					return SessionResult{}, err
				}
				cost = before - AdjustedReward(s.Mission, hintsUsed)
			}
			costLabel := fmt.Sprintf("-%d XP", cost)
			if cost == 0 {
				costLabel = "no XP cost"
			}
			s.reportAttempt(AttemptHint, AttemptStateActive, currentOutcomes, hintsUsed, replaying, 0, false, practiced, discovered, nil)
			if s.Companion {
				message := fmt.Sprintf("Hint %d/%d revealed in the web companion (%s).", hintsUsed, len(s.Mission.Hints), costLabel)
				fmt.Fprintln(s.Out, s.Style.Warning(message))
			} else {
				prefix := fmt.Sprintf("Hint %d/%d (%s):", hintsUsed, len(s.Mission.Hints), costLabel)
				fmt.Fprintf(s.Out, "%s %s\n", s.Style.Warning(prefix), s.Mission.Hints[hintsUsed-1])
			}
			continue
		case "objective":
			if s.Companion {
				fmt.Fprintln(s.Out, s.Style.Accent("The objective and suggested commands are shown in the web companion."))
			} else {
				fmt.Fprintf(s.Out, "%s\n%s\n\n%s\n", s.Style.Section("OBJECTIVE"), s.Mission.Objective, s.Style.CommandGuide(s.Mission.SuggestedCommands))
			}
			continue
		case "status":
			outcomes, err := evaluateOutcomes(ctx, s.Mission.Validation, environment.Environment, lastOutput)
			if err != nil {
				return SessionResult{}, fmt.Errorf("check mission status: %w", err)
			}
			currentOutcomes = outcomes
			s.reportAttempt(AttemptProgress, AttemptStateActive, currentOutcomes, hintsUsed, replaying, 0, false, practiced, discovered, nil)
			if s.Companion {
				fmt.Fprintln(s.Out, s.Style.Accent("Mission status refreshed in the web companion."))
			} else {
				printOutcomeStatus(s.Out, outcomes, s.Style)
			}
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
			if s.Reporter != nil {
				currentOutcomes, err = evaluateOutcomes(ctx, s.Mission.Validation, environment.Environment, "")
				if err != nil {
					return SessionResult{}, fmt.Errorf("refresh companion mission status: %w", err)
				}
				s.reportAttempt(AttemptRestarted, AttemptStateActive, currentOutcomes, hintsUsed, replaying, 0, false, practiced, discovered, nil)
			}
			fmt.Fprintln(s.Out, s.Style.Accent("Mission environment restarted. Hints and command mastery are retained."))
			continue
		case "?", "guide":
			if s.Companion {
				fmt.Fprintln(s.Out, s.Style.Accent("Mission controls and guidance are shown in the web companion."))
			} else {
				printMissionControls(s.Out, s.Style)
			}
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
			if !slices.Contains(practiced, command) {
				practiced = append(practiced, command)
			}
			if s.Player.Commands[command] == 0 && !slices.Contains(discovered, command) {
				discovered = append(discovered, command)
			}
		}
		s.Player.RecordCommands(result.PracticedCommands)
		unlocked := make([]profile.Achievement, 0)
		achievementTime := s.Now()
		if result.PipelineWidth >= 3 {
			if achievement, added := s.Player.UnlockAchievement(profile.AchievementPipeDream, achievementTime); added {
				unlocked = append(unlocked, achievement)
			}
		}
		unlocked = append(unlocked, ReconcileCommandAchievements(s.Player, achievementTime)...)
		if err := s.Saver.Save(*s.Player); err != nil {
			return SessionResult{}, err
		}

		outcomes, err := evaluateOutcomes(ctx, s.Mission.Validation, environment.Environment, result.Output)
		if err != nil {
			return SessionResult{}, fmt.Errorf("validate mission: %w", err)
		}
		currentOutcomes = outcomes
		s.reportAttempt(AttemptProgress, AttemptStateActive, currentOutcomes, hintsUsed, replaying, 0, false, practiced, discovered, unlocked)
		if !allOutcomesSatisfied(outcomes) {
			printAchievements(s.Out, unlocked, s.Style)
			if !s.Companion {
				message := fmt.Sprintf("Progress — %d/%d outcome checks satisfied. Type status to see what remains.", satisfiedOutcomeCount(outcomes), len(outcomes))
				fmt.Fprintln(s.Out, s.Style.Accent(message))
			}
			continue
		}
		if err := environment.close(); err != nil {
			return SessionResult{}, fmt.Errorf("complete mission: close environment before awarding XP: %w", err)
		}

		xp := AdjustedReward(s.Mission, hintsUsed)
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
		s.reportAttempt(AttemptCompleted, AttemptStateCompleted, currentOutcomes, hintsUsed, replaying, xp, firstCompletion, practiced, discovered, unlocked)
		if s.Companion {
			printCompanionCompletion(s.Out, xp, firstCompletion, unlocked, s.Style)
		} else {
			printCompletion(s.Out, s.Mission, xp, firstCompletion, practiced, discovered, unlocked, s.Style)
		}
		return SessionResult{Completed: true, XPAwarded: xp, HintsUsed: hintsUsed, WorldRoute: worldRoute}, nil
	}
}

func (s Session) reportAttempt(eventType AttemptEventType, state string, outcomes []outcomeResult, hintsUsed int, replaying bool, xp int, firstCompletion bool, practiced, discovered []string, unlocked []profile.Achievement) {
	if s.Reporter == nil {
		return
	}
	placement, _ := s.Catalog.Placement(s.Mission.ID)
	revealedCount := min(hintsUsed, len(s.Mission.Hints))
	revealedHints := slices.Clone(s.Mission.Hints[:revealedCount])
	publicOutcomes := make([]AttemptOutcome, len(outcomes))
	for index, outcome := range outcomes {
		publicOutcomes[index] = AttemptOutcome{Description: outcome.Description, Satisfied: outcome.Satisfied}
	}
	achievements := make([]string, len(unlocked))
	for index, achievement := range unlocked {
		achievements[index] = achievement.Title
	}
	explanation := ""
	if state == AttemptStateCompleted {
		explanation = s.Mission.Explanation
	}
	s.Reporter.ReportAttempt(AttemptEvent{
		Type: eventType,
		Snapshot: AttemptSnapshot{
			MissionID:            s.Mission.ID,
			Number:               s.Mission.Number,
			Title:                s.Mission.Title,
			Track:                s.Mission.EffectiveTrack(),
			WorldNumber:          placement.WorldNumber,
			WorldTotal:           placement.WorldTotal,
			WorldName:            placement.WorldName,
			StageNumber:          placement.StageNumber,
			StageTotal:           placement.StageTotal,
			Difficulty:           s.Mission.Difficulty,
			Story:                s.Mission.Story,
			Objective:            s.Mission.Objective,
			SuggestedCommands:    slices.Clone(s.Mission.SuggestedCommands),
			RevealedHints:        revealedHints,
			HintCount:            len(s.Mission.Hints),
			HintsUsed:            hintsUsed,
			Outcomes:             publicOutcomes,
			SatisfiedOutcomes:    satisfiedOutcomeCount(outcomes),
			RewardAvailable:      AdjustedReward(s.Mission, hintsUsed),
			BaseReward:           s.Mission.Rewards.XP,
			Replaying:            replaying,
			State:                state,
			Explanation:          explanation,
			XPAwarded:            xp,
			FirstCompletion:      firstCompletion,
			PracticedCommands:    slices.Clone(practiced),
			DiscoveredCommands:   slices.Clone(discovered),
			UnlockedAchievements: achievements,
		},
	})
}

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
	case "list", "missions", "map", "worlds", "world", "play", "next", "previous", "prev":
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

// AdjustedReward returns the XP still available after hints while preserving
// the mission's minimum quarter-reward floor.
func AdjustedReward(item mission.Mission, hintsUsed int) int {
	return max(item.Rewards.XP-hintsUsed*item.Rewards.HintPenalty, item.Rewards.XP/4)
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
