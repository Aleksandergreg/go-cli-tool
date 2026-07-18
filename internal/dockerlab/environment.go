package dockerlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
)

const (
	managedLabel = "com.opsquest.managed"
	schemaLabel  = "com.opsquest.schema"
	sessionLabel = "com.opsquest.session"
	missionLabel = "com.opsquest.mission"
	aliasLabel   = "com.opsquest.alias"
)

type trackedContainer struct {
	id         string
	logicalID  string
	alias      string
	imageAlias string
	actualName string
}

type environment struct {
	runner     runner
	sessionID  string
	missionID  string
	containers []*trackedContainer
	byAlias    map[string]*trackedContainer

	mutex  sync.RWMutex
	closed bool
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
		result.Commands = []string{"help"}
	case actionList:
		result.Output, err = e.listContainers(ctx, action.all)
		result.Commands = []string{"docker"}
	case actionStart:
		err = e.startContainer(ctx, action.alias)
		if err == nil {
			result.Output = action.alias + "\n"
		}
		result.Commands = []string{"docker"}
	case actionRestart:
		err = e.restartContainer(ctx, action.alias)
		if err == nil {
			result.Output = action.alias + "\n"
		}
		result.Commands = []string{"docker"}
	case actionInspect:
		result.Output, err = e.logicalInspect(ctx, action.alias)
		result.Commands = []string{"docker"}
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
	case "docker_container_running":
		tracked, exists := e.container(condition.Container)
		if !exists {
			return false, nil
		}
		inspection, exists, err := e.inspect(ctx, tracked.id)
		return exists && inspection.State.Running, err
	case "docker_container_count_equals":
		if condition.Count == nil {
			return false, fmt.Errorf("docker_container_count_equals requires count")
		}
		count := 0
		for _, tracked := range e.snapshotContainers() {
			_, exists, err := e.inspect(ctx, tracked.id)
			if err != nil {
				return false, err
			}
			if exists {
				count++
			}
		}
		return count == *condition.Count, nil
	default:
		return false, fmt.Errorf("unknown validation type %q for Docker environment", condition.Type)
	}
}

func (e *environment) Close() error {
	e.mutex.Lock()
	if e.closed {
		e.mutex.Unlock()
		return nil
	}
	e.closed = true
	containers := append([]*trackedContainer(nil), e.containers...)
	e.mutex.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	var cleanupErrors []error
	for index := len(containers) - 1; index >= 0; index-- {
		tracked := containers[index]
		inspection, exists, err := e.inspectUnchecked(ctx, tracked.id)
		if err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("inspect Docker fixture %s before cleanup: %w", tracked.alias, err))
			continue
		}
		if !exists {
			continue
		}
		labels := inspection.Config.Labels
		if labels[managedLabel] != "true" || labels[sessionLabel] != e.sessionID || labels[missionLabel] != e.missionID || labels[aliasLabel] != tracked.alias {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("refusing to remove Docker fixture %s because its ownership labels do not match", tracked.alias))
			continue
		}
		if _, err := e.run(ctx, "container", "rm", "--force", tracked.id); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove Docker fixture %s: %w", tracked.alias, err))
		}
	}
	return errors.Join(cleanupErrors...)
}

func (e *environment) createContainer(ctx context.Context, index int, fixture mission.DockerContainerSpec, reference string) (*trackedContainer, error) {
	actualName := fmt.Sprintf("opsquest-%s-c%02d", e.sessionID, index+1)
	args := []string{
		"container", "create",
		"--pull", "never",
		"--name", actualName,
		"--label", managedLabel + "=true",
		"--label", schemaLabel + "=1",
		"--label", sessionLabel + "=" + e.sessionID,
		"--label", missionLabel + "=" + e.missionID,
		"--label", aliasLabel + "=" + fixture.Name,
		"--network", "none",
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
		"--restart", "no",
		"--stop-timeout", "1",
		"--entrypoint", "/bin/sleep",
		reference,
		"86400",
	}
	result, err := e.run(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("create Docker fixture %s: %w", fixture.Name, err)
	}
	id := strings.TrimSpace(result.stdout)
	if !containerIDPattern.MatchString(id) {
		return nil, fmt.Errorf("create Docker fixture %s: Docker returned an invalid container ID", fixture.Name)
	}
	return &trackedContainer{
		id:         id,
		logicalID:  fmt.Sprintf("lab-%02d", index+1),
		alias:      fixture.Name,
		imageAlias: fixture.Image,
		actualName: actualName,
	}, nil
}

