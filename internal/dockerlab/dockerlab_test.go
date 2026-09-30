package dockerlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
)

const (
	testImageReference = "docker.io/library/busybox@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662"
	testOwnerPID       = 4242
	testOwnerHost      = "test-host"
)

var testNow = time.Date(2026, time.September, 30, 12, 0, 0, 0, time.UTC)

type fakeContainer struct {
	labels        map[string]string
	name          string
	created       time.Time
	running       bool
	restarting    bool
	status        string
	log           string
	exitCode      int
	oneShot       bool
	restartPolicy string
	restartCount  int
	healthCmd     string
	// endpoints maps an attached network's engine name to its ID.
	endpoints map[string]string
}

type fakeNetwork struct {
	labels   map[string]string
	name     string
	created  time.Time
	internal bool
}

type fakeDockerRunner struct {
	mutex sync.Mutex

	calls             [][]string
	containers        map[string]*fakeContainer
	containerOrder    []string
	contextName       string
	contextErr        error
	daemonErr         error
	imageAvailable    bool
	failCreateAt      int
	failAfterCreateAt int
	createAttempts    int
	failNextStart     bool
	failNextInspect   map[string]error
	hideNextInspect   map[string]bool
	failNextRemove    map[string]error
	failManagedList   bool
	stripHealth       bool
	networks          map[string]*fakeNetwork
	networkOrder      []string
	networkCreates    int
	failNetworkCreate bool
	// contextFailure makes every invocation fail the way the Docker CLI does
	// when the selected context cannot be resolved.
	contextFailure bool
	// contextFailureOnNetworkRemove fails only network removal that way.
	contextFailureOnNetworkRemove bool
}

// contextNotFoundMessage is the Docker CLI's output when the selected
// context is missing, captured from Docker 29.7.2.
const contextNotFoundMessage = `Failed to initialize: unable to resolve docker endpoint: context "opsquest-missing-context": context not found: open /home/player/.docker/contexts/meta/fdff/meta.json: no such file or directory`

func (r *fakeDockerRunner) setContextFailure(failing bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.contextFailure = failing
}

func newFakeDockerRunner() *fakeDockerRunner {
	return &fakeDockerRunner{
		containers:      make(map[string]*fakeContainer),
		networks:        make(map[string]*fakeNetwork),
		contextName:     "default",
		imageAvailable:  true,
		failNextInspect: make(map[string]error),
		hideNextInspect: make(map[string]bool),
		failNextRemove:  make(map[string]error),
	}
}

