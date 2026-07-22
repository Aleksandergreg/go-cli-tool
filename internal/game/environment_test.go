package game

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
	"github.com/aleksandergregersen/opsquest/internal/sandbox"
)

type seamCompletionSource struct {
	commands []string
	paths    map[string][]CompletionCandidate
}

func (s *seamCompletionSource) CommandNames() []string {
	return append([]string(nil), s.commands...)
}

func (s *seamCompletionSource) PathCandidates(prefix string) []CompletionCandidate {
	return append([]CompletionCandidate(nil), s.paths[prefix]...)
}

type seamEnvironment struct {
	prompt      string
	completions CompletionSource
	execute     func(context.Context, string) (Execution, error)
	observe     func(context.Context, mission.Condition) (bool, error)
	close       func() error
	closeCount  int
}

func (e *seamEnvironment) PromptLabel() string {
	return e.prompt
}

func (e *seamEnvironment) Execute(ctx context.Context, line string) (Execution, error) {
	if e.execute == nil {
		return Execution{}, nil
	}
	return e.execute(ctx, line)
}

func (e *seamEnvironment) Observe(ctx context.Context, condition mission.Condition) (bool, error) {
	if e.observe == nil {
		return false, fmt.Errorf("unexpected observation %s", condition.Type)
	}
	return e.observe(ctx, condition)
}

func (e *seamEnvironment) CompletionSource() CompletionSource {
	return e.completions
}

func (e *seamEnvironment) Close() error {
	e.closeCount++
	if e.close == nil {
		return nil
	}
	return e.close()
}

type seamReader struct {
	lines       []string
	index       int
	endErr      error
	prompts     []string
	completions []CompletionSource
	edit        func(sandbox.EditorRequest, viSaveFunc) error
}

func (r *seamReader) ReadLine(prompt string, completions CompletionSource) (string, error) {
	r.prompts = append(r.prompts, prompt)
	r.completions = append(r.completions, completions)
	if r.index >= len(r.lines) {
		if r.endErr != nil {
			return "", r.endErr
		}
		return "", io.EOF
	}
	line := r.lines[r.index]
	r.index++
	return line, nil
}

func (r *seamReader) Edit(request sandbox.EditorRequest, save viSaveFunc) error {
	if r.edit == nil {
		return ErrInteractiveEditor
	}
	return r.edit(request, save)
}

func seamCatalogMission(t *testing.T, ref string) (mission.Catalog, mission.Mission) {
	t.Helper()
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, found := catalog.Find(ref)
	if !found {
		t.Fatalf("mission %s not found", ref)
	}
	return catalog, item
}

func TestSessionRejectsMissingCoreDependencies(t *testing.T) {
	player := profile.New("tester")
	if _, err := (Session{}).Run(); err == nil || !strings.Contains(err.Error(), "player profile") {
		t.Fatalf("Session without player error = %v", err)
	}
	if _, err := (Session{Player: &player}).Run(); err == nil || !strings.Contains(err.Error(), "profile persistence") {
		t.Fatalf("Session without saver error = %v", err)
	}
}

