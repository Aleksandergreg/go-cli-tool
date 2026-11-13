package dockerlab

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/game"
)

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