func (r *fakeDockerRunner) run(ctx context.Context, args ...string) (runResult, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if err := ctx.Err(); err != nil {
		return runResult{}, err
	}
	r.calls = append(r.calls, append([]string(nil), args...))
	if r.contextFailure {
		return runResult{stderr: contextNotFoundMessage}, errors.New("exit status 1")
	}

	switch {
	case hasPrefix(args, "context", "show"):
		if r.contextErr != nil {
			return runResult{stderr: r.contextErr.Error()}, r.contextErr
		}
		return runResult{stdout: r.contextName + "\n"}, nil
	case hasPrefix(args, "version", "--format"):
		if r.daemonErr != nil {
			return runResult{stderr: r.daemonErr.Error()}, r.daemonErr
		}
		return runResult{stdout: "27.0.0\n"}, nil
	case hasPrefix(args, "image", "inspect", "--format"):
		if !r.imageAvailable {
			return runResult{stderr: "Error: No such image"}, errors.New("exit status 1")
		}
		return runResult{stdout: "sha256:test-image\n"}, nil
	case hasPrefix(args, "container", "create"):
		r.createAttempts++
		if r.failCreateAt == r.createAttempts {
			return runResult{stderr: "fixture creation failed"}, errors.New("exit status 1")
		}
		// Created IDs use a prefix no seeded test container shares.
		id := fmt.Sprintf("c0ffee%058x", r.createAttempts)
		labels := make(map[string]string)
		name := ""
		restartPolicy := ""
		healthCmd := ""
		endpoints := make(map[string]string)
		for index := 0; index+1 < len(args); index++ {
			switch args[index] {
			case "--network":
				if args[index+1] == "none" {
					endpoints["none"] = "none-network-id"
				} else if network, exists := r.networks[args[index+1]]; exists {
					endpoints[network.name] = args[index+1]
				} else {
					return runResult{stderr: "Error response from daemon: network " + args[index+1] + " not found"}, errors.New("exit status 1")
				}
			case "--name":
				name = args[index+1]
			case "--label":
				labelName, value, found := strings.Cut(args[index+1], "=")
				if found {
					labels[labelName] = value
				}
			case "--restart":
				restartPolicy = args[index+1]
			case "--health-cmd":
				healthCmd = args[index+1]
			}
		}
		container := &fakeContainer{labels: labels, name: name, created: testNow, status: "created", restartPolicy: restartPolicy, healthCmd: healthCmd, endpoints: endpoints}
		if len(args) >= 3 && args[len(args)-3] == "opsquest-service" {
			container.log = args[len(args)-2] + "\n"
		}
		if len(args) >= 3 && args[len(args)-3] == "opsquest-diagnostic" {
			container.oneShot = true
			container.log = args[len(args)-2] + "\n"
			exitCode, err := strconv.Atoi(args[len(args)-1])
			if err != nil {
				return runResult{}, err
			}
			container.exitCode = exitCode
		}
		r.containers[id] = container
		r.containerOrder = append(r.containerOrder, id)
		if r.failAfterCreateAt == r.createAttempts {
			return runResult{stderr: "connection lost after create"}, errors.New("exit status 1")
		}
		return runResult{stdout: id + "\n"}, nil
	case hasPrefix(args, "container", "start") || hasPrefix(args, "container", "restart"):
		if r.failNextStart {
			r.failNextStart = false
			return runResult{stderr: "start failed"}, errors.New("exit status 1")
		}
		id := args[len(args)-1]
		container, exists := r.containers[id]
		if !exists {
			return missingContainerResult(id)
		}
		if container.oneShot && strings.HasPrefix(container.restartPolicy, "on-failure") {
			// Model a crash loop observed between restarts: Docker reports a
			// restarting container as running and appends each attempt's log.
			container.running = true
			container.restarting = true
			container.status = "restarting"
			container.restartCount += crashLoopReadyRestarts
			container.log = strings.Repeat(strings.SplitAfter(container.log, "\n")[0], container.restartCount+1)
		} else if container.oneShot {
			container.running = false
			container.status = "exited"
		} else {
			container.running = true
			container.status = "running"
			container.exitCode = 0
		}
		return runResult{stdout: id + "\n"}, nil
	case hasPrefix(args, "container", "stop"):
		id := args[len(args)-1]
		container, exists := r.containers[id]
		if !exists {
			return missingContainerResult(id)
		}
		container.running = false
		container.restarting = false
		container.status = "exited"
		return runResult{stdout: id + "\n"}, nil
	case hasPrefix(args, "container", "wait"):
		id := args[len(args)-1]
		container, exists := r.containers[id]
		if !exists {
			return missingContainerResult(id)
		}
		return runResult{stdout: strconv.Itoa(container.exitCode) + "\n"}, nil
	case hasPrefix(args, "container", "logs"):
		id := args[len(args)-1]
		container, exists := r.containers[id]
		if !exists {
			return missingContainerResult(id)
		}
		if len(args) == 5 && args[2] == "--tail" {
			tail, err := strconv.Atoi(args[3])
			if err != nil {
				return runResult{}, err
			}
			lines := strings.SplitAfter(container.log, "\n")
			if lines[len(lines)-1] == "" {
				lines = lines[:len(lines)-1]
			}
			if tail < len(lines) {
				lines = lines[len(lines)-tail:]
			}
			return runResult{stdout: strings.Join(lines, "")}, nil
		}
		return runResult{stdout: container.log}, nil
	case hasPrefix(args, "container", "inspect", "--format"):
		reference := args[len(args)-1]
		if err := r.failNextInspect[reference]; err != nil {
			delete(r.failNextInspect, reference)
			return runResult{stderr: err.Error()}, err
		}
		if r.hideNextInspect[reference] {
			delete(r.hideNextInspect, reference)
			return missingContainerResult(reference)
		}
		id := r.resolveContainer(reference)
		container, exists := r.containers[id]
		if !exists {
			return missingContainerResult(id)
		}
		inspection := containerInspection{ID: id, Name: "/" + container.name, Created: container.created.Format(time.RFC3339Nano), RestartCount: container.restartCount}
		inspection.Config.Labels = cloneStrings(container.labels)
		inspection.State.Running = container.running
		inspection.State.Restarting = container.restarting
		inspection.State.Status = container.status
		inspection.State.ExitCode = container.exitCode
		if policy, retries, found := strings.Cut(container.restartPolicy, ":"); found {
			inspection.HostConfig.RestartPolicy.Name = policy
			inspection.HostConfig.RestartPolicy.MaximumRetryCount, _ = strconv.Atoi(retries)
		} else {
			inspection.HostConfig.RestartPolicy.Name = container.restartPolicy
		}
		if len(container.endpoints) > 0 {
			inspection.NetworkSettings.Networks = make(map[string]endpointState)
			for name, networkID := range container.endpoints {
				// Docker can leave NetworkID empty before a first start.
				if container.status == "created" {
					networkID = ""
				}
				inspection.NetworkSettings.Networks[name] = endpointState{NetworkID: networkID}
			}
		}
		if container.healthCmd != "" && container.running && !r.stripHealth {
			health := "unhealthy"
			if container.healthCmd == "true" {
				health = "healthy"
			}
			inspection.State.Health = &healthState{Status: health}
		}
		encoded, err := json.Marshal(inspection)
		if err != nil {
			return runResult{}, err
		}
		return runResult{stdout: string(encoded) + "\n"}, nil
	case hasPrefix(args, "container", "ls", "--all", "--quiet", "--no-trunc") && r.failManagedList && args[len(args)-1] == "label="+managedLabel+"=true":
		return runResult{stderr: "list failed"}, errors.New("exit status 1")
	case hasPrefix(args, "container", "ls", "--all", "--quiet", "--no-trunc"):
		filters := make(map[string]string)
		for index := 0; index+1 < len(args); index++ {
			if args[index] != "--filter" {
				continue
			}
			filter := strings.TrimPrefix(args[index+1], "label=")
			name, value, found := strings.Cut(filter, "=")
			if found {
				filters[name] = value
			}
		}
		var output strings.Builder
		for _, id := range r.containerOrder {
			container, exists := r.containers[id]
			if !exists || !labelsMatch(container.labels, filters) {
				continue
			}
			output.WriteString(id)
			output.WriteByte('\n')
		}
		return runResult{stdout: output.String()}, nil
	case hasPrefix(args, "network", "create"):
		r.networkCreates++
		if r.failNetworkCreate {
			return runResult{stderr: "network creation failed"}, errors.New("exit status 1")
		}
		network := &fakeNetwork{labels: make(map[string]string), name: args[len(args)-1], created: testNow}
		for index := 2; index < len(args)-1; index++ {
			switch args[index] {
			case "--internal":
				network.internal = true
			case "--label":
				labelName, value, _ := strings.Cut(args[index+1], "=")
				network.labels[labelName] = value
				index++
			case "--driver":
				index++
			}
		}
		id := fmt.Sprintf("feed%060x", r.networkCreates)
		r.networks[id] = network
		r.networkOrder = append(r.networkOrder, id)
		return runResult{stdout: id + "\n"}, nil
	case hasPrefix(args, "network", "inspect", "--format"):
		reference := args[len(args)-1]
		id := r.resolveNetwork(reference)
		network, exists := r.networks[id]
		if !exists {
			return runResult{stderr: "Error response from daemon: network " + reference + " not found"}, errors.New("exit status 1")
		}
		encoded, err := json.Marshal(networkInspection{ID: id, Name: network.name, Created: network.created.Format(time.RFC3339Nano), Driver: "bridge", Internal: network.internal, Labels: cloneStrings(network.labels)})
		if err != nil {
			return runResult{}, err
		}
		return runResult{stdout: string(encoded) + "\n"}, nil
	case hasPrefix(args, "network", "ls", "--quiet", "--no-trunc"):
		filter := strings.TrimPrefix(args[len(args)-1], "label=")
		name, value, _ := strings.Cut(filter, "=")
		var output strings.Builder
		for _, id := range r.networkOrder {
			if network, exists := r.networks[id]; exists && network.labels[name] == value {
				output.WriteString(id + "\n")
			}
		}
		return runResult{stdout: output.String()}, nil
	case hasPrefix(args, "network", "rm") && r.contextFailureOnNetworkRemove:
		return runResult{stderr: contextNotFoundMessage}, errors.New("exit status 1")
	case hasPrefix(args, "network", "rm"):
		id := args[len(args)-1]
		network, exists := r.networks[id]
		if !exists {
			return runResult{stderr: "Error response from daemon: network " + id + " not found"}, errors.New("exit status 1")
		}
		for _, container := range r.containers {
			if _, attached := container.endpoints[network.name]; attached && container.running {
				return runResult{stderr: "Error response from daemon: error while removing network: network " + network.name + " id " + id + " has active endpoints"}, errors.New("exit status 1")
			}
		}
		delete(r.networks, id)
		return runResult{stdout: id + "\n"}, nil
	case hasPrefix(args, "network", "connect") || hasPrefix(args, "network", "disconnect"):
		networkID, containerID := args[len(args)-2], args[len(args)-1]
		network, exists := r.networks[networkID]
		if !exists {
			return runResult{stderr: "Error response from daemon: network " + networkID + " not found"}, errors.New("exit status 1")
		}
		container, exists := r.containers[containerID]
		if !exists {
			return missingContainerResult(containerID)
		}
		_, attached := container.endpoints[network.name]
		if args[1] == "disconnect" {
			if !attached {
				return runResult{stderr: "Error response from daemon: container " + containerID + " is not connected to network " + network.name}, errors.New("exit status 1")
			}
			delete(container.endpoints, network.name)
			return runResult{}, nil
		}
		if _, disabled := container.endpoints["none"]; disabled {
			return runResult{stderr: "Error response from daemon: container cannot be connected to multiple networks with one of the networks in private (none) mode"}, errors.New("exit status 1")
		}
		if attached {
			return runResult{stderr: "Error response from daemon: endpoint with name " + container.name + " already exists in network " + network.name}, errors.New("exit status 1")
		}
		if container.endpoints == nil {
			container.endpoints = make(map[string]string)
		}
		container.endpoints[network.name] = networkID
		return runResult{}, nil
	case len(args) == 3 && hasPrefix(args, "container", "rm"):
		id := args[len(args)-1]
		container, exists := r.containers[id]
		if !exists {
			return missingContainerResult(id)
		}
		if container.running {
			return runResult{stderr: "Error response from daemon: cannot remove container: container is running: stop the container before removing or force remove"}, errors.New("exit status 1")
		}
		delete(r.containers, id)
		return runResult{stdout: id + "\n"}, nil
	case hasPrefix(args, "container", "rm", "--force"):
		id := args[len(args)-1]
		if err := r.failNextRemove[id]; err != nil {
			delete(r.failNextRemove, id)
			return runResult{stderr: err.Error()}, err
		}
		if _, exists := r.containers[id]; !exists {
			return missingContainerResult(id)
		}
		delete(r.containers, id)
		return runResult{stdout: id + "\n"}, nil
	default:
		return runResult{stderr: "unexpected fake Docker invocation"}, fmt.Errorf("unexpected Docker args: %q", args)
	}
}

