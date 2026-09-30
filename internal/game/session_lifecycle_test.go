package game

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
)

func TestSessionRejectsMissingCoreDependencies(t *testing.T) {
	player := profile.New("tester")
	if _, err := (Session{}).Run(); err == nil || !strings.Contains(err.Error(), "player profile") {
		t.Fatalf("Session without player error = %v", err)
	}
	if _, err := (Session{Player: &player}).Run(); err == nil || !strings.Contains(err.Error(), "profile persistence") {
		t.Fatalf("Session without saver error = %v", err)
	}
}

func TestSessionUsesFactoryContextAndClosesBeforeAwardingXP(t *testing.T) {
	catalog, item := seamCatalogMission(t, "1")
	player := profile.New("tester")
	completionSource := &seamCompletionSource{commands: []string{"solve"}}
	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("attempt"), "expected")
	closedBeforeXP := false
	environment := &seamEnvironment{
		prompt:      "/generic",
		completions: completionSource,
		execute: func(commandContext context.Context, line string) (Execution, error) {
			if commandContext.Value(contextKey("attempt")) != "expected" {
				return Execution{}, errors.New("session context was not propagated")
			}
			if line != "solve" {
				return Execution{}, fmt.Errorf("unexpected command %q", line)
			}
			return Execution{Output: "/home/operator\n", PracticedCommands: []string{"solve"}}, nil
		},
		observe: func(_ context.Context, condition mission.Condition) (bool, error) {
			return false, fmt.Errorf("output condition %s was delegated", condition.Type)
		},
		close: func() error {
			closedBeforeXP = player.XP == 0 && !player.IsComplete(item.ID)
			return nil
		},
	}
	createCalls := 0
	factory := FactoryFunc(func(factoryContext context.Context, created mission.Mission) (Environment, error) {
		createCalls++
		if factoryContext.Value(contextKey("attempt")) != "expected" || created.ID != item.ID {
			return nil, errors.New("factory received the wrong context or mission")
		}
		return environment, nil
	})
	reader := &seamReader{lines: []string{"solve"}}
	out := &bytes.Buffer{}
	session := Session{
		Mission: item,
		Player:  &player,
		Saver:   profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "tester"),
		Out:     out,
		ErrOut:  &bytes.Buffer{},
		Reader:  reader,
		Catalog: catalog,
		Context: ctx,
		Factory: factory,
		Now:     func() time.Time { return time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC) },
	}

	result, err := session.Run()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Completed || result.XPAwarded != 40 || player.XP != 40 {
		t.Fatalf("result = %#v, player XP = %d", result, player.XP)
	}
	if createCalls != 1 || environment.closeCount != 1 || !closedBeforeXP {
		t.Fatalf("create calls = %d, close calls = %d, closed before XP = %v", createCalls, environment.closeCount, closedBeforeXP)
	}
	if len(reader.prompts) != 1 || reader.prompts[0] != "opsquest:/generic$ " {
		t.Fatalf("prompts = %#v", reader.prompts)
	}
	if len(reader.completions) != 1 || reader.completions[0] != completionSource {
		t.Fatalf("reader completions = %#v, want factory source", reader.completions)
	}
	if !strings.Contains(out.String(), "\n/home/operator\n") {
		t.Fatalf("execution output was not kept raw: %q", out.String())
	}
}

func TestSessionClosesEnvironmentOnEveryTerminalPath(t *testing.T) {
	catalog, first := seamCatalogMission(t, "1")
	_, worldTwoFirst := seamCatalogMission(t, "linux-permissions")
	_, stateMission := seamCatalogMission(t, "linux-workspace")
	readFailure := errors.New("read failed")
	observeFailure := errors.New("observe failed")

	tests := []struct {
		name       string
		item       mission.Mission
		reader     *seamReader
		execute    func(context.Context, string) (Execution, error)
		observe    func(context.Context, mission.Condition) (bool, error)
		wantQuit   bool
		wantSwitch string
		wantWorld  int
		wantErrIs  error
	}{
		{name: "quit", item: first, reader: &seamReader{lines: []string{"quit"}}, wantQuit: true},
		{name: "EOF", item: first, reader: &seamReader{}, wantQuit: true},
		{name: "switch", item: first, reader: &seamReader{lines: []string{"next"}}, wantSwitch: "linux-config-crawl"},
		{name: "stage switch", item: worldTwoFirst, reader: &seamReader{lines: []string{"play 4"}}, wantSwitch: "linux-runaway"},
		{name: "world switch", item: first, reader: &seamReader{lines: []string{"world 2"}}, wantSwitch: "linux-permissions", wantWorld: 2},
		{name: "read error", item: first, reader: &seamReader{endErr: readFailure}, wantErrIs: readFailure},
		{
			name:   "validation error",
			item:   stateMission,
			reader: &seamReader{lines: []string{"change"}},
			execute: func(context.Context, string) (Execution, error) {
				return Execution{}, nil
			},
			observe: func(context.Context, mission.Condition) (bool, error) {
				return false, observeFailure
			},
			wantErrIs: observeFailure,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			environment := &seamEnvironment{prompt: "/test", execute: test.execute, observe: test.observe}
			player := profile.New("tester")
			session := Session{
				Mission: test.item,
				Player:  &player,
				Saver:   profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "tester"),
				Out:     &bytes.Buffer{},
				ErrOut:  &bytes.Buffer{},
				Reader:  test.reader,
				Catalog: catalog,
				Factory: FactoryFunc(func(context.Context, mission.Mission) (Environment, error) {
					return environment, nil
				}),
			}

			result, err := session.Run()
			if test.wantErrIs == nil && err != nil {
				t.Fatal(err)
			}
			if test.wantErrIs != nil && !errors.Is(err, test.wantErrIs) {
				t.Fatalf("Run() error = %v, want errors.Is(..., %v)", err, test.wantErrIs)
			}
			if result.Quit != test.wantQuit || result.SwitchMission != test.wantSwitch || result.WorldRoute != test.wantWorld {
				t.Fatalf("result = %#v", result)
			}
			if environment.closeCount != 1 {
				t.Fatalf("Close() calls = %d, want 1", environment.closeCount)
			}
		})
	}
}

