package dockerlab

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/mission"
)

// setUp creates the mission's networks and fixtures and brings every fixture
// to its teaching state. On error the caller closes the partial attempt.
func (e *environment) setUp(ctx context.Context, setup mission.DockerSetup) error {
	for _, network := range setup.Networks {
		if _, err := e.createNetwork(ctx, network.Name, false); err != nil {
			return err
		}
	}
	images := make(map[string]string, len(setup.Images))
	for _, image := range setup.Images {
		images[image.Alias] = image.Reference
	}
	for index, fixture := range setup.Containers {
		tracked, err := e.createContainer(ctx, index, fixture, images[fixture.Image])
		if tracked != nil {
			e.containers = append(e.containers, tracked)
			e.byAlias[tracked.alias] = tracked
		}
		if err != nil {
			return err
		}
	}
	for _, fixture := range setup.Containers {
		if err := e.startFixture(ctx, fixture); err != nil {
			return err
		}
	}
	// Probes run concurrently once every fixture has started, so waiting
	// after all starts costs one probe interval rather than one per fixture.
	for _, fixture := range setup.Containers {
		if fixture.Health == "" || fixture.State != mission.DockerStateRunning {
			continue
		}
		expected := fixture.Health
		settled := func(inspection containerInspection) bool {
			return healthStatus(inspection) == expected
		}
		if err := e.waitForFixture(ctx, fixture.Name, settled); err != nil {
			return fmt.Errorf("wait for Docker fixture %s health: %w", fixture.Name, err)
		}
	}
	return nil
}

// startFixture brings one created fixture to its initial state: a visible
// crash loop, a completed diagnostic job, or a running service.
func (e *environment) startFixture(ctx context.Context, fixture mission.DockerContainerSpec) error {
	switch {
	case fixture.Restart == mission.DockerRestartOnFailure:
		if err := e.startContainer(ctx, fixture.Name); err != nil {
			return fmt.Errorf("start crash-loop Docker fixture %s: %w", fixture.Name, err)
		}
		looping := func(inspection containerInspection) bool {
			return inspection.RestartCount >= crashLoopReadyRestarts
		}
		if err := e.waitForFixture(ctx, fixture.Name, looping); err != nil {
			return fmt.Errorf("wait for crash-loop Docker fixture %s: %w", fixture.Name, err)
		}
	case fixture.ExitCode != nil:
		if err := e.startContainer(ctx, fixture.Name); err != nil {
			return fmt.Errorf("start diagnostic Docker fixture %s: %w", fixture.Name, err)
		}
		if err := e.waitContainer(ctx, fixture.Name, *fixture.ExitCode); err != nil {
			return fmt.Errorf("wait for diagnostic Docker fixture %s: %w", fixture.Name, err)
		}
	case fixture.State == mission.DockerStateRunning:
		if err := e.startContainer(ctx, fixture.Name); err != nil {
			return fmt.Errorf("start Docker fixture %s: %w", fixture.Name, err)
		}
	}
	return nil
}

func (e *environment) createContainer(ctx context.Context, index int, fixture mission.DockerContainerSpec, reference string) (*trackedContainer, error) {
	actualName := fmt.Sprintf("opsquest-%s-c%02d", e.sessionID, index+1)
	tracked := &trackedContainer{
		logicalID:  fmt.Sprintf("lab-%02d", index+1),
		alias:      fixture.Name,
		imageAlias: fixture.Image,
		actualName: actualName,
	}
	joined, err := e.fixtureNetworks(fixture)
	if err != nil {
		return nil, err
	}
	args := []string{
		"container", "create",
		"--pull", "never",
		"--name", actualName,
	}
	args = append(args, e.ownershipLabels(fixture.Name)...)
	args = append(args, networkArgs(fixture.Name, joined)...)
	args = append(args, hardeningArgs(fixture)...)
	args = append(args, healthArgs(fixture)...)
	args = append(args, fixtureCommand(fixture, reference)...)
	result, err := e.run(ctx, args...)
	if err != nil {
		createErr := fmt.Errorf("create Docker fixture %s: %w", fixture.Name, err)
		return e.reconcileAmbiguousCreate(tracked, createErr)
	}
	id := strings.TrimSpace(result.stdout)
	if !containerIDPattern.MatchString(id) {
		createErr := fmt.Errorf("create Docker fixture %s: Docker returned an invalid container ID", fixture.Name)
		return e.reconcileAmbiguousCreate(tracked, createErr)
	}
	tracked.id = id
	for _, network := range joined[min(1, len(joined)):] {
		if _, err := e.run(ctx, "network", "connect", "--alias", fixture.Name, network.id, id); err != nil {
			return tracked, fmt.Errorf("connect Docker fixture %s to network %s: %w", fixture.Name, network.name, err)
		}
	}
	return tracked, nil
}