func (r *fakeDockerRunner) resolveContainer(reference string) string {
	if _, exists := r.containers[reference]; exists {
		return reference
	}
	for id, container := range r.containers {
		if container.name == reference {
			return id
		}
	}
	return reference
}

func (r *fakeDockerRunner) resolveNetwork(reference string) string {
	if _, exists := r.networks[reference]; exists {
		return reference
	}
	for id, network := range r.networks {
		if network.name == reference {
			return id
		}
	}
	return reference
}

func (r *fakeDockerRunner) seedNetwork(id string, network *fakeNetwork) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.networks[id] = network
	r.networkOrder = append(r.networkOrder, id)
}

func (r *fakeDockerRunner) hasNetwork(id string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	_, exists := r.networks[id]
	return exists
}

func (r *fakeDockerRunner) networkCount() int {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return len(r.networks)
}

func labelsMatch(labels, filters map[string]string) bool {
	for name, value := range filters {
		if labels[name] != value {
			return false
		}
	}
	return true
}

func (r *fakeDockerRunner) seedContainer(id string, container *fakeContainer) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.containers[id] = container
	r.containerOrder = append(r.containerOrder, id)
}

func (r *fakeDockerRunner) hasContainer(id string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	_, exists := r.containers[id]
	return exists
}

func (r *fakeDockerRunner) resetCalls() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.calls = nil
}

