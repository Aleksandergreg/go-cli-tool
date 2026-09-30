package dockerlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
)

const (
	managedLabel = "com.opsquest.managed"
	schemaLabel  = "com.opsquest.schema"
	sessionLabel = "com.opsquest.session"
	missionLabel = "com.opsquest.mission"
	aliasLabel   = "com.opsquest.alias"
	// Owner labels let a later process recognize fixtures whose OpsQuest
	// process exited without cleanup. They are advisory: removal still
	// requires every ownership label above to match.
	ownerPIDLabel  = "com.opsquest.owner-pid"
	ownerHostLabel = "com.opsquest.owner-host"

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
	ownerPID     int
	ownerHost    string
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

func (e *environment) CompletionSource() game.CompletionSource {
	return dockerCompletion{environment: e}
}

func (e *environment) Execute(ctx context.Context, line string) (game.Execution, error) {
	if err := e.ready(ctx); err != nil {
		return game.Execution{}, err
	}
	action, err := parseAction(line)
	if err != nil {
		return game.Execution{}, err
	}
	result := game.Execution{PipelineWidth: 1}
	switch action.kind {
	case actionHelp:
		result.Output = dockerHelp
		result.PracticedCommands = []string{"help"}
	case actionList:
		result.Output, err = e.listContainers(ctx, action.all, action.filters)
		result.PracticedCommands = []string{"docker"}
	case actionStart:
		err = e.startContainer(ctx, action.alias)
		if err == nil {
			result.Output = action.alias + "\n"
		}
		result.PracticedCommands = []string{"docker"}
	case actionRestart:
		err = e.restartContainer(ctx, action.alias)
		if err == nil {
			result.Output = action.alias + "\n"
		}
		result.PracticedCommands = []string{"docker"}
	case actionStop:
		err = e.stopContainer(ctx, action.alias)
		if err == nil {
			result.Output = action.alias + "\n"
		}
		result.PracticedCommands = []string{"docker"}
	case actionRemove:
		err = e.removeContainer(ctx, action.alias)
		if err == nil {
			result.Output = action.alias + "\n"
		}
		result.PracticedCommands = []string{"docker"}
	case actionInspect:
		result.Output, err = e.logicalInspect(ctx, action.alias)
		result.PracticedCommands = []string{"docker"}
	case actionLogs:
		result.Output, err = e.containerLogs(ctx, action.alias, action.tail)
		result.PracticedCommands = []string{"docker"}
	case actionNetworkList:
		result.Output, err = e.listNetworks(ctx)
		result.PracticedCommands = []string{"docker"}
	case actionNetworkCreate:
		result.Output, err = e.createPlayerNetwork(ctx, action.network)
		result.PracticedCommands = []string{"docker"}
	case actionNetworkRemove:
		err = e.removeNetwork(ctx, action.network)
		if err == nil {
			result.Output = action.network + "\n"
		}
		result.PracticedCommands = []string{"docker"}
	case actionNetworkInspect:
		result.Output, err = e.logicalNetworkInspect(ctx, action.network)
		result.PracticedCommands = []string{"docker"}
	case actionNetworkConnect:
		err = e.connectNetwork(ctx, action.network, action.alias)
		result.PracticedCommands = []string{"docker"}
	case actionNetworkDisconnect:
		err = e.disconnectNetwork(ctx, action.network, action.alias)
		result.PracticedCommands = []string{"docker"}
	default:
		err = fmt.Errorf("unsupported Docker action")
	}
	return result, err
}

func (e *environment) Observe(ctx context.Context, condition mission.Condition) (bool, error) {
	if err := e.ready(ctx); err != nil {
		return false, err
	}
	switch condition.Type {
	case mission.ConditionDockerContainerRunning:
		tracked, exists := e.container(condition.Container)
		if !exists {
			return false, nil
		}
		inspection, exists, err := e.inspect(ctx, tracked.id)
		return exists && inspection.State.Running, err
	case mission.ConditionDockerContainerStopped:
		tracked, exists := e.container(condition.Container)
		if !exists {
			return false, nil
		}
		inspection, exists, err := e.inspect(ctx, tracked.id)
		return exists && !inspection.State.Running, err
	case mission.ConditionDockerContainerAbsent:
		tracked, exists := e.container(condition.Container)
		if !exists {
			return false, nil
		}
		_, exists, err := e.inspect(ctx, tracked.id)
		return err == nil && !exists, err
	case mission.ConditionDockerNetworkShared, mission.ConditionDockerNetworkIsolated:
		if len(condition.Containers) != 2 {
			return false, fmt.Errorf("%s requires exactly two containers", condition.Type)
		}
		shared, bothExist, err := e.shareNetwork(ctx, condition.Containers[0], condition.Containers[1])
		if err != nil || !bothExist {
			return false, err
		}
		return shared == (condition.Type == mission.ConditionDockerNetworkShared), nil
	case mission.ConditionDockerNetworkAbsent:
		return e.networkAbsent(ctx, condition.Network)
	case mission.ConditionDockerContainerCountEqual:
		if condition.Count == nil {
			return false, fmt.Errorf("docker_container_count_equals requires count")
		}
		count, err := e.countOwnedContainers(ctx)
		if err != nil {
			return false, err
		}
		return count == *condition.Count, nil
	default:
		return false, fmt.Errorf("unknown validation type %q for Docker environment", condition.Type)
	}
}

func (e *environment) Close() error {
	e.cleanupMutex.Lock()
	defer e.cleanupMutex.Unlock()

	e.mutex.Lock()
	if e.closed {
		e.mutex.Unlock()
		return nil
	}
	if !e.closing {
		e.closing = true
		e.cleanupPending = append([]*trackedContainer(nil), e.containers...)
		e.networksPending = append([]*trackedNetwork(nil), e.networks...)
	}
	containers := append([]*trackedContainer(nil), e.cleanupPending...)
	networks := append([]*trackedNetwork(nil), e.networksPending...)
	e.mutex.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	var cleanupErrors []error
	unresolved := make([]*trackedContainer, 0, len(containers))
	for index := len(containers) - 1; index >= 0; index-- {
		tracked := containers[index]
		inspection, exists, err := e.inspectTrackedForCleanup(ctx, tracked)
		if err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("inspect Docker fixture %s before cleanup: %w", tracked.alias, err))
			unresolved = append(unresolved, tracked)
			continue
		}
		if !exists {
			continue
		}
		labels := inspection.Config.Labels
		if labels[managedLabel] != "true" || labels[schemaLabel] != "1" || labels[sessionLabel] != e.sessionID || labels[missionLabel] != e.missionID || labels[aliasLabel] != tracked.alias {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("refusing to remove Docker fixture %s because its ownership labels do not match", tracked.alias))
			unresolved = append(unresolved, tracked)
			continue
		}
		tracked.id = inspection.ID
		if _, err := e.run(ctx, "container", "rm", "--force", inspection.ID); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove Docker fixture %s: %w", tracked.alias, err))
			unresolved = append(unresolved, tracked)
		}
	}

	// Networks go last: Docker refuses to remove a network while any of
	// its containers still exist.
	unresolvedNetworks, networkErrors := e.closeNetworks(ctx, networks)
	cleanupErrors = append(cleanupErrors, networkErrors...)

	e.mutex.Lock()
	e.cleanupPending = unresolved
	e.networksPending = unresolvedNetworks
	e.closed = len(unresolved) == 0 && len(unresolvedNetworks) == 0
	e.mutex.Unlock()
	return errors.Join(cleanupErrors...)
}

