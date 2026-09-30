package dockerlab

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
)

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