func (r *fakeDockerRunner) snapshotCalls() [][]string {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	result := make([][]string, len(r.calls))
	for index := range r.calls {
		result[index] = append([]string(nil), r.calls[index]...)
	}
	return result
}

func (r *fakeDockerRunner) containerCount() int {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return len(r.containers)
}

func (r *fakeDockerRunner) corruptLabel(id, name, value string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.containers[id].labels[name] = value
}

func (r *fakeDockerRunner) failRemoveOnce(id string, err error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.failNextRemove[id] = err
}

func (r *fakeDockerRunner) removeContainer(id string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	delete(r.containers, id)
}

func missingContainerResult(id string) (runResult, error) {
	return runResult{stderr: "Error: No such container: " + id}, errors.New("exit status 1")
}

func hasPrefix(actual []string, prefix ...string) bool {
	return len(actual) >= len(prefix) && reflect.DeepEqual(actual[:len(prefix)], prefix)
}

func cloneStrings(source map[string]string) map[string]string {
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func testDockerMission() mission.Mission {
	return mission.Mission{
		ID:          "docker-container-census",
		Number:      17,
		Track:       mission.TrackDocker,
		Environment: mission.EnvironmentDocker,
		Docker: &mission.DockerSetup{
			Images: []mission.DockerImageSpec{{Alias: "fixture", Reference: testImageReference}},
			Containers: []mission.DockerContainerSpec{
				{Name: "api", Image: "fixture", State: "stopped"},
				{Name: "metrics", Image: "fixture", State: "running"},
			},
		},
	}
}

func testDiagnosticDockerMission() mission.Mission {
	exitCode := 23
	return mission.Mission{
		ID:          "docker-diagnostic",
		Number:      21,
		Track:       mission.TrackDocker,
		Environment: mission.EnvironmentDocker,
		Docker: &mission.DockerSetup{
			Images: []mission.DockerImageSpec{{Alias: "fixture", Reference: testImageReference}},
			Containers: []mission.DockerContainerSpec{{
				Name:     "job",
				Image:    "fixture",
				State:    mission.DockerStateStopped,
				Log:      "ERROR: literal $(touch /host); still data",
				ExitCode: &exitCode,
			}},
		},
	}
}

func testFactory(commandRunner runner) *Factory {
	factory := newFactory(game.SandboxFactory{}, commandRunner, nil, func() (string, error) {
		return strings.Repeat("a", 24), nil
	})
	factory.owner = testOwner(func(pid int) bool { return pid == testOwnerPID })
	factory.pollInterval = time.Millisecond
	return factory
}

func testOwner(alive func(int) bool) processOwner {
	return processOwner{pid: testOwnerPID, host: testOwnerHost, alive: alive, now: func() time.Time { return testNow }}
}

// firstCall returns the first recorded Docker invocation with prefix and its
// position, so setup assertions do not depend on unrelated preflight calls.
func firstCall(t *testing.T, calls [][]string, prefix ...string) (int, []string) {
	t.Helper()
	for index, call := range calls {
		if hasPrefix(call, prefix...) {
			return index, call
		}
	}
	t.Fatalf("no Docker call with prefix %q in %q", prefix, calls)
	return -1, nil
}

func createTestEnvironment(t *testing.T, commandRunner *fakeDockerRunner) *environment {
	t.Helper()
	created, err := testFactory(commandRunner).Create(context.Background(), testDockerMission())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	environment, ok := created.(*environment)
	if !ok {
		t.Fatalf("Create() type = %T, want *environment", created)
	}
	t.Cleanup(func() {
		_ = environment.Close()
	})
	return environment
}

func createEnvironmentForMission(t *testing.T, commandRunner *fakeDockerRunner, item mission.Mission) *environment {
	t.Helper()
	created, err := testFactory(commandRunner).Create(context.Background(), item)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	environment, ok := created.(*environment)
	if !ok {
		t.Fatalf("Create() type = %T, want *environment", created)
	}
	t.Cleanup(func() {
		_ = environment.Close()
	})
	return environment
}

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

type execNotFoundError struct{}

func (execNotFoundError) Error() string { return "executable not found" }

func TestCreateUsesExactLabelsAndResourceLimits(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	environment := createTestEnvironment(t, commandRunner)
	calls := commandRunner.snapshotCalls()
	if len(calls) < 6 {
		t.Fatalf("setup calls = %q", calls)
	}
	wantCreate := []string{
		"container", "create",
		"--pull", "never",
		"--name", "opsquest-aaaaaaaaaaaaaaaaaaaaaaaa-c01",
		"--label", "com.opsquest.managed=true",
		"--label", "com.opsquest.schema=1",
		"--label", "com.opsquest.session=aaaaaaaaaaaaaaaaaaaaaaaa",
		"--label", "com.opsquest.mission=docker-container-census",
		"--label", "com.opsquest.alias=api",
		"--label", "com.opsquest.owner-pid=4242",
		"--label", "com.opsquest.owner-host=test-host",
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
		testImageReference,
		"86400",
	}
	createIndex, create := firstCall(t, calls, "container", "create")
	if !reflect.DeepEqual(create, wantCreate) {
		t.Fatalf("first create args:\n got: %q\nwant: %q", create, wantCreate)
	}
	metricsID := environment.byAlias["metrics"].id
	if !reflect.DeepEqual(calls[createIndex+2], []string{"container", "start", metricsID}) {
		t.Fatalf("running fixture start = %q", calls[createIndex+2])
	}
	for _, call := range calls {
		joined := strings.Join(call, " ")
		for _, forbidden := range []string{"--privileged", "--volume", "--mount", "/var/run/docker.sock", "--network host"} {
			if strings.Contains(joined, forbidden) {
				t.Errorf("unsafe argument %q in %q", forbidden, call)
			}
		}
	}
}

func TestDiagnosticFixtureUsesFixedCommandAndExposesSanitizedObservations(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	environment := createEnvironmentForMission(t, commandRunner, testDiagnosticDockerMission())
	calls := commandRunner.snapshotCalls()
	if len(calls) < 6 {
		t.Fatalf("setup calls = %q", calls)
	}
	logText := "ERROR: literal $(touch /host); still data"
	wantTail := []string{
		"--entrypoint", "/bin/sh",
		testImageReference,
		"-c", `printf '%s\n' "$1"; exit "$2"`,
		"opsquest-diagnostic", logText, "23",
	}
	createIndex, create := firstCall(t, calls, "container", "create")
	if len(create) < len(wantTail) || !reflect.DeepEqual(create[len(create)-len(wantTail):], wantTail) {
		t.Fatalf("diagnostic create tail:\n got: %q\nwant: %q", create, wantTail)
	}
	if strings.Contains(create[len(create)-4], logText) {
		t.Fatalf("diagnostic data was interpolated into shell program: %q", create[len(create)-4])
	}
	jobID := environment.byAlias["job"].id
	if !reflect.DeepEqual(calls[createIndex+1], []string{"container", "start", jobID}) || !reflect.DeepEqual(calls[createIndex+2], []string{"container", "wait", jobID}) {
		t.Fatalf("diagnostic lifecycle calls = %q", calls[createIndex+1:createIndex+3])
	}

	logged, err := environment.Execute(context.Background(), "docker container logs job")
	if err != nil || logged.Output != logText+"\n" {
		t.Fatalf("logs output = %q, error = %v", logged.Output, err)
	}
	inspected, err := environment.Execute(context.Background(), "docker container inspect job")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(inspected.Output, `"Name": "job"`) || !strings.Contains(inspected.Output, `"ExitCode": 23`) || strings.Contains(inspected.Output, jobID) || strings.Contains(inspected.Output, environment.byAlias["job"].actualName) {
		t.Fatalf("sanitized diagnostic inspection:\n%s", inspected.Output)
	}
	stopped := mission.Condition{Type: mission.ConditionDockerContainerStopped, Container: "job"}
	if got, err := environment.Observe(context.Background(), stopped); err != nil || !got {
		t.Fatalf("diagnostic stopped outcome = %v, %v", got, err)
	}
}

func TestPlayerInputIsParsedBeforeDockerTransport(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	environment := createTestEnvironment(t, commandRunner)
	commandRunner.resetCalls()
	api := environment.byAlias["api"]

	unsafe := []string{
		"docker start api; touch /tmp/escaped",
		"docker start $(whoami)",
		"docker start `whoami`",
		"docker run --privileged busybox",
		"docker start host-container",
		"docker start " + api.id,
		"docker start " + api.actualName,
		"sh -c docker ps",
		"docker ps | docker start api",
		"docker logs api --tail 20",
		"docker stop api metrics",
	}
	for _, line := range unsafe {
		if _, err := environment.Execute(context.Background(), line); err == nil {
			t.Errorf("Execute(%q) unexpectedly succeeded", line)
		}
	}
	if calls := commandRunner.snapshotCalls(); len(calls) != 0 {
		t.Fatalf("rejected player input reached Docker transport: %q", calls)
	}

	if _, err := environment.Execute(context.Background(), "docker start api"); err != nil {
		t.Fatalf("safe start error = %v", err)
	}
	if calls := commandRunner.snapshotCalls(); !reflect.DeepEqual(calls, [][]string{{"container", "start", api.id}}) {
		t.Fatalf("safe start transport calls = %q", calls)
	}
}

func TestLogicalCommandsAndOutcomeObservation(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	environment := createTestEnvironment(t, commandRunner)

	apiRunning := mission.Condition{Type: "docker_container_running", Container: "api"}
	apiStopped := mission.Condition{Type: "docker_container_stopped", Container: "api"}
	metricsRunning := mission.Condition{Type: "docker_container_running", Container: "metrics"}
	count := 2
	containerCount := mission.Condition{Type: "docker_container_count_equals", Count: &count}
	if got, err := environment.Observe(context.Background(), apiRunning); err != nil || got {
		t.Fatalf("initial api outcome = %v, %v", got, err)
	}
	if got, err := environment.Observe(context.Background(), metricsRunning); err != nil || !got {
		t.Fatalf("initial metrics outcome = %v, %v", got, err)
	}
	if got, err := environment.Observe(context.Background(), containerCount); err != nil || !got {
		t.Fatalf("container count outcome = %v, %v", got, err)
	}
	extraID := strings.Repeat("e", 64)
	commandRunner.mutex.Lock()
	commandRunner.containers[extraID] = &fakeContainer{
		labels: map[string]string{
			managedLabel: "true", schemaLabel: "1", sessionLabel: environment.sessionID, missionLabel: environment.missionID,
		},
		name: "opsquest-extra",
	}
	commandRunner.containerOrder = append(commandRunner.containerOrder, extraID)
	commandRunner.mutex.Unlock()
	if got, err := environment.Observe(context.Background(), containerCount); err != nil || got {
		t.Fatalf("container count with extra attempt-owned fixture = %v, %v", got, err)
	}
	commandRunner.mutex.Lock()
	delete(commandRunner.containers, extraID)
	commandRunner.mutex.Unlock()

	listed, err := environment.Execute(context.Background(), "docker ps")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(listed.Output, "api") || !strings.Contains(listed.Output, "metrics") {
		t.Fatalf("running list output:\n%s", listed.Output)
	}
	listed, err = environment.Execute(context.Background(), "docker container ls --all")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listed.Output, "api") || !strings.Contains(listed.Output, "metrics") {
		t.Fatalf("all list output:\n%s", listed.Output)
	}
	if strings.Contains(listed.Output, environment.byAlias["api"].id) || strings.Contains(listed.Output, environment.byAlias["api"].actualName) || strings.Contains(listed.Output, testImageReference) {
		t.Fatalf("logical list leaked engine identifiers:\n%s", listed.Output)
	}

	if _, err := environment.Execute(context.Background(), "docker restart api"); err != nil {
		t.Fatal(err)
	}
	if got, err := environment.Observe(context.Background(), apiRunning); err != nil || !got {
		t.Fatalf("api outcome after restart = %v, %v", got, err)
	}
	inspected, err := environment.Execute(context.Background(), "docker inspect api")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(inspected.Output, `"Name": "api"`) || !strings.Contains(inspected.Output, `"Running": true`) {
		t.Fatalf("logical inspect output:\n%s", inspected.Output)
	}
	if strings.Contains(inspected.Output, environment.byAlias["api"].id) || strings.Contains(inspected.Output, environment.byAlias["api"].actualName) {
		t.Fatalf("logical inspect leaked engine identifiers:\n%s", inspected.Output)
	}
	if _, err := environment.Execute(context.Background(), "docker container stop api"); err != nil {
		t.Fatal(err)
	}
	if got, err := environment.Observe(context.Background(), apiStopped); err != nil || !got {
		t.Fatalf("api stopped outcome after stop = %v, %v", got, err)
	}
}