// reconcileAmbiguousCreate handles the case where the Docker CLI reports an
// error or malformed output after the daemon may already have created the
// container. The generated name is internal, and ownership labels must match
// before the recovered exact ID is admitted to the cleanup set.
func (e *environment) reconcileAmbiguousCreate(tracked *trackedContainer, createErr error) (*trackedContainer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	inspection, exists, err := e.inspectReferenceUnchecked(ctx, tracked.actualName)
	if err != nil {
		// The create may have reached the daemon even though reconciliation is
		// temporarily unavailable. Retain the generated name so cleanup can
		// safely re-inspect ownership before removing anything.
		return tracked, errors.Join(createErr, fmt.Errorf("reconcile Docker fixture %s: %w", tracked.alias, err))
	}
	if !exists {
		// A successful create can become visible just after a transient missing
		// result. Keep the generated name in the cleanup set so Close performs
		// one more ownership-checked inspection before considering it absent.
		return tracked, createErr
	}
	labels := inspection.Config.Labels
	if !e.owns(labels, tracked.alias) {
		return nil, errors.Join(createErr, fmt.Errorf("refusing to recover Docker fixture %s because its ownership labels do not match", tracked.alias))
	}
	if !containerIDPattern.MatchString(inspection.ID) {
		return nil, errors.Join(createErr, fmt.Errorf("reconcile Docker fixture %s: Docker returned an invalid container ID", tracked.alias))
	}
	tracked.id = inspection.ID
	return tracked, createErr
}

// fixtureNetworks resolves the declared networks a fixture joins, in order.
func (e *environment) fixtureNetworks(fixture mission.DockerContainerSpec) ([]*trackedNetwork, error) {
	var joined []*trackedNetwork
	for _, name := range fixture.Networks {
		network, exists := e.network(name)
		if !exists || network.id == "" {
			return nil, fmt.Errorf("create Docker fixture %s: network %q was not created", fixture.Name, name)
		}
		joined = append(joined, network)
	}
	return joined, nil
}

// networkArgs keeps networking disabled for a fixture without declared
// networks. Otherwise the fixture starts on its first attempt network and
// joins the rest after creation, before it starts.
func networkArgs(alias string, joined []*trackedNetwork) []string {
	if len(joined) == 0 {
		return []string{"--network", "none"}
	}
	return []string{"--network", joined[0].id, "--network-alias", alias}
}

// hardeningArgs are the fixed runtime restrictions every fixture receives.
// Only a declared crash-loop fixture gets a restart policy other than no.
func hardeningArgs(fixture mission.DockerContainerSpec) []string {
	restartPolicy := "no"
	if fixture.Restart == mission.DockerRestartOnFailure {
		restartPolicy = fmt.Sprintf("on-failure:%d", crashLoopMaxRetries)
	}
	return []string{
		"--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,nodev,size=8m",
		"--user", "65532:65532",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--pids-limit", "64",
		"--memory", "128m",
		"--memory-swap", "128m",
		"--cpus", "0.5",
		"--ulimit", "nofile=256:256",
		"--restart", restartPolicy,
		"--stop-timeout", "1",
	}
}

// healthArgs configures a fixed probe for a fixture that declares health.
// The probe is a BusyBox applet selected by a validated enum; mission content
// and player input never supply the probe command.
func healthArgs(fixture mission.DockerContainerSpec) []string {
	probe, configured := healthProbes[fixture.Health]
	if !configured {
		return nil
	}
	return []string{
		"--health-cmd", probe,
		"--health-interval", "1s",
		"--health-timeout", "1s",
		"--health-retries", "1",
	}
}

// fixtureCommand selects the fixed program a fixture runs. Log text, exit
// status, and lifetime are data arguments, never interpolated into the shell
// program.
func fixtureCommand(fixture mission.DockerContainerSpec, reference string) []string {
	lifetime := strconv.Itoa(int(fixtureLifetime / time.Second))
	switch {
	case fixture.ExitCode != nil:
		return []string{
			"--entrypoint", "/bin/sh",
			reference,
			"-c", `printf '%s\n' "$1"; exit "$2"`,
			"opsquest-diagnostic", fixture.Log, strconv.Itoa(*fixture.ExitCode),
		}
	case fixture.Log != "":
		return []string{
			"--entrypoint", "/bin/sh",
			reference,
			"-c", `printf '%s\n' "$1"; exec sleep "$2"`,
			"opsquest-service", fixture.Log, lifetime,
		}
	default:
		return []string{"--entrypoint", "/bin/sleep", reference, lifetime}
	}
}

// healthProbes maps a fixture health enum to a fixed probe command.
var healthProbes = map[string]string{
	mission.DockerHealthHealthy:   "true",
	mission.DockerHealthUnhealthy: "false",
}

func (e *environment) waitContainer(ctx context.Context, alias string, expectedExitCode int) error {
	result, err := e.runOnAlias(ctx, alias, "container", "wait")
	if err != nil {
		return err
	}
	exitCode, err := strconv.Atoi(strings.TrimSpace(result.stdout))
	if err != nil || exitCode != expectedExitCode {
		return fmt.Errorf("Docker fixture %s exited with unexpected status %q", alias, strings.TrimSpace(result.stdout))
	}
	return nil
}

// waitForFixture polls sanitized inspection until ready reports true. Setup
// uses it for fixtures whose teaching state (a health verdict or a visible
// crash loop) appears shortly after start.
func (e *environment) waitForFixture(ctx context.Context, alias string, ready func(containerInspection) bool) error {
	tracked, exists := e.container(alias)
	if !exists {
		return fmt.Errorf("docker: container %q is not part of this mission", alias)
	}
	interval := e.pollInterval
	if interval <= 0 {
		interval = fixtureReadyInterval
	}
	timeout := e.readyTimeout
	if timeout <= 0 {
		timeout = fixtureReadyTimeout
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		inspection, exists, err := e.inspect(ctx, tracked.id)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("Docker fixture %s disappeared during setup", alias)
		}
		if ready(inspection) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("Docker fixture %s did not reach its teaching state within %s", alias, timeout)
		case <-time.After(interval):
		}
	}
}
