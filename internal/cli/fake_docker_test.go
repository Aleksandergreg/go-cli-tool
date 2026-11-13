package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
)

type cliDockerFactory struct {
	available          bool
	detail             string
	created            int
	availabilityChecks int
}

func (f *cliDockerFactory) Availability(_ context.Context, item mission.Mission) game.Availability {
	f.availabilityChecks++
	if item.EffectiveEnvironment() != mission.EnvironmentDocker {
		return game.Availability{Available: true, Detail: "in-memory sandbox ready"}
	}
	return game.Availability{Available: f.available, Detail: f.detail}
}

func (f *cliDockerFactory) Create(ctx context.Context, item mission.Mission) (game.Environment, error) {
	if item.EffectiveEnvironment() != mission.EnvironmentDocker {
		return (game.SandboxFactory{}).Create(ctx, item)
	}
	if !f.available {
		return nil, fmt.Errorf("%s", f.detail)
	}
	f.created++
	return &cliDockerEnvironment{
		running: map[string]bool{
			"api":     false,
			"metrics": true,
		},
		containerCount: 2,
	}, nil
}

type cliDockerEnvironment struct {
	running        map[string]bool
	containerCount int
	closed         bool
}

func (e *cliDockerEnvironment) PromptLabel() string { return "docker" }

func (e *cliDockerEnvironment) CompletionSource() game.CompletionSource { return nil }

func (e *cliDockerEnvironment) Execute(ctx context.Context, line string) (game.Execution, error) {
	if err := ctx.Err(); err != nil {
		return game.Execution{}, err
	}
	if e.closed {
		return game.Execution{}, fmt.Errorf("Docker test environment is closed")
	}
	fields := strings.Fields(line)
	if len(fields) == 3 && fields[0] == "docker" && fields[1] == "ps" && fields[2] == "-a" {
		return game.Execution{
			Output:            "CONTAINER ID   NAME      STATUS\nlab-01         api       Exited\nlab-02         metrics   Up\n",
			PracticedCommands: []string{"docker"},
			PipelineWidth:     1,
		}, nil
	}
	if len(fields) == 3 && fields[0] == "docker" && fields[1] == "start" && fields[2] == "api" {
		e.running["api"] = true
		return game.Execution{Output: "api\n", PracticedCommands: []string{"docker"}, PipelineWidth: 1}, nil
	}
	return game.Execution{}, fmt.Errorf("unsupported Docker test command %q", line)
}

func (e *cliDockerEnvironment) Observe(ctx context.Context, condition mission.Condition) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	switch condition.Type {
	case "docker_container_running":
		return e.running[condition.Container], nil
	case "docker_container_count_equals":
		if condition.Count == nil {
			return false, fmt.Errorf("missing container count")
		}
		return e.containerCount == *condition.Count, nil
	default:
		return false, fmt.Errorf("unsupported Docker test observation %q", condition.Type)
	}
}

func (e *cliDockerEnvironment) Close() error {
	e.closed = true
	return nil
}

type cliJanitorFactory struct {
	cliDockerFactory
	orphans     int
	checkErr    error
	removeErr   error
	checks      int
	removeCalls int
}

func (f *cliJanitorFactory) OrphanedResources(context.Context) (int, error) {
	f.checks++
	return f.orphans, f.checkErr
}

func (f *cliJanitorFactory) RemoveOrphanedResources(context.Context) (int, error) {
	f.removeCalls++
	if f.removeErr != nil {
		return 1, f.removeErr
	}
	removed := f.orphans
	f.orphans = 0
	return removed, nil
}

// zeroRemovalJanitor fails before removing anything, as when the Docker CLI
// is absent.
type zeroRemovalJanitor struct {
	*cliJanitorFactory
}

func (f *zeroRemovalJanitor) RemoveOrphanedResources(context.Context) (int, error) {
	f.removeCalls++
	return 0, f.removeErr
}
