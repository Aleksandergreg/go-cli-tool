package dockerlab

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/mission"
)

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