func TestHelpAndCompletionExposeOnlyTeachingSubset(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	environment := createTestEnvironment(t, commandRunner)
	commandRunner.resetCalls()

	result, err := environment.Execute(context.Background(), "docker --help")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"docker ps", "docker start ALIAS", "docker stop ALIAS", "docker inspect ALIAS", "docker logs ALIAS"} {
		if !strings.Contains(result.Output, expected) {
			t.Errorf("help missing %q:\n%s", expected, result.Output)
		}
	}
	if calls := commandRunner.snapshotCalls(); len(calls) != 0 {
		t.Fatalf("help reached Docker transport: %q", calls)
	}

	completion := environment.CompletionSource()
	if got := completion.CommandNames(); !reflect.DeepEqual(got, []string{"docker", "help"}) {
		t.Fatalf("command completion = %q", got)
	}
	if got := completion.PathCandidates("a"); !reflect.DeepEqual(got, []game.CompletionCandidate{{Value: "api"}}) {
		t.Fatalf("alias completion = %#v", got)
	}
}

func TestCloseVerifiesOwnershipAndIsIdempotent(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	environment := createTestEnvironment(t, commandRunner)
	apiID := environment.byAlias["api"].id
	metricsID := environment.byAlias["metrics"].id
	commandRunner.resetCalls()

	if err := environment.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	want := [][]string{
		{"container", "inspect", "--format", "{{json .}}", metricsID},
		{"container", "rm", "--force", metricsID},
		{"container", "inspect", "--format", "{{json .}}", apiID},
		{"container", "rm", "--force", apiID},
	}
	if got := commandRunner.snapshotCalls(); !reflect.DeepEqual(got, want) {
		t.Fatalf("cleanup calls:\n got: %q\nwant: %q", got, want)
	}
	if commandRunner.containerCount() != 0 {
		t.Fatalf("containers after Close = %d", commandRunner.containerCount())
	}
	if err := environment.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if got := commandRunner.snapshotCalls(); !reflect.DeepEqual(got, want) {
		t.Fatalf("second Close issued more calls: %q", got)
	}
}

