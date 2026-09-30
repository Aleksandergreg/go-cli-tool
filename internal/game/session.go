package game

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

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
	a, err := s.begin()
	if err != nil {
		return SessionResult{}, err
	}
	defer func() {
		if err := a.environment.close(); err != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("close mission environment: %w", err))
		}
	}()
	if err := a.announce(); err != nil {
		return SessionResult{}, err
	}
	a.practiced = make([]string, 0)
	a.discovered = make([]string, 0)
	for {
		if err := a.ctx.Err(); err != nil {
			return SessionResult{}, fmt.Errorf("mission context: %w", err)
		}
		line, readErr := a.reader.ReadLine(a.Style.Prompt(a.environment.PromptLabel()), a.environment.CompletionSource())
		if errors.Is(readErr, io.EOF) {
			fmt.Fprintln(a.Out)
			return a.pause(), nil
		}
		if readErr != nil {
			return SessionResult{}, fmt.Errorf("read command: %w", readErr)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		next, err := a.handle(line)
		if err != nil {
			return SessionResult{}, err
		}
		if next.done {
			return next.result, nil
		}
	}
}

// attempt is the mutable state of one running mission session.
type attempt struct {
	Session
	ctx         context.Context
	factory     Factory
	environment *managedEnvironment
	reader      CommandLineReader
	replaying   bool
	hintsUsed   int
	outcomes    []outcomeResult
	practiced   []string
	discovered  []string
	lastOutput  string
	worldRoute  int
}

// step is the effect of one input line: done ends the session with result.
type step struct {
	done   bool
	result SessionResult
}

func finish(result SessionResult) step {
	return step{done: true, result: result}
}

// begin validates the session, applies defaults, and creates the first
// mission environment.
func (s Session) begin() (*attempt, error) {
	if s.Player == nil {
		return nil, fmt.Errorf("mission session requires a player profile")
	}
	if s.Saver == nil {
		return nil, fmt.Errorf("mission session requires profile persistence")
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
	a := &attempt{Session: s, ctx: defaultContext(s.Context), factory: defaultFactory(s.Factory)}
	environment, err := createManagedEnvironment(a.ctx, a.factory, s.Mission)
	if err != nil {
		return nil, fmt.Errorf("prepare mission: %w", err)
	}
	a.environment = environment
	if a.Now == nil {
		a.Now = time.Now
	}
	a.replaying = s.Player.IsComplete(s.Mission.ID)
	a.hintsUsed = s.Player.MissionHints(s.Mission.ID)
	a.reader = s.Reader
	if a.reader == nil {
		a.reader = NewCommandLineReader(a.In, a.Out)
	}
	return a, nil
}

// announce reports the starting state to the companion and presents the
// mission in the terminal.
func (a *attempt) announce() error {
	if a.Reporter != nil {
		outcomes, err := evaluateOutcomes(a.ctx, a.Mission.Validation, a.environment.Environment, "")
		if err != nil {
			return fmt.Errorf("prepare companion mission status: %w", err)
		}
		a.outcomes = outcomes
		a.report(AttemptStarted, AttemptStateActive, nil)
	}
	if a.Companion {
		fmt.Fprintf(a.Out, "\nMission %02d ready in the web companion. Enter lab commands below.\n", a.Mission.Number)
	} else {
		printMission(a.Out, a.Mission, a.hintsUsed, a.replaying, a.Catalog, a.Style)
	}
	return nil
}

// handle routes one input line to navigation, a built-in lab command, or the
// mission environment.
func (a *attempt) handle(line string) (step, error) {
	if fields, navigation, err := missionNavigationFields(line); navigation {
		if err != nil {
			a.fail("invalid mission navigation: " + err.Error())
			return step{}, nil
		}
		if next, handled := a.navigate(fields); handled {
			return next, nil
		}
	}
	switch line {
	case "quit", "exit", ":q":
		fmt.Fprintln(a.Out, a.Style.Accent("Mission paused. Your profile progress is safe."))
		return finish(a.pause()), nil
	case "hint":
		return step{}, a.revealHint()
	case "objective":
		a.showObjective()
		return step{}, nil
	case "status":
		return step{}, a.refreshStatus()
	case "restart":
		return step{}, a.restart()
	case "?", "guide":
		a.showControls()
		return step{}, nil
	}
	return a.execute(line)
}

// pause reports the paused attempt and ends the session without completing.
func (a *attempt) pause() SessionResult {
	a.report(AttemptPaused, AttemptStatePaused, nil)
	return SessionResult{Quit: true, HintsUsed: a.hintsUsed}
}

// report sends the current attempt state to the companion reporter.
func (a *attempt) report(eventType AttemptEventType, state string, unlocked []profile.Achievement) {
	a.reportAttempt(eventType, state, a.outcomes, a.hintsUsed, a.replaying, 0, false, a.practiced, a.discovered, unlocked)
}

func (a *attempt) fail(message string) {
	fmt.Fprintln(a.ErrOut, a.ErrorStyle.Failure(message))
}
