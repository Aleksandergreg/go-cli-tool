package game

import (
	"context"
	"errors"
	"fmt"

	"github.com/aleksandergregersen/opsquest/internal/mission"
)

// Execution is the environment-neutral result of one player command. Output
// remains unstyled because it is both rendered to the player and supplied to
// mission validators.
type Execution struct {
	Output string
	// PracticedCommands contains successful teaching-command names eligible for
	// player mastery. It is not the raw command line entered by the player.
	PracticedCommands []string
	PipelineWidth     int
	Interactive       *InteractiveAction
}

// InteractiveAction lets an environment hand terminal-only work back to the
// game without exposing an environment-specific editor request in Execution.
// The simulated Linux environment uses it for vi; non-interactive
// environments leave it nil.
type InteractiveAction struct {
	Command string
	Run     func(CommandLineReader) error
}

// CompletionCandidate is one environment-provided token candidate. Directory
// candidates retain a trailing slash and do not close a quoted token.
type CompletionCandidate struct {
	Value     string
	Directory bool
}

// CompletionSource supplies the environment-specific part of terminal
// completion. Mission navigation and controls are added by the game itself.
type CompletionSource interface {
	CommandNames() []string
	PathCandidates(prefix string) []CompletionCandidate
}

// Environment is one isolated mission attempt. Implementations execute only
// their supported teaching subset, observe declarative outcomes, and release
// every resource owned by the attempt from Close. Close must be safe to call
// after partial setup and retryable after a cleanup error.
type Environment interface {
	PromptLabel() string
	Execute(context.Context, string) (Execution, error)
	Observe(context.Context, mission.Condition) (bool, error)
	CompletionSource() CompletionSource
	Close() error
}

// Factory creates a fresh isolated environment from declarative mission
// content. Restarting a mission closes the old environment and calls Create
// again instead of mutating an existing attempt in place. A failed partial
// setup may return both an environment and an error; callers must close that
// environment, and createManagedEnvironment enforces that contract.
type Factory interface {
	Create(context.Context, mission.Mission) (Environment, error)
}

// Availability describes whether a factory can prepare one mission on the
// current machine without creating any persistent mission resources.
type Availability struct {
	Available bool
	Detail    string
}

// AvailabilityChecker is an optional non-mutating factory capability used by
// CLI discovery and diagnostics.
type AvailabilityChecker interface {
	Availability(context.Context, mission.Mission) Availability
}

// EnvironmentAvailability reports ready by default so simple test factories
// and the simulated environment do not need boilerplate capability checks.
func EnvironmentAvailability(ctx context.Context, factory Factory, item mission.Mission) Availability {
	if ctx == nil {
		ctx = context.Background()
	}
	if factory == nil {
		factory = SandboxFactory{}
	}
	if checker, ok := factory.(AvailabilityChecker); ok {
		return checker.Availability(ctx, item)
	}
	return Availability{Available: true, Detail: "ready"}
}

// FactoryFunc adapts a function into a Factory, which keeps focused tests and
// future environment registries small.
type FactoryFunc func(context.Context, mission.Mission) (Environment, error)

func (f FactoryFunc) Create(ctx context.Context, item mission.Mission) (Environment, error) {
	return f(ctx, item)
}

type managedEnvironment struct {
	Environment
	closed bool
}

func createManagedEnvironment(ctx context.Context, factory Factory, item mission.Mission) (*managedEnvironment, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if factory == nil {
		factory = SandboxFactory{}
	}
	environment, err := factory.Create(ctx, item)
	if err != nil {
		if environment != nil {
			err = joinEnvironmentCloseError(err, environment.Close())
		}
		return nil, err
	}
	if environment == nil {
		return nil, fmt.Errorf("environment factory returned a nil environment")
	}
	return &managedEnvironment{Environment: environment}, nil
}

func (e *managedEnvironment) close() error {
	if e == nil || e.closed {
		return nil
	}
	if err := e.Environment.Close(); err != nil {
		return err
	}
	e.closed = true
	return nil
}

func joinEnvironmentCloseError(primary, closeErr error) error {
	if closeErr == nil {
		return primary
	}
	if primary == nil {
		return fmt.Errorf("close mission environment: %w", closeErr)
	}
	return errors.Join(primary, fmt.Errorf("close mission environment: %w", closeErr))
}