func (e *environment) inspectTrackedForCleanup(ctx context.Context, tracked *trackedContainer) (containerInspection, bool, error) {
	if tracked.id != "" {
		return e.inspectUnchecked(ctx, tracked.id)
	}
	inspection, exists, err := e.inspectReferenceUnchecked(ctx, tracked.actualName)
	if err != nil || !exists {
		return inspection, exists, err
	}
	if !containerIDPattern.MatchString(inspection.ID) {
		return containerInspection{}, false, fmt.Errorf("Docker inspection returned an invalid container ID")
	}
	return inspection, true, nil
}

func (e *environment) createContainer(ctx context.Context, index int, fixture mission.DockerContainerSpec, reference string) (*trackedContainer, error) {
	actualName := fmt.Sprintf("opsquest-%s-c%02d", e.sessionID, index+1)
	tracked := &trackedContainer{
		logicalID:  fmt.Sprintf("lab-%02d", index+1),
		alias:      fixture.Name,
		imageAlias: fixture.Image,
		actualName: actualName,
	}
	args := []string{
		"container", "create",
		"--pull", "never",
		"--name", actualName,
		"--label", managedLabel + "=true",
		"--label", schemaLabel + "=1",
		"--label", sessionLabel + "=" + e.sessionID,
		"--label", missionLabel + "=" + e.missionID,
		"--label", aliasLabel + "=" + fixture.Name,
	}
	if e.ownerHost != "" && e.ownerPID > 0 {
		args = append(args,
			"--label", ownerPIDLabel+"="+strconv.Itoa(e.ownerPID),
			"--label", ownerHostLabel+"="+e.ownerHost,
		)
	}
	restartPolicy := "no"
	if fixture.Restart == mission.DockerRestartOnFailure {
		restartPolicy = fmt.Sprintf("on-failure:%d", crashLoopMaxRetries)
	}
	// A fixture without declared networks keeps networking disabled. One
	// with networks starts on the first attempt-owned internal network and
	// joins the rest before it starts.
	networkArgs := []string{"--network", "none"}
	var joined []*trackedNetwork
	for _, name := range fixture.Networks {
		network, exists := e.network(name)
		if !exists || network.id == "" {
			return nil, fmt.Errorf("create Docker fixture %s: network %q was not created", fixture.Name, name)
		}
		joined = append(joined, network)
	}
	if len(joined) > 0 {
		networkArgs = []string{"--network", joined[0].id, "--network-alias", fixture.Name}
	}
	args = append(args, networkArgs...)
	args = append(args,
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
	)
	if probe, configured := healthProbes[fixture.Health]; configured {
		// The probe is a fixed BusyBox applet selected by a validated enum;
		// mission content and player input never supply the probe command.
		args = append(args,
			"--health-cmd", probe,
			"--health-interval", "1s",
			"--health-timeout", "1s",
			"--health-retries", "1",
		)
	}
	lifetime := strconv.Itoa(int(fixtureLifetime / time.Second))
	switch {
	case fixture.ExitCode != nil:
		args = append(args,
			"--entrypoint", "/bin/sh",
			reference,
			"-c", `printf '%s\n' "$1"; exit "$2"`,
			"opsquest-diagnostic", fixture.Log, strconv.Itoa(*fixture.ExitCode),
		)
	case fixture.Log != "":
		args = append(args,
			"--entrypoint", "/bin/sh",
			reference,
			"-c", `printf '%s\n' "$1"; exec sleep "$2"`,
			"opsquest-service", fixture.Log, lifetime,
		)
	default:
		args = append(args,
			"--entrypoint", "/bin/sleep",
			reference,
			lifetime,
		)
	}
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
	if labels[managedLabel] != "true" || labels[sessionLabel] != e.sessionID || labels[missionLabel] != e.missionID || labels[aliasLabel] != tracked.alias {
		return nil, errors.Join(createErr, fmt.Errorf("refusing to recover Docker fixture %s because its ownership labels do not match", tracked.alias))
	}
	if !containerIDPattern.MatchString(inspection.ID) {
		return nil, errors.Join(createErr, fmt.Errorf("reconcile Docker fixture %s: Docker returned an invalid container ID", tracked.alias))
	}
	tracked.id = inspection.ID
	return tracked, createErr
}

