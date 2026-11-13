package dockerlab

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/game"
)

// sameCleanupCalls compares Docker invocations per resource. Close cleans
// resources concurrently, so only the order of calls that target the same
// exact ID, such as inspect before rm, is deterministic.
func sameCleanupCalls(got, want [][]string) bool {
	byResource := func(calls [][]string) map[string][][]string {
		grouped := make(map[string][][]string)
		for _, call := range calls {
			resource := call[len(call)-1]
			grouped[resource] = append(grouped[resource], call)
		}
		return grouped
	}
	return len(got) == len(want) && reflect.DeepEqual(byResource(got), byResource(want))
}

func TestCloseCleansResourcesConcurrentlyWithinOneBudget(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	environment := createTestEnvironment(t, commandRunner)
	gate := &concurrencyGate{runner: commandRunner, want: len(environment.containers), release: make(chan struct{})}
	environment.runner = gate

	if err := environment.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if gate.timedOut {
		t.Fatal("Close() inspected fixtures one at a time instead of concurrently")
	}
	if commandRunner.containerCount() != 0 {
		t.Fatalf("containers after Close = %d", commandRunner.containerCount())
	}
}

func TestCleanupConcurrentlyBoundsParallelismAndKeepsFailureOrder(t *testing.T) {
	items := make([]int, cleanupParallelism*3)
	for index := range items {
		items[index] = index
	}
	var mutex sync.Mutex
	running, peak := 0, 0
	unresolved, cleanupErrors := cleanupConcurrently(items, func(item int) error {
		mutex.Lock()
		running++
		peak = max(peak, running)
		mutex.Unlock()
		time.Sleep(time.Millisecond)
		mutex.Lock()
		running--
		mutex.Unlock()
		if item%10 == 0 {
			return fmt.Errorf("item %d", item)
		}
		return nil
	})
	if peak < 2 || peak > cleanupParallelism {
		t.Fatalf("peak parallelism = %d, want between 2 and %d", peak, cleanupParallelism)
	}
	if want := []int{20, 10, 0}; !reflect.DeepEqual(unresolved, want) {
		t.Fatalf("unresolved = %v, want %v", unresolved, want)
	}
	if len(cleanupErrors) != 3 || cleanupErrors[0].Error() != "item 20" {
		t.Fatalf("cleanup errors = %v", cleanupErrors)
	}
}

// concurrencyGate holds every first inspection until all fixtures are being
// inspected at once, which only concurrent cleanup can achieve.
type concurrencyGate struct {
	runner   runner
	want     int
	mutex    sync.Mutex
	waiting  int
	release  chan struct{}
	timedOut bool
}

func (g *concurrencyGate) run(ctx context.Context, args ...string) (runResult, error) {
	if args[0] == "container" && args[1] == "inspect" {
		g.mutex.Lock()
		g.waiting++
		if g.waiting == g.want {
			close(g.release)
		}
		g.mutex.Unlock()
		select {
		case <-g.release:
		case <-time.After(2 * time.Second):
			g.mutex.Lock()
			g.timedOut = true
			g.mutex.Unlock()
		}
	}
	return g.runner.run(ctx, args...)
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
	if got := commandRunner.snapshotCalls(); !sameCleanupCalls(got, want) {
		t.Fatalf("cleanup calls:\n got: %q\nwant: %q", got, want)
	}
	if commandRunner.containerCount() != 0 {
		t.Fatalf("containers after Close = %d", commandRunner.containerCount())
	}
	if err := environment.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if got := commandRunner.snapshotCalls(); !sameCleanupCalls(got, want) {
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
	if got := commandRunner.snapshotCalls(); !sameCleanupCalls(got, firstCalls) {
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
	if got := commandRunner.snapshotCalls(); !sameCleanupCalls(got, want) {
		t.Fatalf("cleanup calls after retry:\n got: %q\nwant: %q", got, want)
	}
	if commandRunner.containerCount() != 0 {
		t.Fatalf("containers after retry = %d", commandRunner.containerCount())
	}
	if err := environment.Close(); err != nil {
		t.Fatalf("completed Close() error = %v", err)
	}
	if got := commandRunner.snapshotCalls(); !sameCleanupCalls(got, want) {
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
	if got := commandRunner.snapshotCalls(); !sameCleanupCalls(got, want) {
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
	if got := commandRunner.snapshotCalls(); !sameCleanupCalls(got, want) {
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