func TestObjectiveRecallsSuggestedCommandsWithoutUsingHint(t *testing.T) {
	catalog, item := seamCatalogMission(t, "4")
	player := profile.New("tester")
	out := &bytes.Buffer{}
	session := Session{
		Mission: item,
		Player:  &player,
		Saver:   profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "tester"),
		Out:     out,
		ErrOut:  &bytes.Buffer{},
		Reader:  &seamReader{lines: []string{"objective", "quit"}},
		Catalog: catalog,
	}

	result, err := session.Run()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Quit || result.HintsUsed != 0 || player.MissionHints(item.ID) != 0 {
		t.Fatalf("result = %#v, persisted hints = %d", result, player.MissionHints(item.ID))
	}
	if got := strings.Count(out.String(), "Commands you may need to solve this level:"); got != 2 {
		t.Fatalf("command guide occurrences = %d, want mission intro and objective recall\n%s", got, out.String())
	}
	if got := strings.Count(out.String(), "  find, grep"); got != 2 {
		t.Fatalf("suggested command occurrences = %d, want 2\n%s", got, out.String())
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
	_, worldTwoFirst := seamCatalogMission(t, "6")
	_, stateMission := seamCatalogMission(t, "3")
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
		{name: "stage switch", item: worldTwoFirst, reader: &seamReader{lines: []string{"play 3"}}, wantSwitch: "linux-runaway"},
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

func TestSandboxEnvironmentPreservesInteractiveVi(t *testing.T) {
	_, item := seamCatalogMission(t, "13")
	environment, err := (SandboxFactory{}).Create(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := environment.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	execution, err := environment.Execute(context.Background(), "vi app.env")
	if err != nil {
		t.Fatal(err)
	}
	if execution.Interactive == nil || execution.Interactive.Command != "vi" || execution.Interactive.Run == nil {
		t.Fatalf("interactive execution = %#v", execution.Interactive)
	}
	reader := &seamReader{edit: func(request sandbox.EditorRequest, save viSaveFunc) error {
		updated := strings.Replace(request.Content, "LOG_LEVEL=debug", "LOG_LEVEL=info", 1)
		return save(request.Path, updated)
	}}
	if err := execution.Interactive.Run(reader); err != nil {
		t.Fatal(err)
	}
	satisfied, err := environment.Observe(context.Background(), mission.Condition{
		Type: "file_content_contains", Path: "/etc/byteworks/app.env", Value: "LOG_LEVEL=info",
	})
	if err != nil || !satisfied {
		t.Fatalf("saved vi outcome = %v, %v", satisfied, err)
	}
}

type unavailableFactory struct {
	Factory
	availability Availability
}

func (f unavailableFactory) Availability(context.Context, mission.Mission) Availability {
	return f.availability
}

func TestEnvironmentAvailabilityIsOptionalAndNonMutating(t *testing.T) {
	_, simulated := seamCatalogMission(t, "1")
	_, docker := seamCatalogMission(t, "17")
	plainFactory := FactoryFunc(func(context.Context, mission.Mission) (Environment, error) {
		t.Fatal("availability unexpectedly created an environment")
		return nil, nil
	})
	if got := EnvironmentAvailability(nil, plainFactory, simulated); !got.Available {
		t.Fatalf("plain factory availability = %#v", got)
	}
	want := Availability{Detail: "Docker daemon unavailable"}
	if got := EnvironmentAvailability(context.Background(), unavailableFactory{availability: want}, docker); got != want {
		t.Fatalf("checked availability = %#v, want %#v", got, want)
	}
	if got := EnvironmentAvailability(context.Background(), SandboxFactory{}, simulated); !got.Available {
		t.Fatalf("sandbox availability = %#v", got)
	}
	if got := EnvironmentAvailability(context.Background(), SandboxFactory{}, docker); got.Available || got.Detail == "" {
		t.Fatalf("Docker availability through sandbox = %#v", got)
	}
}

func TestFactoryErrorClosesPartiallyCreatedEnvironment(t *testing.T) {
	_, item := seamCatalogMission(t, "1")
	createFailure := errors.New("create failed")
	environment := &seamEnvironment{}
	_, err := createManagedEnvironment(context.Background(), FactoryFunc(func(context.Context, mission.Mission) (Environment, error) {
		return environment, createFailure
	}), item)
	if !errors.Is(err, createFailure) || environment.closeCount != 1 {
		t.Fatalf("create error = %v, Close() calls = %d", err, environment.closeCount)
	}
}

func TestOutputValidationStaysCentralAndStateDelegatesToEnvironment(t *testing.T) {
	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("observer"), "expected")
	count := 2
	observed := make([]mission.Condition, 0)
	environment := &seamEnvironment{observe: func(observeContext context.Context, condition mission.Condition) (bool, error) {
		if observeContext.Value(contextKey("observer")) != "expected" {
			return false, errors.New("observer context was not propagated")
		}
		observed = append(observed, condition)
		return condition.Type == "docker_container_count_equals" && condition.Count != nil && *condition.Count == 2, nil
	}}
	validation := mission.Validation{All: []mission.Condition{
		{Type: "output_equals", Value: "ready"},
		{Type: "docker_container_count_equals", Count: &count},
	}}

	outcomes, err := evaluateOutcomes(ctx, validation, environment, "ready\n")
	if err != nil {
		t.Fatal(err)
	}
	if !allOutcomesSatisfied(outcomes) || len(observed) != 1 || observed[0].Type != "docker_container_count_equals" {
		t.Fatalf("outcomes = %#v, delegated conditions = %#v", outcomes, observed)
	}
}

func TestValidateAndProgressKeepSandboxCompatibility(t *testing.T) {
	box, err := sandbox.New(mission.Setup{
		Directories: []mission.DirectorySpec{{Path: "/work"}},
		Files:       []mission.FileSpec{{Path: "/work/result.txt", Content: "done\n"}},
	}, "/work")
	if err != nil {
		t.Fatal(err)
	}
	validation := mission.Validation{All: []mission.Condition{
		{Type: "output_equals", Value: "ready"},
		{Type: "file_content_equals", Path: "/work/result.txt", Value: "done"},
	}}
	complete, err := Validate(validation, box, "ready\n")
	if err != nil || !complete {
		t.Fatalf("Validate() = %v, %v", complete, err)
	}
	satisfied, total, err := Progress(validation, box, "wrong")
	if err != nil || satisfied != 1 || total != 2 {
		t.Fatalf("Progress() = %d/%d, %v", satisfied, total, err)
	}
}
