package dockerlab

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
)

const (
	dockerOperationTimeout = 10 * time.Second
	cleanupTimeout         = 10 * time.Second
	orbStackContext        = "orbstack"
)

var (
	containerIDPattern = regexp.MustCompile(`^[a-f0-9]{12,64}$`)
)

// Factory routes simulated missions to the existing fallback and owns the
// complete lifecycle of Docker-backed attempts.
type Factory struct {
	fallback   game.Factory
	runner     runner
	lookupErr  error
	newSession func() (string, error)
	owner      processOwner
	// pollInterval and readyTimeout override fixture readiness polling in
	// tests.
	pollInterval time.Duration
	readyTimeout time.Duration
}

var (
	_ game.Factory             = (*Factory)(nil)
	_ game.AvailabilityChecker = (*Factory)(nil)
	_ game.ResourceJanitor     = (*Factory)(nil)
)

// NewFactory creates a combined environment factory. Docker remains optional:
// construction succeeds when the CLI is absent so simulated Linux missions
// continue to work and Availability can explain the missing prerequisite.
func NewFactory(fallback game.Factory) *Factory {
	if fallback == nil {
		fallback = game.SandboxFactory{}
	}
	binary, err := exec.LookPath("docker")
	var commandRunner runner
	if err == nil {
		commandRunner = execRunner{binary: binary}
	}
	return &Factory{
		fallback:   fallback,
		runner:     commandRunner,
		lookupErr:  err,
		newSession: randomSessionID,
		owner:      currentProcessOwner(),
	}
}

func newFactory(fallback game.Factory, commandRunner runner, lookupErr error, newSession func() (string, error)) *Factory {
	if fallback == nil {
		fallback = game.SandboxFactory{}
	}
	if newSession == nil {
		newSession = randomSessionID
	}
	return &Factory{fallback: fallback, runner: commandRunner, lookupErr: lookupErr, newSession: newSession, owner: currentProcessOwner()}
}

func (f *Factory) Create(ctx context.Context, item mission.Mission) (game.Environment, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if item.EffectiveEnvironment() != mission.EnvironmentDocker {
		return f.fallback.Create(ctx, item)
	}
	if item.Docker == nil {
		return nil, fmt.Errorf("docker mission %s has no Docker setup", item.ID)
	}
	if err := mission.ValidateDockerSetup(*item.Docker); err != nil {
		return nil, fmt.Errorf("docker mission %s: %w", item.ID, err)
	}
	availability := f.Availability(ctx, item)
	if !availability.Available {
		return nil, errors.New(availability.Detail)
	}
	sessionID, err := f.newSession()
	if err != nil {
		return nil, fmt.Errorf("create Docker session ID: %w", err)
	}
	if !containerIDPattern.MatchString(sessionID) {
		return nil, fmt.Errorf("create Docker session ID: generated value is invalid")
	}

	// Reclaim fixtures abandoned by earlier crashed processes before adding
	// new ones. This is best effort: a failed sweep must not block play, and
	// the same cleanup is available explicitly through opsquest doctor.
	_, _ = f.removeOrphans(ctx)

	environment := &environment{
		runner:        f.runner,
		sessionID:     sessionID,
		missionID:     item.ID,
		owner:         f.owner,
		pollInterval:  f.pollInterval,
		readyTimeout:  f.readyTimeout,
		byAlias:       make(map[string]*trackedContainer),
		networkByName: make(map[string]*trackedNetwork),
	}
	if err := environment.setUp(ctx, *item.Docker); err != nil {
		return environment, joinSetupCleanupError(err, environment.Close())
	}
	return environment, nil
}

func randomSessionID() (string, error) {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func joinSetupCleanupError(primary, cleanup error) error {
	if cleanup == nil {
		return primary
	}
	return fmt.Errorf("%w; cleanup Docker setup: %v", primary, cleanup)
}
