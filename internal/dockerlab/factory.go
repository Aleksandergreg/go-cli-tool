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
	if err := validateDockerSetup(*item.Docker); err != nil {
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
			return nil, joinSetupCleanupError(createErr, environment.Close())
		}
	}
	for _, fixture := range item.Docker.Containers {
		if fixture.State != "running" {
			continue
		}
		if err := environment.startContainer(ctx, fixture.Name); err != nil {
			return nil, joinSetupCleanupError(fmt.Errorf("start Docker fixture %s: %w", fixture.Name, err), environment.Close())
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
	if err := validateDockerSetup(*item.Docker); err != nil {
		return game.Availability{Available: false, Detail: fmt.Sprintf("Docker mission %s is invalid: %v", item.ID, err)}
	}
	if f.lookupErr != nil || f.runner == nil {
		return game.Availability{
			Available: false,
			Detail:    "Docker labs unavailable: docker executable not found in PATH; install Docker, then run 'opsquest doctor'. Linux missions remain available.",
		}
	}
	if err := ctx.Err(); err != nil {
		return game.Availability{Available: false, Detail: fmt.Sprintf("Docker availability check canceled: %v", err)}
	}
	operationCtx, cancel := context.WithTimeout(ctx, dockerOperationTimeout)
	result, err := f.runner.run(operationCtx, "version", "--format", "{{.Server.Version}}")
	cancel()
	if err != nil || strings.TrimSpace(result.stdout) == "" {
		detail := dockerFailureDetail(result, err)
		return game.Availability{
			Available: false,
			Detail:    "Docker labs unavailable: the Docker daemon could not be reached" + detail + "; start Docker, check your access, then run 'opsquest doctor'. Linux missions remain available.",
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
			return game.Availability{
				Available: false,
				Detail: fmt.Sprintf(
					"Docker lab image %s is not available locally; run 'docker pull %s' and try again. OpsQuest never pulls images automatically.",
					image.Reference,
					image.Reference,
				),
			}
		}
	}
	return game.Availability{Available: true, Detail: "Docker is ready for this mission."}
}

func randomSessionID() (string, error) {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func validateDockerSetup(setup mission.DockerSetup) error {
	if len(setup.Images) == 0 || len(setup.Containers) == 0 {
		return fmt.Errorf("at least one image and container are required")
	}
	images := make(map[string]bool, len(setup.Images))
	for _, image := range setup.Images {
		if !mission.ValidDockerLogicalName(image.Alias) {
			return fmt.Errorf("invalid image alias %q", image.Alias)
		}
		if images[image.Alias] {
			return fmt.Errorf("duplicate image alias %q", image.Alias)
		}
		if !mission.ValidDockerImageReference(image.Reference) {
			return fmt.Errorf("image %s must use a digest-pinned reference", image.Alias)
		}
		images[image.Alias] = true
	}
	containers := make(map[string]bool, len(setup.Containers))
	for _, fixture := range setup.Containers {
		if !mission.ValidDockerLogicalName(fixture.Name) {
			return fmt.Errorf("invalid container name %q", fixture.Name)
		}
		if containers[fixture.Name] {
			return fmt.Errorf("duplicate container name %q", fixture.Name)
		}
		if !images[fixture.Image] {
			return fmt.Errorf("container %s uses undeclared image alias %q", fixture.Name, fixture.Image)
		}
		if fixture.State != "running" && fixture.State != "stopped" {
			return fmt.Errorf("container %s has unsupported state %q", fixture.Name, fixture.State)
		}
		containers[fixture.Name] = true
	}
	return nil
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