func TestCloseRetriesOnlyUnresolvedContainers(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	environment := createTestEnvironment(t, commandRunner)
	apiID := environment.byAlias["api"].id
	metricsID := environment.byAlias["metrics"].id
	transientFailure := errors.New("transient remove failure")
	commandRunner.failRemoveOnce(metricsID, transientFailure)
	commandRunner.resetCalls()

	firstErr := environment.Close()
	if firstErr == nil || !strings.Contains(firstErr.Error(), transientFailure.Error()) {
		t.Fatalf("first Close() error = %v, want transient failure", firstErr)
	}
	if commandRunner.containerCount() != 1 {
		t.Fatalf("containers after failed Close = %d, want only unresolved fixture", commandRunner.containerCount())
	}
	firstCalls := [][]string{
		{"container", "inspect", "--format", "{{json .}}", metricsID},
		{"container", "rm", "--force", metricsID},
		{"container", "inspect", "--format", "{{json .}}", apiID},
		{"container", "rm", "--force", apiID},
	}
	if got := commandRunner.snapshotCalls(); !reflect.DeepEqual(got, firstCalls) {
		t.Fatalf("first cleanup calls:\n got: %q\nwant: %q", got, firstCalls)
	}
	if _, err := environment.Execute(context.Background(), "docker ps"); err == nil || !strings.Contains(err.Error(), "environment is closed") {
		t.Fatalf("Execute() after failed cleanup error = %v, want closed environment", err)
	}

	if err := environment.Close(); err != nil {
		t.Fatalf("retry Close() error = %v", err)
	}
	want := append(firstCalls,
		[]string{"container", "inspect", "--format", "{{json .}}", metricsID},
		[]string{"container", "rm", "--force", metricsID},
	)
	if got := commandRunner.snapshotCalls(); !reflect.DeepEqual(got, want) {
		t.Fatalf("cleanup calls after retry:\n got: %q\nwant: %q", got, want)
	}
	if commandRunner.containerCount() != 0 {
		t.Fatalf("containers after retry = %d", commandRunner.containerCount())
	}
	if err := environment.Close(); err != nil {
		t.Fatalf("completed Close() error = %v", err)
	}
	if got := commandRunner.snapshotCalls(); !reflect.DeepEqual(got, want) {
		t.Fatalf("completed Close issued more calls: %q", got)
	}
}

