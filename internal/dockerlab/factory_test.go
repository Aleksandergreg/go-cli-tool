package dockerlab

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
)

func TestAvailabilityDistinguishesOptionalDockerFailures(t *testing.T) {
	item := testDockerMission()
	t.Run("CLI missing", func(t *testing.T) {
		factory := newFactory(game.SandboxFactory{}, nil, execNotFoundError{}, nil)
		availability := factory.Availability(context.Background(), item)
		if availability.Available || !strings.Contains(availability.Detail, "executable not found") || !strings.Contains(availability.Detail, "OrbStack") || !strings.Contains(availability.Detail, "Linux missions remain available") {
			t.Fatalf("availability = %#v", availability)
		}
	})

	t.Run("daemon unavailable", func(t *testing.T) {
		commandRunner := newFakeDockerRunner()
		commandRunner.daemonErr = errors.New("Cannot connect to the Docker daemon")
		availability := testFactory(commandRunner).Availability(context.Background(), item)
		if availability.Available || !strings.Contains(availability.Detail, "Docker-compatible engine could not be reached") || !strings.Contains(availability.Detail, "start Docker or OrbStack") {
			t.Fatalf("availability = %#v", availability)
		}
	})

	t.Run("OrbStack unavailable", func(t *testing.T) {
		commandRunner := newFakeDockerRunner()
		commandRunner.contextName = "orbstack"
		commandRunner.daemonErr = errors.New("Cannot connect to the Docker daemon")
		availability := testFactory(commandRunner).Availability(context.Background(), item)
		if availability.Available || !strings.Contains(availability.Detail, "start OrbStack") || !strings.Contains(availability.Detail, "'orbstack' Docker context") {
			t.Fatalf("availability = %#v", availability)
		}
	})

	t.Run("image missing", func(t *testing.T) {
		commandRunner := newFakeDockerRunner()
		commandRunner.imageAvailable = false
		availability := testFactory(commandRunner).Availability(context.Background(), item)
		if availability.Available || !strings.Contains(availability.Detail, "docker pull "+testImageReference) || !strings.Contains(availability.Detail, "never pulls") {
			t.Fatalf("availability = %#v", availability)
		}
		for _, call := range commandRunner.snapshotCalls() {
			if len(call) > 0 && call[0] == "pull" {
				t.Fatalf("Availability invoked an image pull: %q", call)
			}
		}
	})

	t.Run("OrbStack image missing", func(t *testing.T) {
		commandRunner := newFakeDockerRunner()
		commandRunner.contextName = "orbstack"
		commandRunner.imageAvailable = false
		availability := testFactory(commandRunner).Availability(context.Background(), item)
		if availability.Available || !strings.Contains(availability.Detail, "DOCKER_CONTEXT=orbstack docker pull "+testImageReference) {
			t.Fatalf("availability = %#v", availability)
		}
	})

	t.Run("ready", func(t *testing.T) {
		availability := testFactory(newFakeDockerRunner()).Availability(context.Background(), item)
		if !availability.Available || availability.Detail != "Docker is ready for this mission." {
			t.Fatalf("availability = %#v", availability)
		}
	})

	t.Run("OrbStack ready", func(t *testing.T) {
		commandRunner := newFakeDockerRunner()
		commandRunner.contextName = "orbstack"
		availability := testFactory(commandRunner).Availability(context.Background(), item)
		if !availability.Available || !strings.Contains(availability.Detail, "OrbStack is ready") || !strings.Contains(availability.Detail, "'orbstack' Docker context") {
			t.Fatalf("availability = %#v", availability)
		}
	})

	t.Run("context detection failure is advisory", func(t *testing.T) {
		commandRunner := newFakeDockerRunner()
		commandRunner.contextErr = errors.New("context metadata unavailable")
		availability := testFactory(commandRunner).Availability(context.Background(), item)
		if !availability.Available || availability.Detail != "Docker is ready for this mission." {
			t.Fatalf("availability = %#v", availability)
		}
	})
}

type fallbackEnvironment struct{}

func (fallbackEnvironment) PromptLabel() string { return "/fallback" }

func (fallbackEnvironment) Execute(context.Context, string) (game.Execution, error) {
	return game.Execution{}, nil
}

func (fallbackEnvironment) Observe(context.Context, mission.Condition) (bool, error) {
	return false, nil
}

func (fallbackEnvironment) CompletionSource() game.CompletionSource { return fallbackCompletion{} }

func (fallbackEnvironment) Close() error { return nil }

type fallbackCompletion struct{}

func (fallbackCompletion) CommandNames() []string { return nil }

func (fallbackCompletion) PathCandidates(string) []game.CompletionCandidate {
	return nil
}

func TestFactoryDelegatesSimulatedMissionsWithoutDocker(t *testing.T) {
	called := false
	fallback := game.FactoryFunc(func(_ context.Context, item mission.Mission) (game.Environment, error) {
		called = true
		return fallbackEnvironment{}, nil
	})
	factory := newFactory(fallback, nil, execNotFoundError{}, nil)
	created, err := factory.Create(context.Background(), mission.Mission{ID: "linux", Environment: mission.EnvironmentSimulated})
	if err != nil {
		t.Fatal(err)
	}
	if !called || created.PromptLabel() != "/fallback" {
		t.Fatalf("fallback called = %v, environment = %#v", called, created)
	}
}

func TestInvalidDockerSetupNeverReachesRunner(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	item := testDockerMission()
	item.Docker.Images[0].Reference = "--privileged"
	availability := testFactory(commandRunner).Availability(context.Background(), item)
	if availability.Available || !strings.Contains(availability.Detail, "pinned by sha256 digest") {
		t.Fatalf("availability = %#v", availability)
	}
	if len(commandRunner.snapshotCalls()) != 0 {
		t.Fatalf("invalid setup reached Docker runner: %q", commandRunner.snapshotCalls())
	}
}

func TestRuntimeUsesCatalogDockerNameRules(t *testing.T) {
	item := testDockerMission()
	item.Docker.Images[0].Alias = "fixture.v1"
	item.Docker.Containers[0].Image = "fixture.v1"
	item.Docker.Containers[1].Image = "fixture.v1"
	item.Docker.Containers[0].Name = "api_worker"
	if err := mission.ValidateDockerSetup(*item.Docker); err != nil {
		t.Fatalf("catalog-valid Docker names were rejected at runtime: %v", err)
	}
	if action, err := parseAction("docker start api_worker"); err != nil || action.alias != "api_worker" {
		t.Fatalf("parse catalog-valid alias = %#v, %v", action, err)
	}
}