// healthProbes maps a fixture health enum to a fixed probe command.
var healthProbes = map[string]string{
	mission.DockerHealthHealthy:   "true",
	mission.DockerHealthUnhealthy: "false",
}

func (e *environment) startContainer(ctx context.Context, alias string) error {
	_, err := e.runOnAlias(ctx, alias, "container", "start")
	return err
}

func (e *environment) restartContainer(ctx context.Context, alias string) error {
	_, err := e.runOnAlias(ctx, alias, "container", "restart")
	return err
}

func (e *environment) stopContainer(ctx context.Context, alias string) error {
	_, err := e.runOnAlias(ctx, alias, "container", "stop")
	return err
}

// removeContainer deletes one stopped mission container. Running containers
// are refused rather than forced so the lesson keeps stop and remove distinct.
func (e *environment) removeContainer(ctx context.Context, alias string) error {
	tracked, exists := e.container(alias)
	if !exists {
		return fmt.Errorf("docker: container %q is not part of this mission", alias)
	}
	inspection, exists, err := e.inspect(ctx, tracked.id)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("docker: container %q has already been removed", alias)
	}
	if inspection.State.Running {
		return fmt.Errorf("docker: cannot remove running container %q; stop it first", alias)
	}
	_, err = e.runOnAlias(ctx, alias, "container", "rm")
	return err
}

