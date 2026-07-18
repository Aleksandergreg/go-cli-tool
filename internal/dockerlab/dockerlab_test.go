package dockerlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
)

const testImageReference = "docker.io/library/busybox@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662"

type fakeContainer struct {
	labels  map[string]string
	running bool
	status  string
}

type fakeDockerRunner struct {
	mutex sync.Mutex

	calls          [][]string
	containers     map[string]*fakeContainer
	containerOrder []string
	daemonErr      error
	imageAvailable bool
	failCreateAt   int
	createAttempts int
	failNextStart  bool
}

func newFakeDockerRunner() *fakeDockerRunner {
	return &fakeDockerRunner{containers: make(map[string]*fakeContainer), imageAvailable: true}
}

func (r *fakeDockerRunner) run(ctx context.Context, args ...string) (runResult, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if err := ctx.Err(); err != nil {
		return runResult{}, err
	}
	r.calls = append(r.calls, append([]string(nil), args...))

	switch {
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
		id := strings.Repeat(string(rune('b'+len(r.containerOrder))), 64)
		labels := make(map[string]string)
		for index := 0; index+1 < len(args); index++ {
			if args[index] != "--label" {
				continue
			}
			name, value, found := strings.Cut(args[index+1], "=")
			if found {
				labels[name] = value
			}
		}
		r.containers[id] = &fakeContainer{labels: labels, status: "created"}
		r.containerOrder = append(r.containerOrder, id)
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
		container.running = true
		container.status = "running"
		return runResult{stdout: id + "\n"}, nil
	case hasPrefix(args, "container", "inspect", "--format"):
		id := args[len(args)-1]
		container, exists := r.containers[id]
		if !exists {
			return missingContainerResult(id)
		}
		inspection := containerInspection{ID: id}
		inspection.Config.Labels = cloneStrings(container.labels)
		inspection.State.Running = container.running
		inspection.State.Status = container.status
		encoded, err := json.Marshal(inspection)
		if err != nil {
			return runResult{}, err
		}
		return runResult{stdout: string(encoded) + "\n"}, nil
	case hasPrefix(args, "container", "rm", "--force"):
		id := args[len(args)-1]
		if _, exists := r.containers[id]; !exists {
			return missingContainerResult(id)
		}
		delete(r.containers, id)
		return runResult{stdout: id + "\n"}, nil
	default:
		return runResult{stderr: "unexpected fake Docker invocation"}, fmt.Errorf("unexpected Docker args: %q", args)
	}
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

func testFactory(commandRunner runner) *Factory {
	return newFactory(game.SandboxFactory{}, commandRunner, nil, func() (string, error) {
		return strings.Repeat("a", 24), nil
	})
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

func TestAvailabilityDistinguishesOptionalDockerFailures(t *testing.T) {
	item := testDockerMission()
	t.Run("CLI missing", func(t *testing.T) {
		factory := newFactory(game.SandboxFactory{}, nil, execNotFoundError{}, nil)
		availability := factory.Availability(context.Background(), item)
		if availability.Available || !strings.Contains(availability.Detail, "executable not found") || !strings.Contains(availability.Detail, "Linux missions remain available") {
			t.Fatalf("availability = %#v", availability)
		}
	})

	t.Run("daemon unavailable", func(t *testing.T) {
		commandRunner := newFakeDockerRunner()
		commandRunner.daemonErr = errors.New("Cannot connect to the Docker daemon")
		availability := testFactory(commandRunner).Availability(context.Background(), item)
		if availability.Available || !strings.Contains(availability.Detail, "daemon could not be reached") {
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

	t.Run("ready", func(t *testing.T) {
		availability := testFactory(newFakeDockerRunner()).Availability(context.Background(), item)
		if !availability.Available {
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
	if len(calls) < 5 {
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
	if !reflect.DeepEqual(calls[2], wantCreate) {
		t.Fatalf("first create args:\n got: %q\nwant: %q", calls[2], wantCreate)
	}
	metricsID := environment.byAlias["metrics"].id
	if !reflect.DeepEqual(calls[4], []string{"container", "start", metricsID}) {
		t.Fatalf("running fixture start = %q", calls[4])
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
}

func TestHelpAndCompletionExposeOnlyTeachingSubset(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	environment := createTestEnvironment(t, commandRunner)
	commandRunner.resetCalls()

	result, err := environment.Execute(context.Background(), "docker --help")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"docker ps", "docker start ALIAS", "docker inspect ALIAS"} {
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

func TestCloseRefusesContainerWithMismatchedOwnership(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	environment := createTestEnvironment(t, commandRunner)
	apiID := environment.byAlias["api"].id
	commandRunner.corruptLabel(apiID, sessionLabel, "another-session")
	if err := environment.Close(); err == nil || !strings.Contains(err.Error(), "ownership labels do not match") {
		t.Fatalf("Close() error = %v", err)
	}
	commandRunner.mutex.Lock()
	_, apiExists := commandRunner.containers[apiID]
	commandRunner.mutex.Unlock()
	if !apiExists {
		t.Fatal("mismatched container was removed")
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
	if availability.Available || !strings.Contains(availability.Detail, "digest-pinned") {
		t.Fatalf("availability = %#v", availability)
	}
	if len(commandRunner.snapshotCalls()) != 0 {
		t.Fatalf("invalid setup reached Docker runner: %q", commandRunner.snapshotCalls())
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
