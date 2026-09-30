package dockerlab

import (
	"context"
	"strings"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/mission"
)

func TestNetworkErrorsRedactExactIdentity(t *testing.T) {
	network := &trackedNetwork{id: "feed" + strings.Repeat("9", 60), logicalID: "net-01", name: "db-net", actualName: "opsquest-" + strings.Repeat("a", 24) + "-n01"}
	message := "network " + network.id + " (" + network.id[:12] + ") named " + network.actualName + " failed"
	got := redact(message, network.identity())
	if strings.Contains(got, network.id[:12]) || strings.Contains(got, network.actualName) || !strings.Contains(got, "net-01") || !strings.Contains(got, "db-net") {
		t.Fatalf("redactNetworkIdentifiers() = %q", got)
	}
}

func TestMissingNetworkClassificationIsReferenceSpecific(t *testing.T) {
	name := "opsquest-" + strings.Repeat("a", 24) + "-n01"
	id := "feed" + strings.Repeat("0", 59) + "1"
	tests := []struct {
		name      string
		stderr    string
		reference string
		want      bool
	}{
		{name: "daemon missing name", stderr: "Error response from daemon: network " + name + " not found", reference: name, want: true},
		{name: "daemon missing ID", stderr: "Error response from daemon: network " + id + " not found", reference: id, want: true},
		{name: "legacy CLI wording", stderr: "Error: No such network: " + name, reference: name, want: true},
		{name: "context not found", stderr: contextNotFoundMessage, reference: name},
		{name: "different network", stderr: "Error response from daemon: network other-net not found", reference: name},
		{name: "daemon unreachable", stderr: "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?", reference: name},
		{name: "empty reference", stderr: "Error response from daemon: network  not found", reference: ""},
	}
	for _, test := range tests {
		if got := isMissingNetwork(runResult{stderr: test.stderr}, test.reference); got != test.want {
			t.Errorf("%s: isMissingNetwork() = %v, want %v", test.name, got, test.want)
		}
	}
}

func TestContextFailureNeitherPassesValidationNorDropsNetworkCleanup(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	environment := createEnvironmentForMission(t, commandRunner, testNetworkDockerMission())
	networks := commandRunner.networkCount()
	if networks != 3 {
		t.Fatalf("fixture networks = %d, want 3", networks)
	}

	commandRunner.setContextFailure(true)
	absent := mission.Condition{Type: mission.ConditionDockerNetworkAbsent, Network: "db-net"}
	if got, err := environment.Observe(context.Background(), absent); err == nil || got || !strings.Contains(err.Error(), "context not found") {
		t.Fatalf("Observe(network absent) during context failure = %v, %v; want error", got, err)
	}
	shared := networkCondition(mission.ConditionDockerNetworkShared, "web", "api")
	if got, err := environment.Observe(context.Background(), shared); err == nil || got {
		t.Fatalf("Observe(shared) during context failure = %v, %v; want error", got, err)
	}
	if err := environment.Close(); err == nil {
		t.Fatal("Close() during context failure reported success")
	}
	if commandRunner.networkCount() != networks {
		t.Fatalf("networks changed during failed Close: %d", commandRunner.networkCount())
	}

	commandRunner.setContextFailure(false)
	if err := environment.Close(); err != nil {
		t.Fatalf("Close() after context recovery = %v", err)
	}
	if commandRunner.networkCount() != 0 || commandRunner.containerCount() != 0 {
		t.Fatalf("resources leaked after recovered Close: %d networks, %d containers", commandRunner.networkCount(), commandRunner.containerCount())
	}
}

func TestJanitorPropagatesNetworkRemovalFailures(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	session := strings.Repeat("1", 24)
	orphan := "feed" + strings.Repeat("1", 60)
	commandRunner.seedNetwork(orphan, &fakeNetwork{labels: orphanLabels(session, "999", testOwnerHost), name: "opsquest-" + session + "-n01", created: testNow})
	factory := testFactory(commandRunner)

	commandRunner.mutex.Lock()
	commandRunner.contextFailureOnNetworkRemove = true
	commandRunner.mutex.Unlock()
	removed, err := factory.RemoveOrphanedResources(context.Background())
	if err == nil || !strings.Contains(err.Error(), "context not found") || removed != 0 {
		t.Fatalf("RemoveOrphanedResources() during context failure = %d, %v; want error and nothing counted", removed, err)
	}
	if !commandRunner.hasNetwork(orphan) {
		t.Fatal("orphaned network disappeared during a failed removal")
	}

	commandRunner.mutex.Lock()
	commandRunner.contextFailureOnNetworkRemove = false
	commandRunner.mutex.Unlock()
	if removed, err := factory.RemoveOrphanedResources(context.Background()); err != nil || removed != 1 || commandRunner.hasNetwork(orphan) {
		t.Fatalf("RemoveOrphanedResources() after recovery = %d, %v", removed, err)
	}
}
