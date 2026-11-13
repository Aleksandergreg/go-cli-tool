package dockerlab

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/game"
)

const (
	// fixtureLifetime bounds every long-lived fixture process, so a fixture
	// orphaned by an abnormal exit stops consuming resources within a day.
	fixtureLifetime = 24 * time.Hour
	// crashLoopMaxRetries bounds a crash-loop fixture's restart policy. With
	// Docker's capped exponential backoff it keeps looping for roughly 40
	// minutes, which comfortably covers a mission attempt.
	crashLoopMaxRetries = 50
	// crashLoopReadyRestarts is how many restarts setup waits for, so the
	// player's first logs and inspection already show a loop.
	crashLoopReadyRestarts = 2
	fixtureReadyTimeout    = 10 * time.Second
	fixtureReadyInterval   = 100 * time.Millisecond
)

type trackedContainer struct {
	id         string
	logicalID  string
	alias      string
	imageAlias string
	actualName string
}

type environment struct {
	runner       runner
	sessionID    string
	missionID    string
	owner        processOwner
	pollInterval time.Duration
	readyTimeout time.Duration
	containers   []*trackedContainer
	byAlias      map[string]*trackedContainer
	// networks holds every network this attempt created, including ones
	// the player later removed, so cleanup can verify each exact ID.
	networks      []*trackedNetwork
	networkByName map[string]*trackedNetwork

	cleanupMutex    sync.Mutex
	mutex           sync.RWMutex
	closing         bool
	closed          bool
	cleanupPending  []*trackedContainer
	networksPending []*trackedNetwork
}

var (
	_ game.Environment      = (*environment)(nil)
	_ game.CompletionSource = dockerCompletion{}
)

func (e *environment) PromptLabel() string {
	return "docker"
}

func (e *environment) Execute(ctx context.Context, line string) (game.Execution, error) {
	if err := e.ready(ctx); err != nil {
		return game.Execution{}, err
	}
	action, err := parseAction(line)
	if err != nil {
		return game.Execution{}, err
	}
	if action.kind == actionHelp {
		return game.Execution{Output: dockerHelp, PracticedCommands: []string{"help"}, PipelineWidth: 1}, nil
	}
	output, err := e.perform(ctx, action)
	return game.Execution{Output: output, PracticedCommands: []string{"docker"}, PipelineWidth: 1}, err
}

// perform runs one parsed Docker action and returns its terminal output.
func (e *environment) perform(ctx context.Context, action dockerAction) (string, error) {
	switch action.kind {
	case actionList:
		return e.listContainers(ctx, action.all, action.filters)
	case actionStart:
		return echo(action.alias, e.startContainer(ctx, action.alias))
	case actionRestart:
		return echo(action.alias, e.restartContainer(ctx, action.alias))
	case actionStop:
		return echo(action.alias, e.stopContainer(ctx, action.alias))
	case actionRemove:
		return echo(action.alias, e.removeContainer(ctx, action.alias))
	case actionInspect:
		return e.logicalInspect(ctx, action.alias)
	case actionLogs:
		return e.containerLogs(ctx, action.alias, action.tail)
	case actionNetworkList:
		return e.listNetworks(ctx)
	case actionNetworkCreate:
		return e.createPlayerNetwork(ctx, action.network)
	case actionNetworkRemove:
		return echo(action.network, e.removeNetwork(ctx, action.network))
	case actionNetworkInspect:
		return e.logicalNetworkInspect(ctx, action.network)
	case actionNetworkConnect:
		return "", e.connectNetwork(ctx, action.network, action.alias)
	case actionNetworkDisconnect:
		return "", e.disconnectNetwork(ctx, action.network, action.alias)
	default:
		return "", fmt.Errorf("unsupported Docker action")
	}
}

// echo prints the target name after a successful command, as Docker does for
// lifecycle and removal commands.
func echo(name string, err error) (string, error) {
	if err != nil {
		return "", err
	}
	return name + "\n", nil
}

func (e *environment) ready(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	if e.closing || e.closed {
		return fmt.Errorf("Docker mission environment is closed")
	}
	return nil
}

func (e *environment) container(alias string) (*trackedContainer, bool) {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	tracked, exists := e.byAlias[alias]
	return tracked, exists
}

func (e *environment) snapshotContainers() []*trackedContainer {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	return append([]*trackedContainer(nil), e.containers...)
}