func TestCloseTreatsAlreadyMissingContainerAsResolved(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	environment := createTestEnvironment(t, commandRunner)
	apiID := environment.byAlias["api"].id
	metricsID := environment.byAlias["metrics"].id
	commandRunner.removeContainer(metricsID)
	commandRunner.resetCalls()

	if err := environment.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	want := [][]string{
		{"container", "inspect", "--format", "{{json .}}", metricsID},
		{"container", "inspect", "--format", "{{json .}}", apiID},
		{"container", "rm", "--force", apiID},
	}
	if got := commandRunner.snapshotCalls(); !reflect.DeepEqual(got, want) {
		t.Fatalf("cleanup calls:\n got: %q\nwant: %q", got, want)
	}
	if commandRunner.containerCount() != 0 {
		t.Fatalf("containers after Close = %d", commandRunner.containerCount())
	}
}

func TestCloseRefusesContainerWithMismatchedOwnership(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	environment := createTestEnvironment(t, commandRunner)
	apiID := environment.byAlias["api"].id
	metricsID := environment.byAlias["metrics"].id
	commandRunner.corruptLabel(apiID, sessionLabel, "another-session")
	commandRunner.resetCalls()
	for attempt := 1; attempt <= 2; attempt++ {
		if err := environment.Close(); err == nil || !strings.Contains(err.Error(), "ownership labels do not match") {
			t.Fatalf("Close() attempt %d error = %v", attempt, err)
		}
	}
	commandRunner.mutex.Lock()
	_, apiExists := commandRunner.containers[apiID]
	_, metricsExists := commandRunner.containers[metricsID]
	commandRunner.mutex.Unlock()
	if !apiExists {
		t.Fatal("mismatched container was removed")
	}
	if metricsExists {
		t.Fatal("owned container was not removed")
	}
	want := [][]string{
		{"container", "inspect", "--format", "{{json .}}", metricsID},
		{"container", "rm", "--force", metricsID},
		{"container", "inspect", "--format", "{{json .}}", apiID},
		{"container", "inspect", "--format", "{{json .}}", apiID},
	}
	if got := commandRunner.snapshotCalls(); !reflect.DeepEqual(got, want) {
		t.Fatalf("protected cleanup calls:\n got: %q\nwant: %q", got, want)
	}

	commandRunner.corruptLabel(apiID, sessionLabel, environment.sessionID)
	if err := environment.Close(); err != nil {
		t.Fatalf("Close() after ownership repair error = %v", err)
	}
	if commandRunner.containerCount() != 0 {
		t.Fatalf("containers after ownership repair = %d", commandRunner.containerCount())
	}
}

