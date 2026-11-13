package game

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
	"github.com/aleksandergregersen/opsquest/internal/sandbox"
)

func TestObjectiveRecallsSuggestedCommandsWithoutUsingHint(t *testing.T) {
	catalog, item := seamCatalogMission(t, "linux-find-logs")
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

func TestSandboxEnvironmentPreservesInteractiveVi(t *testing.T) {
	_, item := seamCatalogMission(t, "linux-config-surgery")
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

func TestEnvironmentAvailabilityIsOptionalAndNonMutating(t *testing.T) {
	_, simulated := seamCatalogMission(t, "1")
	_, docker := seamCatalogMission(t, "docker-container-census")
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
