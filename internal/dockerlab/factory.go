package dockerlab

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
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
}

var (
	_ game.Factory             = (*Factory)(nil)
	_ game.AvailabilityChecker = (*Factory)(nil)
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
	}
}

func newFactory(fallback game.Factory, commandRunner runner, lookupErr error, newSession func() (string, error)) *Factory {
	if fallback == nil {
		fallback = game.SandboxFactory{}
	}
	if newSession == nil {
		newSession = randomSessionID
	}
	return &Factory{fallback: fallback, runner: commandRunner, lookupErr: lookupErr, newSession: newSession}
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

	environment := &environment{
		runner:    f.runner,
		sessionID: sessionID,
		missionID: item.ID,
		byAlias:   make(map[string]*trackedContainer),
	}
	images := make(map[string]string, len(item.Docker.Images))
	for _, image := range item.Docker.Images {
		images[image.Alias] = image.Reference
	}
	for index, fixture := range item.Docker.Containers {
		tracked, createErr := environment.createContainer(ctx, index, fixture, images[fixture.Image])
		if tracked != nil {
			environment.containers = append(environment.containers, tracked)
			environment.byAlias[tracked.alias] = tracked
		}
		if createErr != nil {
			return environment, joinSetupCleanupError(createErr, environment.Close())
		}
	}
	for _, fixture := range item.Docker.Containers {
		if fixture.ExitCode != nil {
			if err := environment.startContainer(ctx, fixture.Name); err != nil {
				return environment, joinSetupCleanupError(fmt.Errorf("start diagnostic Docker fixture %s: %w", fixture.Name, err), environment.Close())
			}
			if err := environment.waitContainer(ctx, fixture.Name, *fixture.ExitCode); err != nil {
				return environment, joinSetupCleanupError(fmt.Errorf("wait for diagnostic Docker fixture %s: %w", fixture.Name, err), environment.Close())
			}
			continue
		}
		if fixture.State != mission.DockerStateRunning {
			continue
		}
		if err := environment.startContainer(ctx, fixture.Name); err != nil {
			return environment, joinSetupCleanupError(fmt.Errorf("start Docker fixture %s: %w", fixture.Name, err), environment.Close())
		}
	}
	return environment, nil
}

// Availability checks only the Docker resources needed by item and never
// pulls an image or otherwise mutates the daemon.
func (f *Factory) Availability(ctx context.Context, item mission.Mission) game.Availability {
	if ctx == nil {
		ctx = context.Background()
	}
	if item.EffectiveEnvironment() != mission.EnvironmentDocker {
		return game.EnvironmentAvailability(ctx, f.fallback, item)
	}
	if item.Docker == nil {
		return game.Availability{Available: false, Detail: fmt.Sprintf("Docker mission %s has no Docker setup", item.ID)}
	}
	if err := mission.ValidateDockerSetup(*item.Docker); err != nil {
		return game.Availability{Available: false, Detail: fmt.Sprintf("Docker mission %s is invalid: %v", item.ID, err)}
	}
	if f.lookupErr != nil || f.runner == nil {
		return game.Availability{
			Available: false,
			Detail:    "Docker labs unavailable: docker executable not found in PATH; install Docker Engine, Docker Desktop, or OrbStack, then run 'opsquest doctor'. Linux missions remain available.",
		}
	}
	if err := ctx.Err(); err != nil {
		return game.Availability{Available: false, Detail: fmt.Sprintf("Docker availability check canceled: %v", err)}
	}
	runtime := f.detectDockerRuntime(ctx)
	operationCtx, cancel := context.WithTimeout(ctx, dockerOperationTimeout)
	result, err := f.runner.run(operationCtx, "version", "--format", "{{.Server.Version}}")
	cancel()
	if err != nil || strings.TrimSpace(result.stdout) == "" {
		detail := dockerFailureDetail(result, err)
		instruction := "start Docker or OrbStack and check the active Docker context"
		if runtime == dockerRuntimeOrbStack {
			instruction = "start OrbStack and check the 'orbstack' Docker context"
		}
		return game.Availability{
			Available: false,
			Detail:    "Docker labs unavailable: the Docker-compatible engine could not be reached" + detail + "; " + instruction + ", then run 'opsquest doctor'. Linux missions remain available.",
		}
	}
	seen := make(map[string]bool, len(item.Docker.Images))
	for _, image := range item.Docker.Images {
		if seen[image.Reference] {
			continue
		}
		seen[image.Reference] = true
		operationCtx, cancel = context.WithTimeout(ctx, dockerOperationTimeout)
		result, err = f.runner.run(operationCtx, "image", "inspect", "--format", "{{.Id}}", image.Reference)
		cancel()
		if err != nil || strings.TrimSpace(result.stdout) == "" {
			pullCommand := "docker pull " + image.Reference
			if runtime == dockerRuntimeOrbStack {
				pullCommand = "DOCKER_CONTEXT=orbstack " + pullCommand
			}
			return game.Availability{
				Available: false,
				Detail: fmt.Sprintf(
					"Docker lab image %s is not available locally; run '%s' and try again. OpsQuest never pulls images automatically.",
					image.Reference,
					pullCommand,
				),
			}
		}
	}
	if runtime == dockerRuntimeOrbStack {
		return game.Availability{Available: true, Detail: "OrbStack is ready for this mission through the 'orbstack' Docker context."}
	}
	return game.Availability{Available: true, Detail: "Docker is ready for this mission."}
}

type dockerRuntime uint8

const (
	dockerRuntimeDefault dockerRuntime = iota
	dockerRuntimeOrbStack
)

// detectDockerRuntime identifies known Docker-compatible providers only when
// the Docker CLI reports their official context name. Detection is advisory:
// an older CLI or a broken local context store must not make an otherwise
// reachable Docker engine unavailable.
func (f *Factory) detectDockerRuntime(ctx context.Context) dockerRuntime {
	operationCtx, cancel := context.WithTimeout(ctx, dockerOperationTimeout)
	defer cancel()
	result, err := f.runner.run(operationCtx, "context", "show")
	if err == nil && strings.TrimSpace(result.stdout) == orbStackContext {
		return dockerRuntimeOrbStack
	}
	return dockerRuntimeDefault
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

func dockerFailureDetail(result runResult, err error) string {
	message := strings.TrimSpace(result.stderr)
	if message == "" && err != nil {
		message = err.Error()
	}
	if message == "" {
		return ""
	}
	if len(message) > 240 {
		message = message[:240] + "…"
	}
	return ": " + message
}
