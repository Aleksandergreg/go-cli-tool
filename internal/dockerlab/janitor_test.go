package dockerlab

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/game"
)

func orphanLabels(session, pid, host string) map[string]string {
	labels := map[string]string{
		managedLabel: "true",
		schemaLabel:  "1",
		sessionLabel: session,
		missionLabel: "docker-container-census",
		aliasLabel:   "api",
	}
	if pid != "" {
		labels[ownerPIDLabel] = pid
	}
	if host != "" {
		labels[ownerHostLabel] = host
	}
	return labels
}

func seedOrphanCandidates(commandRunner *fakeDockerRunner) (orphans, kept []string) {
	session := func(char string) string { return strings.Repeat(char, 24) }
	id := func(char string) string { return strings.Repeat(char, 64) }
	old := testNow.Add(-orphanMinimumAge - time.Minute)
	young := testNow.Add(-time.Hour)
	candidates := []struct {
		id        string
		container *fakeContainer
		orphan    bool
	}{
		// The owner process on this host has exited.
		{id: id("1"), orphan: true, container: &fakeContainer{labels: orphanLabels(session("1"), "999", testOwnerHost), name: "opsquest-" + session("1") + "-c01", created: young, running: true}},
		// The owner process on this host is alive.
		{id: id("2"), container: &fakeContainer{labels: orphanLabels(session("2"), "4242", testOwnerHost), name: "opsquest-" + session("2") + "-c01", created: young, running: true}},
		// Another host cannot be probed; a young fixture may be live.
		{id: id("3"), container: &fakeContainer{labels: orphanLabels(session("3"), "999", "other-host"), name: "opsquest-" + session("3") + "-c01", created: young}},
		// Another host's fixture past its lifetime is abandoned.
		{id: id("4"), orphan: true, container: &fakeContainer{labels: orphanLabels(session("4"), "999", "other-host"), name: "opsquest-" + session("4") + "-c02", created: old}},
		// Fixtures from before owner labels fall back to age.
		{id: id("5"), orphan: true, container: &fakeContainer{labels: orphanLabels(session("5"), "", ""), name: "opsquest-" + session("5") + "-c01", created: old}},
		{id: id("6"), container: &fakeContainer{labels: orphanLabels(session("6"), "", ""), name: "opsquest-" + session("6") + "-c01", created: young}},
		// Ownership proofs that fail keep the container regardless of age.
		{id: id("7"), container: &fakeContainer{labels: orphanLabels(session("7"), "999", testOwnerHost), name: "user-database", created: old}},
		{id: id("8"), container: &fakeContainer{labels: orphanLabels(session("8"), "999", testOwnerHost), name: "opsquest-" + session("9") + "-c01", created: old}},
		{id: id("9"), container: &fakeContainer{labels: func() map[string]string {
			labels := orphanLabels(session("9"), "999", testOwnerHost)
			labels[schemaLabel] = "2"
			return labels
		}(), name: "opsquest-" + session("9") + "-c01", created: old}},
		{id: id("a"), container: &fakeContainer{labels: func() map[string]string {
			labels := orphanLabels(session("a"), "999", testOwnerHost)
			delete(labels, aliasLabel)
			return labels
		}(), name: "opsquest-" + session("a") + "-c01", created: old}},
		{id: id("d"), container: &fakeContainer{labels: orphanLabels("not-a-session", "999", testOwnerHost), name: "opsquest-not-a-session-c01", created: old}},
		// An unparsable owner PID is not proof of death; it falls back to age.
		{id: id("e"), container: &fakeContainer{labels: orphanLabels(session("e"), "pid", testOwnerHost), name: "opsquest-" + session("e") + "-c01", created: young}},
	}
	for _, candidate := range candidates {
		commandRunner.seedContainer(candidate.id, candidate.container)
		if candidate.orphan {
			orphans = append(orphans, candidate.id)
		} else {
			kept = append(kept, candidate.id)
		}
	}
	return orphans, kept
}