func TestSetupFailureCleansAlreadyCreatedContainers(t *testing.T) {
	t.Run("later create fails", func(t *testing.T) {
		commandRunner := newFakeDockerRunner()
		commandRunner.failCreateAt = 2
		if _, err := testFactory(commandRunner).Create(context.Background(), testDockerMission()); err == nil {
			t.Fatal("Create() unexpectedly succeeded")
		}
		if commandRunner.containerCount() != 0 {
			t.Fatalf("containers after failed setup = %d", commandRunner.containerCount())
		}
	})

	t.Run("ambiguous create is recovered by owned name", func(t *testing.T) {
		commandRunner := newFakeDockerRunner()
		commandRunner.failAfterCreateAt = 2
		if _, err := testFactory(commandRunner).Create(context.Background(), testDockerMission()); err == nil {
			t.Fatal("Create() unexpectedly succeeded")
		}
		if commandRunner.containerCount() != 0 {
			t.Fatalf("containers after ambiguous create = %d", commandRunner.containerCount())
		}
		calls := commandRunner.snapshotCalls()
		ambiguousName := "opsquest-aaaaaaaaaaaaaaaaaaaaaaaa-c02"
		foundRecoveryInspect := false
		for _, call := range calls {
			if reflect.DeepEqual(call, []string{"container", "inspect", "--format", "{{json .}}", ambiguousName}) {
				foundRecoveryInspect = true
			}
		}
		if !foundRecoveryInspect {
			t.Fatalf("ambiguous create did not reconcile by generated name: %q", calls)
		}
	})

	t.Run("transient ambiguous-create inspection remains cleanup eligible", func(t *testing.T) {
		commandRunner := newFakeDockerRunner()
		commandRunner.failAfterCreateAt = 2
		ambiguousName := "opsquest-aaaaaaaaaaaaaaaaaaaaaaaa-c02"
		commandRunner.failNextInspect[ambiguousName] = errors.New("temporary inspect failure")

		partial, err := testFactory(commandRunner).Create(context.Background(), testDockerMission())
		if err == nil || partial == nil {
			t.Fatalf("Create() = environment %T, error %v; want partial environment and error", partial, err)
		}
		if commandRunner.containerCount() != 0 {
			t.Fatalf("containers after retried reconciliation cleanup = %d", commandRunner.containerCount())
		}
		if err := partial.Close(); err != nil {
			t.Fatalf("Close() after setup cleanup = %v", err)
		}
	})

	t.Run("delayed ambiguous create remains cleanup eligible", func(t *testing.T) {
		commandRunner := newFakeDockerRunner()
		commandRunner.failAfterCreateAt = 2
		ambiguousName := "opsquest-aaaaaaaaaaaaaaaaaaaaaaaa-c02"
		commandRunner.hideNextInspect[ambiguousName] = true

		partial, err := testFactory(commandRunner).Create(context.Background(), testDockerMission())
		if err == nil || partial == nil {
			t.Fatalf("Create() = environment %T, error %v; want partial environment and error", partial, err)
		}
		if commandRunner.containerCount() != 0 {
			t.Fatalf("containers after delayed reconciliation cleanup = %d", commandRunner.containerCount())
		}
		if err := partial.Close(); err != nil {
			t.Fatalf("Close() after setup cleanup = %v", err)
		}
	})

	t.Run("fixture start fails", func(t *testing.T) {
		commandRunner := newFakeDockerRunner()
		commandRunner.failNextStart = true
		if _, err := testFactory(commandRunner).Create(context.Background(), testDockerMission()); err == nil {
			t.Fatal("Create() unexpectedly succeeded")
		}
		if commandRunner.containerCount() != 0 {
			t.Fatalf("containers after failed start = %d", commandRunner.containerCount())
		}
	})
}

func TestConcurrentSessionsCleanOnlyTheirOwnContainers(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	sessions := []string{strings.Repeat("a", 24), strings.Repeat("d", 24)}
	index := 0
	factory := newFactory(game.SandboxFactory{}, commandRunner, nil, func() (string, error) {
		value := sessions[index]
		index++
		return value, nil
	})
	firstCreated, err := factory.Create(context.Background(), testDockerMission())
	if err != nil {
		t.Fatal(err)
	}
	secondCreated, err := factory.Create(context.Background(), testDockerMission())
	if err != nil {
		t.Fatal(err)
	}
	first := firstCreated.(*environment)
	second := secondCreated.(*environment)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if commandRunner.containerCount() != 2 {
		t.Fatalf("first cleanup affected second session; remaining = %d", commandRunner.containerCount())
	}
	for _, tracked := range second.snapshotContainers() {
		commandRunner.mutex.Lock()
		container := commandRunner.containers[tracked.id]
		commandRunner.mutex.Unlock()
		if container == nil || container.labels[sessionLabel] != sessions[1] {
			t.Fatalf("second-session fixture %s was changed", tracked.alias)
		}
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if commandRunner.containerCount() != 0 {
		t.Fatalf("containers after both closes = %d", commandRunner.containerCount())
	}
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
func (fallbackEnvironment) Close() error                            { return nil }

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

func TestLimitedBufferCapsCapturedOutput(t *testing.T) {
	buffer := newLimitedBuffer(4)
	if count, err := buffer.Write([]byte("abcdef")); err != nil || count != 6 {
		t.Fatalf("Write() = %d, %v", count, err)
	}
	if got := buffer.String(); got != "abcd" || !buffer.exceeded {
		t.Fatalf("limited buffer = %q, exceeded=%v", got, buffer.exceeded)
	}
}
