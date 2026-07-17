package game

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
	"github.com/aleksandergregersen/opsquest/internal/sandbox"
)

type Session struct {
	Mission mission.Mission
	Player  *profile.Profile
	Store   profile.Store
	In      io.Reader
	Out     io.Writer
	Err     io.Writer
	Now     func() time.Time
}

type SessionResult struct {
	Completed bool
	Quit      bool
	XPAwarded int
	HintsUsed int
}

func (s Session) Run() (SessionResult, error) {
	box, err := sandbox.New(s.Mission.Setup, s.Mission.StartDir)
	if err != nil {
		return SessionResult{}, fmt.Errorf("prepare mission: %w", err)
	}
	if s.Now == nil {
		s.Now = time.Now
	}

	printMission(s.Out, s.Mission)
	reader := bufio.NewScanner(s.In)
	// Commands are intentionally bounded: this is a teaching shell, not a place
	// to paste generated megabytes into memory.
	reader.Buffer(make([]byte, 1024), 64*1024)
	hintsUsed := s.Player.MissionHints(s.Mission.ID)
	discovered := make([]string, 0)
	discoveredSet := make(map[string]bool)

	for {
		fmt.Fprintf(s.Out, "opsquest:%s$ ", box.CWD)
		if !reader.Scan() {
			if err := reader.Err(); err != nil {
				return SessionResult{}, fmt.Errorf("read command: %w", err)
			}
			fmt.Fprintln(s.Out)
			return SessionResult{Quit: true, HintsUsed: hintsUsed}, nil
		}
		line := strings.TrimSpace(reader.Text())
		if line == "" {
			continue
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
		case "?":
			fmt.Fprintln(s.Out, "Mission controls: hint, objective, quit. Type help for available shell commands.")
			continue
		}

		result, executeErr := box.Execute(line)
		if executeErr != nil {
			fmt.Fprintf(s.Err, "%v\n", executeErr)
			continue
		}
		if result.Output != "" {
			fmt.Fprint(s.Out, result.Output)
			if !strings.HasSuffix(result.Output, "\n") {
				fmt.Fprintln(s.Out)
			}
		}
		for _, command := range result.Commands {
			if s.Player.Commands[command] == 0 && !discoveredSet[command] {
				discovered = append(discovered, command)
				discoveredSet[command] = true
			}
		}
		s.Player.RecordCommands(result.Commands)
		if err := s.Store.Save(*s.Player); err != nil {
			return SessionResult{}, err
		}

		complete, err := Validate(s.Mission.Validation, box, result.Output)
		if err != nil {
			return SessionResult{}, fmt.Errorf("validate mission: %w", err)
		}
		if !complete {
			continue
		}

		xp := s.Mission.Rewards.XP - hintsUsed*s.Mission.Rewards.HintPenalty
		minimum := s.Mission.Rewards.XP / 4
		if xp < minimum {
			xp = minimum
		}
		firstCompletion := s.Player.Complete(s.Mission.ID, xp, hintsUsed, s.Now())
		if !firstCompletion {
			xp = 0
		}
		if err := s.Store.Save(*s.Player); err != nil {
			return SessionResult{}, err
		}
		printCompletion(s.Out, s.Mission, xp, firstCompletion, result.Commands, discovered)
		return SessionResult{Completed: true, XPAwarded: xp, HintsUsed: hintsUsed}, nil
	}
}

func printMission(out io.Writer, item mission.Mission) {
	fmt.Fprintf(out, "\nMISSION %02d: %s\n", item.Number, item.Title)
	fmt.Fprintln(out, strings.Repeat("=", len(item.Title)+12))
	fmt.Fprintf(out, "\n%s\n\n%s\n\n", item.Story, item.Objective)
	fmt.Fprintln(out, "Mission controls: hint, objective, quit. Type help for lab commands.")
}

func printCompletion(out io.Writer, item mission.Mission, xp int, first bool, commands, discovered []string) {
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
	} else if len(commands) > 0 {
		fmt.Fprintf(out, "Command practiced: %s\n", strings.Join(commands, ", "))
	}
	fmt.Fprintf(out, "\n%s\n", item.Explanation)
}
