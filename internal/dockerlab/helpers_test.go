package dockerlab

import (
	"context"
	"reflect"
	"strings"
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

func hasPrefix(actual []string, prefix ...string) bool {
	return len(actual) >= len(prefix) && reflect.DeepEqual(actual[:len(prefix)], prefix)
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

type execNotFoundError struct{}

func (execNotFoundError) Error() string { return "executable not found" }