func TestJanitorFindsOnlyProvablyAbandonedFixtures(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	orphans, kept := seedOrphanCandidates(commandRunner)
	factory := testFactory(commandRunner)
	var _ game.ResourceJanitor = factory

	count, err := factory.OrphanedResources(context.Background())
	if err != nil || count != len(orphans) {
		t.Fatalf("OrphanedResources() = %d, %v; want %d", count, err, len(orphans))
	}
	for _, call := range commandRunner.snapshotCalls() {
		if hasPrefix(call, "container", "rm") {
			t.Fatalf("read-only orphan check removed a container: %q", call)
		}
	}

	commandRunner.resetCalls()
	removed, err := factory.RemoveOrphanedResources(context.Background())
	if err != nil || removed != len(orphans) {
		t.Fatalf("RemoveOrphanedResources() = %d, %v; want %d", removed, err, len(orphans))
	}
	var removals [][]string
	for _, call := range commandRunner.snapshotCalls() {
		if hasPrefix(call, "container", "rm") {
			removals = append(removals, call)
		}
	}
	var wantRemovals [][]string
	for _, id := range orphans {
		wantRemovals = append(wantRemovals, []string{"container", "rm", "--force", id})
	}
	if !reflect.DeepEqual(removals, wantRemovals) {
		t.Fatalf("removals = %q, want %q", removals, wantRemovals)
	}
	for _, id := range orphans {
		if commandRunner.hasContainer(id) {
			t.Errorf("orphan %s survived cleanup", id[:12])
		}
	}
	for _, id := range kept {
		if !commandRunner.hasContainer(id) {
			t.Errorf("non-orphan %s was removed", id[:12])
		}
	}
	if count, err := factory.OrphanedResources(context.Background()); err != nil || count != 0 {
		t.Fatalf("OrphanedResources() after cleanup = %d, %v", count, err)
	}
}

func TestCreateSweepsOrphansButNeverLiveAttempts(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	orphans, kept := seedOrphanCandidates(commandRunner)
	first := createTestEnvironment(t, commandRunner)
	for _, id := range orphans {
		if commandRunner.hasContainer(id) {
			t.Errorf("Create() left orphan %s", id[:12])
		}
	}
	for _, id := range kept {
		if !commandRunner.hasContainer(id) {
			t.Errorf("Create() removed non-orphan %s", id[:12])
		}
	}

	factory := testFactory(commandRunner)
	factory.newSession = func() (string, error) { return strings.Repeat("f", 24), nil }
	second, err := factory.Create(context.Background(), testDockerMission())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	for _, tracked := range first.snapshotContainers() {
		if !commandRunner.hasContainer(tracked.id) {
			t.Fatalf("a second attempt swept live fixture %s", tracked.alias)
		}
	}
}

func TestSweepFailureDoesNotBlockSetup(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	commandRunner.failManagedList = true
	environment := createTestEnvironment(t, commandRunner)
	if len(environment.snapshotContainers()) != 2 {
		t.Fatalf("setup after failed sweep created %d fixtures", len(environment.snapshotContainers()))
	}
	if _, err := testFactory(commandRunner).OrphanedResources(context.Background()); err == nil || !strings.Contains(err.Error(), "list failed") {
		t.Fatalf("OrphanedResources() error = %v", err)
	}
}

func TestJanitorReportsMissingDocker(t *testing.T) {
	factory := newFactory(game.SandboxFactory{}, nil, execNotFoundError{}, nil)
	if _, err := factory.OrphanedResources(context.Background()); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("OrphanedResources() error = %v", err)
	}
	if _, err := factory.RemoveOrphanedResources(context.Background()); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("RemoveOrphanedResources() error = %v", err)
	}
}

func TestJanitorRejectsMalformedListing(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	commandRunner.seedContainer("not-a-container-id", &fakeContainer{labels: orphanLabels(strings.Repeat("1", 24), "999", testOwnerHost)})
	if _, err := testFactory(commandRunner).RemoveOrphanedResources(context.Background()); err == nil || !strings.Contains(err.Error(), "invalid container ID") {
		t.Fatalf("RemoveOrphanedResources() error = %v", err)
	}
}

func TestProcessAlive(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Fatal("current process reported dead")
	}
	for _, pid := range []int{0, -1} {
		if processAlive(pid) {
			t.Errorf("processAlive(%d) = true", pid)
		}
	}
}