// runOnAlias runs one fixed Docker action against the exact container ID
// behind alias. Engine errors about a missing container are translated so
// removed fixtures never reveal their real container ID.
func (e *environment) runOnAlias(ctx context.Context, alias string, args ...string) (runResult, error) {
	tracked, exists := e.container(alias)
	if !exists {
		return runResult{}, fmt.Errorf("docker: container %q is not part of this mission", alias)
	}
	operationCtx, cancel := context.WithTimeout(ctx, dockerOperationTimeout)
	defer cancel()
	result, err := e.runner.run(operationCtx, append(args, tracked.id)...)
	if err != nil {
		if isMissingContainer(result) {
			return result, fmt.Errorf("docker: container %q has been removed", alias)
		}
		return result, fmt.Errorf("Docker command failed%s", redactIdentifiers(dockerFailureDetail(result, err), tracked))
	}
	return result, nil
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

func (e *environment) containerLogs(ctx context.Context, alias string, tail int) (string, error) {
	args := []string{"container", "logs"}
	if tail >= 0 {
		args = append(args, "--tail", strconv.Itoa(tail))
	}
	result, err := e.runOnAlias(ctx, alias, args...)
	if err != nil {
		return "", err
	}
	return result.stdout, nil
}

func (e *environment) listContainers(ctx context.Context, all bool, filters []listFilter) (string, error) {
	type row struct {
		logicalID string
		image     string
		status    string
		alias     string
	}
	rows := make([]row, 0, len(e.containers))
	// Eight columns preserves the original listing layout for plain states.
	statusWidth := 8
	for _, tracked := range e.snapshotContainers() {
		inspection, exists, err := e.inspect(ctx, tracked.id)
		if err != nil {
			return "", err
		}
		// A status filter selects stopped containers even without --all,
		// matching docker ps.
		if !exists || !all && !hasFilterKey(filters, "status") && !inspection.State.Running || !matchesFilters(inspection, filters) {
			continue
		}
		status := displayStatus(inspection)
		statusWidth = max(statusWidth, len(status))
		rows = append(rows, row{logicalID: tracked.logicalID, image: tracked.imageAlias, status: status, alias: tracked.alias})
	}
	var output strings.Builder
	fmt.Fprintf(&output, "%-13s %-9s %-*s %s\n", "CONTAINER ID", "IMAGE", statusWidth, "STATUS", "NAMES")
	for _, item := range rows {
		fmt.Fprintf(&output, "%-13s %-9s %-*s %s\n", item.logicalID, item.image, statusWidth, item.status, item.alias)
	}
	return output.String(), nil
}

func displayStatus(inspection containerInspection) string {
	status := inspection.State.Status
	if status == "" {
		if inspection.State.Running {
			status = "running"
		} else {
			status = "stopped"
		}
	}
	if health := healthStatus(inspection); inspection.State.Running && health != "none" {
		status += " (" + health + ")"
	}
	return status
}

// healthStatus reports the probe verdict, or none for a fixture without a
// health probe. Like docker ps, listings only show it for running containers.
func healthStatus(inspection containerInspection) string {
	if inspection.State.Health == nil || inspection.State.Health.Status == "" {
		return "none"
	}
	return inspection.State.Health.Status
}

func hasFilterKey(filters []listFilter, key string) bool {
	for _, filter := range filters {
		if filter.key == key {
			return true
		}
	}
	return false
}

// matchesFilters follows docker ps semantics: values for one key are
// alternatives, and different keys must all match.
func matchesFilters(inspection containerInspection, filters []listFilter) bool {
	matched := make(map[string]bool, len(filters))
	for _, filter := range filters {
		if _, seen := matched[filter.key]; !seen {
			matched[filter.key] = false
		}
		var actual string
		switch filter.key {
		case "status":
			actual = inspection.State.Status
		case "health":
			actual = "none"
			if inspection.State.Running {
				actual = healthStatus(inspection)
			}
		}
		if actual == filter.value {
			matched[filter.key] = true
		}
	}
	for _, ok := range matched {
		if !ok {
			return false
		}
	}
	return true
}

func (e *environment) logicalInspect(ctx context.Context, alias string) (string, error) {
	tracked, exists := e.container(alias)
	if !exists {
		return "", fmt.Errorf("docker: container %q is not part of this mission", alias)
	}
	inspection, exists, err := e.inspect(ctx, tracked.id)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("docker: container %q no longer exists", alias)
	}
	type logicalHealth struct {
		Status string `json:"Status"`
	}
	logical := struct {
		ID           string `json:"Id"`
		Name         string `json:"Name"`
		Image        string `json:"Image"`
		RestartCount int    `json:"RestartCount"`
		State        struct {
			Running    bool           `json:"Running"`
			Restarting bool           `json:"Restarting"`
			Status     string         `json:"Status"`
			ExitCode   int            `json:"ExitCode"`
			Health     *logicalHealth `json:"Health,omitempty"`
		} `json:"State"`
		HostConfig struct {
			RestartPolicy restartPolicy `json:"RestartPolicy"`
		} `json:"HostConfig"`
		NetworkSettings struct {
			Networks []string `json:"Networks"`
		} `json:"NetworkSettings"`
	}{ID: tracked.logicalID, Name: tracked.alias, Image: tracked.imageAlias, RestartCount: inspection.RestartCount}
	logical.State.Running = inspection.State.Running
	logical.State.Restarting = inspection.State.Restarting
	logical.State.Status = inspection.State.Status
	logical.State.ExitCode = inspection.State.ExitCode
	if health := healthStatus(inspection); health != "none" {
		logical.State.Health = &logicalHealth{Status: health}
	}
	logical.NetworkSettings.Networks = e.logicalNetworks(inspection)
	logical.HostConfig.RestartPolicy = inspection.HostConfig.RestartPolicy
	if logical.HostConfig.RestartPolicy.Name == "" {
		logical.HostConfig.RestartPolicy.Name = "no"
	}
	encoded, err := json.MarshalIndent(logical, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded) + "\n", nil
}