func (e *environment) startContainer(ctx context.Context, alias string) error {
	tracked, exists := e.container(alias)
	if !exists {
		return fmt.Errorf("docker: container %q is not part of this mission", alias)
	}
	_, err := e.run(ctx, "container", "start", tracked.id)
	return err
}

func (e *environment) restartContainer(ctx context.Context, alias string) error {
	tracked, exists := e.container(alias)
	if !exists {
		return fmt.Errorf("docker: container %q is not part of this mission", alias)
	}
	_, err := e.run(ctx, "container", "restart", tracked.id)
	return err
}

func (e *environment) listContainers(ctx context.Context, all bool) (string, error) {
	var output strings.Builder
	output.WriteString("CONTAINER ID  IMAGE     STATUS   NAMES\n")
	for _, tracked := range e.snapshotContainers() {
		inspection, exists, err := e.inspect(ctx, tracked.id)
		if err != nil {
			return "", err
		}
		if !exists || !all && !inspection.State.Running {
			continue
		}
		status := inspection.State.Status
		if status == "" {
			if inspection.State.Running {
				status = "running"
			} else {
				status = "stopped"
			}
		}
		fmt.Fprintf(&output, "%-13s %-9s %-8s %s\n", tracked.logicalID, tracked.imageAlias, status, tracked.alias)
	}
	return output.String(), nil
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
	logical := struct {
		ID    string `json:"Id"`
		Name  string `json:"Name"`
		Image string `json:"Image"`
		State struct {
			Running bool   `json:"Running"`
			Status  string `json:"Status"`
		} `json:"State"`
	}{ID: tracked.logicalID, Name: tracked.alias, Image: tracked.imageAlias}
	logical.State.Running = inspection.State.Running
	logical.State.Status = inspection.State.Status
	encoded, err := json.MarshalIndent(logical, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded) + "\n", nil
}

type containerInspection struct {
	ID     string `json:"Id"`
	Config struct {
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	State struct {
		Running bool   `json:"Running"`
		Status  string `json:"Status"`
	} `json:"State"`
}

func (e *environment) inspect(ctx context.Context, id string) (containerInspection, bool, error) {
	if err := e.ready(ctx); err != nil {
		return containerInspection{}, false, err
	}
	return e.inspectUnchecked(ctx, id)
}

func (e *environment) inspectUnchecked(ctx context.Context, id string) (containerInspection, bool, error) {
	result, err := e.run(ctx, "container", "inspect", "--format", "{{json .}}", id)
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
	if inspection.ID != "" && inspection.ID != id {
		return containerInspection{}, false, fmt.Errorf("Docker inspection returned an unexpected container ID")
	}
	return inspection, true, nil
}

func (e *environment) run(ctx context.Context, args ...string) (runResult, error) {
	operationCtx, cancel := context.WithTimeout(ctx, dockerOperationTimeout)
	defer cancel()
	result, err := e.runner.run(operationCtx, args...)
	if err != nil {
		return result, fmt.Errorf("Docker command failed%s", dockerFailureDetail(result, err))
	}
	return result, nil
}

func (e *environment) ready(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	if e.closed {
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
	values := []string{"--all", "-a", "container", "inspect", "ls", "ps", "restart", "start"}
	for _, tracked := range c.environment.snapshotContainers() {
		values = append(values, tracked.alias)
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