func TestSessionRestartClosesAndRecreatesEnvironment(t *testing.T) {
	catalog, item := seamCatalogMission(t, "1")
	first := &seamEnvironment{prompt: "/first"}
	second := &seamEnvironment{prompt: "/second"}
	created := 0
	factory := FactoryFunc(func(context.Context, mission.Mission) (Environment, error) {
		created++
		switch created {
		case 1:
			return first, nil
		case 2:
			if first.closeCount != 1 {
				return nil, fmt.Errorf("first environment was not closed before recreation")
			}
			return second, nil
		default:
			return nil, fmt.Errorf("unexpected create call %d", created)
		}
	})
	reader := &seamReader{lines: []string{"restart", "quit"}}
	player := profile.New("tester")
	session := Session{
		Mission: item,
		Player:  &player,
		Saver:   profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "tester"),
		Out:     &bytes.Buffer{},
		ErrOut:  &bytes.Buffer{},
		Reader:  reader,
		Catalog: catalog,
		Factory: factory,
	}

	result, err := session.Run()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Quit || created != 2 || first.closeCount != 1 || second.closeCount != 1 {
		t.Fatalf("result = %#v, created = %d, close counts = %d/%d", result, created, first.closeCount, second.closeCount)
	}
	if len(reader.prompts) != 2 || reader.prompts[0] != "opsquest:/first$ " || reader.prompts[1] != "opsquest:/second$ " {
		t.Fatalf("restart prompts = %#v", reader.prompts)
	}
}

func TestSessionCancellationDuringExecutionClosesEnvironment(t *testing.T) {
	catalog, item := seamCatalogMission(t, "1")
	ctx, cancel := context.WithCancel(context.Background())
	environment := &seamEnvironment{
		prompt: "/test",
		execute: func(context.Context, string) (Execution, error) {
			cancel()
			return Execution{}, context.Canceled
		},
	}
	player := profile.New("tester")
	session := Session{
		Mission: item,
		Player:  &player,
		Saver:   profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "tester"),
		Out:     &bytes.Buffer{},
		ErrOut:  &bytes.Buffer{},
		Reader:  &seamReader{lines: []string{"solve"}},
		Catalog: catalog,
		Context: ctx,
		Factory: FactoryFunc(func(context.Context, mission.Mission) (Environment, error) { return environment, nil }),
	}

	_, err := session.Run()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context cancellation", err)
	}
	if environment.closeCount != 1 {
		t.Fatalf("Close() calls = %d, want 1", environment.closeCount)
	}
}

func TestCompletionCleanupFailurePreventsXPAndIsRetriedByDefer(t *testing.T) {
	catalog, item := seamCatalogMission(t, "1")
	closeFailure := errors.New("cleanup failed")
	closeAttempts := 0
	environment := &seamEnvironment{
		prompt: "/test",
		execute: func(context.Context, string) (Execution, error) {
			return Execution{Output: "/home/operator\n"}, nil
		},
		close: func() error {
			closeAttempts++
			if closeAttempts == 1 {
				return closeFailure
			}
			return nil
		},
	}
	player := profile.New("tester")
	session := Session{
		Mission: item,
		Player:  &player,
		Saver:   profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "tester"),
		Out:     &bytes.Buffer{},
		ErrOut:  &bytes.Buffer{},
		Reader:  &seamReader{lines: []string{"solve"}},
		Catalog: catalog,
		Factory: FactoryFunc(func(context.Context, mission.Mission) (Environment, error) { return environment, nil }),
	}

	_, err := session.Run()
	if !errors.Is(err, closeFailure) {
		t.Fatalf("Run() error = %v, want cleanup failure", err)
	}
	if player.XP != 0 || player.IsComplete(item.ID) {
		t.Fatalf("cleanup failure awarded completion: XP = %d, completed = %v", player.XP, player.IsComplete(item.ID))
	}
	if environment.closeCount != 2 {
		t.Fatalf("Close() calls = %d, want explicit attempt plus deferred retry", environment.closeCount)
	}
}

func TestDeferredCleanupFailureIsReturnedOnce(t *testing.T) {
	catalog, item := seamCatalogMission(t, "1")
	closeFailure := errors.New("cleanup failed")
	environment := &seamEnvironment{prompt: "/test", close: func() error { return closeFailure }}
	player := profile.New("tester")
	session := Session{
		Mission: item,
		Player:  &player,
		Saver:   profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "tester"),
		Out:     &bytes.Buffer{},
		ErrOut:  &bytes.Buffer{},
		Reader:  &seamReader{lines: []string{"quit"}},
		Catalog: catalog,
		Factory: FactoryFunc(func(context.Context, mission.Mission) (Environment, error) { return environment, nil }),
	}

	result, err := session.Run()
	if !result.Quit || !errors.Is(err, closeFailure) {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
	if environment.closeCount != 1 {
		t.Fatalf("Close() calls = %d, want 1", environment.closeCount)
	}
}