type restartPolicy struct {
	Name              string `json:"Name"`
	MaximumRetryCount int    `json:"MaximumRetryCount"`
}

type healthState struct {
	Status string `json:"Status"`
}

type containerInspection struct {
	ID           string `json:"Id"`
	Name         string `json:"Name"`
	Created      string `json:"Created"`
	RestartCount int    `json:"RestartCount"`
	Config       struct {
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	HostConfig struct {
		RestartPolicy restartPolicy `json:"RestartPolicy"`
	} `json:"HostConfig"`
	State struct {
		Running    bool         `json:"Running"`
		Restarting bool         `json:"Restarting"`
		Status     string       `json:"Status"`
		ExitCode   int          `json:"ExitCode"`
		Health     *healthState `json:"Health"`
	} `json:"State"`
	NetworkSettings struct {
		// Networks is keyed by the engine network name. NetworkID can be
		// empty before a container first starts, so both identify a network.
		Networks map[string]endpointState `json:"Networks"`
	} `json:"NetworkSettings"`
}

type endpointState struct {
	NetworkID string `json:"NetworkID"`
}

func (e *environment) inspect(ctx context.Context, id string) (containerInspection, bool, error) {
	if err := e.ready(ctx); err != nil {
		return containerInspection{}, false, err
	}
	return e.inspectUnchecked(ctx, id)
}

func (e *environment) inspectUnchecked(ctx context.Context, id string) (containerInspection, bool, error) {
	inspection, exists, err := e.inspectReferenceUnchecked(ctx, id)
	if err != nil || !exists {
		return inspection, exists, err
	}
	if inspection.ID != id {
		return containerInspection{}, false, fmt.Errorf("Docker inspection returned an unexpected container ID")
	}
	return inspection, true, nil
}

func (e *environment) inspectReferenceUnchecked(ctx context.Context, reference string) (containerInspection, bool, error) {
	return inspectReference(ctx, e.runner, reference)
}

// inspectReference inspects one exact container ID or generated name. It is
// shared by attempt environments and the orphan janitor.
func inspectReference(ctx context.Context, commandRunner runner, reference string) (containerInspection, bool, error) {
	result, err := runDocker(ctx, commandRunner, "container", "inspect", "--format", "{{json .}}", reference)
	if err != nil {
		if isMissingContainer(result) {
			return containerInspection{}, false, nil
		}
		return containerInspection{}, false, err
	}
	var inspection containerInspection
	if err := json.Unmarshal([]byte(result.stdout), &inspection); err != nil {
		return containerInspection{}, false, fmt.Errorf("decode Docker inspection: %w", err)
	}
	return inspection, true, nil
}

func (e *environment) countOwnedContainers(ctx context.Context) (int, error) {
	result, err := e.run(ctx,
		"container", "ls", "--all", "--quiet", "--no-trunc",
		"--filter", "label="+sessionLabel+"="+e.sessionID,
	)
	if err != nil {
		return 0, err
	}
	ids := strings.Fields(result.stdout)
	count := 0
	for _, id := range ids {
		if !containerIDPattern.MatchString(id) {
			return 0, fmt.Errorf("Docker container listing returned an invalid container ID")
		}
		inspection, exists, err := e.inspect(ctx, id)
		if err != nil {
			return 0, err
		}
		if !exists {
			continue
		}
		labels := inspection.Config.Labels
		if labels[managedLabel] == "true" && labels[schemaLabel] == "1" && labels[sessionLabel] == e.sessionID && labels[missionLabel] == e.missionID {
			count++
		}
	}
	return count, nil
}

func (e *environment) run(ctx context.Context, args ...string) (runResult, error) {
	return runDocker(ctx, e.runner, args...)
}

// runDocker runs one fixed Docker invocation with the standard operation
// timeout and a bounded error summary.
func runDocker(ctx context.Context, commandRunner runner, args ...string) (runResult, error) {
	operationCtx, cancel := context.WithTimeout(ctx, dockerOperationTimeout)
	defer cancel()
	result, err := commandRunner.run(operationCtx, args...)
	if err != nil {
		return result, fmt.Errorf("Docker command failed%s", dockerFailureDetail(result, err))
	}
	return result, nil
}

// redactIdentifiers replaces a tracked container's engine ID and generated
// name in player-visible error text with its logical identity.
func redactIdentifiers(message string, tracked *trackedContainer) string {
	if tracked.id != "" {
		message = strings.ReplaceAll(message, tracked.id, tracked.logicalID)
		if len(tracked.id) >= 12 {
			message = strings.ReplaceAll(message, tracked.id[:12], tracked.logicalID)
		}
	}
	return strings.ReplaceAll(message, tracked.actualName, tracked.alias)
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

func isMissingContainer(result runResult) bool {
	message := strings.ToLower(result.stderr + "\n" + result.stdout)
	return strings.Contains(message, "no such container") || strings.Contains(message, "no such object")
}

type dockerCompletion struct {
	environment *environment
}

func (dockerCompletion) CommandNames() []string {
	return []string{"docker", "help"}
}

func (c dockerCompletion) PathCandidates(prefix string) []game.CompletionCandidate {
	values := []string{"--all", "--filter", "--tail", "-a", "connect", "container", "create", "disconnect", "inspect", "logs", "ls", "network", "ps", "restart", "rm", "start", "stop"}
	for _, tracked := range c.environment.snapshotContainers() {
		values = append(values, tracked.alias)
	}
	seen := make(map[string]bool)
	for _, network := range c.environment.snapshotNetworks() {
		if !seen[network.name] {
			seen[network.name] = true
			values = append(values, network.name)
		}
	}
	sort.Strings(values)
	candidates := make([]game.CompletionCandidate, 0)
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			candidates = append(candidates, game.CompletionCandidate{Value: value})
		}
	}
	return candidates
}
