package dockerlab

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
)

func TestIntegrationRealDockerContainerCensus(t *testing.T) {
	if os.Getenv("OPSQUEST_DOCKER_TEST") != "1" {
		t.Skip("set OPSQUEST_DOCKER_TEST=1 to run the disposable Docker integration test")
	}
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, found := catalog.Find("docker-container-census")
	if !found {
		t.Fatal("docker-container-census mission is not in the embedded catalog")
	}
	factory := NewFactory(game.SandboxFactory{})
	availability := factory.Availability(context.Background(), item)
	if !availability.Available {
		t.Fatalf("Docker integration prerequisite unavailable: %s", availability.Detail)
	}
	if os.Getenv("DOCKER_CONTEXT") == orbStackContext && !strings.Contains(availability.Detail, "OrbStack") {
		t.Fatalf("OrbStack context was not identified: %s", availability.Detail)
	}
	created, err := factory.Create(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := created.Close(); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	}()
	lab, ok := created.(*environment)
	if !ok {
		t.Fatalf("Docker factory returned %T, want *environment", created)
	}

	condition := mission.Condition{Type: "docker_container_running", Container: "api"}
	if running, err := created.Observe(context.Background(), condition); err != nil || running {
		t.Fatalf("initial api running = %v, %v", running, err)
	}
	if _, err := created.Execute(context.Background(), "docker ps -a"); err != nil {
		t.Fatal(err)
	}
	if _, err := created.Execute(context.Background(), "docker start api"); err != nil {
		t.Fatal(err)
	}
	for _, outcome := range item.Validation.All {
		if satisfied, err := created.Observe(context.Background(), outcome); err != nil || !satisfied {
			t.Fatalf("outcome %#v after canonical solution = %v, %v", outcome, satisfied, err)
		}
	}
	ids := make([]string, 0, len(lab.containers))
	for _, tracked := range lab.snapshotContainers() {
		ids = append(ids, tracked.id)
	}
	if err := created.Close(); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if _, exists, err := lab.inspectReferenceUnchecked(context.Background(), id); err != nil || exists {
			t.Fatalf("fixture %s after cleanup exists = %v, error = %v", id, exists, err)
		}
	}
}

func TestIntegrationRealDockerDiagnosticsAndStop(t *testing.T) {
	if os.Getenv("OPSQUEST_DOCKER_TEST") != "1" {
		t.Skip("set OPSQUEST_DOCKER_TEST=1 to run the disposable Docker integration test")
	}
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	factory := NewFactory(game.SandboxFactory{})

	t.Run("diagnostic logs and exit status", func(t *testing.T) {
		item, found := catalog.Find("docker-last-broadcast")
		if !found {
			t.Fatal("docker-last-broadcast mission is not in the embedded catalog")
		}
		created := createRealDockerEnvironment(t, factory, item)
		logged, err := created.Execute(context.Background(), "docker logs checkout")
		if err != nil || !strings.Contains(logged.Output, "ERROR missing DATABASE_URL") {
			t.Fatalf("diagnostic logs = %q, error = %v", logged.Output, err)
		}
		inspected, err := created.Execute(context.Background(), "docker inspect checkout")
		if err != nil || !strings.Contains(inspected.Output, `"Name": "checkout"`) || !strings.Contains(inspected.Output, `"ExitCode": 78`) {
			t.Fatalf("diagnostic inspection = %q, error = %v", inspected.Output, err)
		}
		stopped := mission.Condition{Type: mission.ConditionDockerContainerStopped, Container: "checkout"}
		if satisfied, err := created.Observe(context.Background(), stopped); err != nil || !satisfied {
			t.Fatalf("diagnostic stopped outcome = %v, %v", satisfied, err)
		}
	})

	t.Run("targeted stop", func(t *testing.T) {
		item, found := catalog.Find("docker-quiet-worker")
		if !found {
			t.Fatal("docker-quiet-worker mission is not in the embedded catalog")
		}
		created := createRealDockerEnvironment(t, factory, item)
		if _, err := created.Execute(context.Background(), "docker stop worker"); err != nil {
			t.Fatal(err)
		}
		for _, outcome := range item.Validation.All {
			if satisfied, err := created.Observe(context.Background(), outcome); err != nil || !satisfied {
				t.Fatalf("outcome %#v after targeted stop = %v, %v", outcome, satisfied, err)
			}
		}
	})
}

func createRealDockerEnvironment(t *testing.T, factory *Factory, item mission.Mission) game.Environment {
	t.Helper()
	availability := factory.Availability(context.Background(), item)
	if !availability.Available {
		t.Fatalf("Docker integration prerequisite unavailable: %s", availability.Detail)
	}
	created, err := factory.Create(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := created.Close(); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	return created
}

func TestIntegrationRealDockerContainerTriage(t *testing.T) {
	if os.Getenv("OPSQUEST_DOCKER_TEST") != "1" {
		t.Skip("set OPSQUEST_DOCKER_TEST=1 to run the disposable Docker integration test")
	}
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	factory := NewFactory(game.SandboxFactory{})
	run := func(t *testing.T, created game.Environment, line string) string {
		t.Helper()
		result, err := created.Execute(context.Background(), line)
		if err != nil {
			t.Fatalf("Execute(%q) error = %v", line, err)
		}
		return result.Output
	}
	assertOutcomes := func(t *testing.T, created game.Environment, item mission.Mission, lastOutput string) {
		t.Helper()
		for _, outcome := range item.Validation.All {
			var satisfied bool
			var err error
			switch outcome.Type {
			case mission.ConditionOutputEquals:
				satisfied = strings.TrimSpace(lastOutput) == strings.TrimSpace(outcome.Value)
			case mission.ConditionOutputContains:
				satisfied = strings.Contains(lastOutput, outcome.Value)
			default:
				satisfied, err = created.Observe(context.Background(), outcome)
			}
			if err != nil || !satisfied {
				t.Fatalf("outcome %#v = %v, %v; last output %q", outcome, satisfied, err, lastOutput)
			}
		}
	}

	t.Run("health, exit codes, and rm", func(t *testing.T) {
		item, _ := catalog.Find("docker-postmortem-triage")
		created := createRealDockerEnvironment(t, factory, item)
		listed := run(t, created, "docker ps")
		if !strings.Contains(listed, "(healthy)") || !strings.Contains(listed, "(unhealthy)") {
			t.Fatalf("health did not settle before play:\n%s", listed)
		}
		unhealthy := run(t, created, "docker ps --filter health=unhealthy")
		if !strings.Contains(unhealthy, "api-canary") || strings.Contains(unhealthy, "api \n") {
			t.Fatalf("health filter:\n%s", unhealthy)
		}
		if _, err := created.Execute(context.Background(), "docker rm api-canary"); err == nil || !strings.Contains(err.Error(), "stop it first") {
			t.Fatalf("rm running error = %v", err)
		}
		run(t, created, "docker stop api-canary")
		run(t, created, "docker rm sync-orders")
		last := run(t, created, "docker rm sync-users")
		if _, err := created.Execute(context.Background(), "docker logs sync-users"); err == nil || !strings.Contains(err.Error(), "has been removed") {
			t.Fatalf("logs after rm error = %v", err)
		}
		assertOutcomes(t, created, item, last)
	})

	t.Run("crash loop", func(t *testing.T) {
		item, _ := catalog.Find("docker-crash-loop")
		created := createRealDockerEnvironment(t, factory, item)
		inspected := run(t, created, "docker inspect payments")
		if !strings.Contains(inspected, `"Name": "on-failure"`) || strings.Contains(inspected, `"RestartCount": 0`) || strings.Contains(inspected, `"RestartCount": 1,`) {
			t.Fatalf("crash-loop inspection:\n%s", inspected)
		}
		run(t, created, "docker stop payments")
		time.Sleep(2 * time.Second)
		stopped := mission.Condition{Type: mission.ConditionDockerContainerStopped, Container: "payments"}
		if satisfied, err := created.Observe(context.Background(), stopped); err != nil || !satisfied {
			t.Fatalf("manually stopped crash loop restarted: %v, %v", satisfied, err)
		}
		logs := run(t, created, "docker logs payments")
		if strings.Count(logs, "FATAL cannot read") < 2 {
			t.Fatalf("crash-loop logs = %q", logs)
		}
		assertOutcomes(t, created, item, logs)
	})

	t.Run("tail", func(t *testing.T) {
		item, _ := catalog.Find("docker-tail-end")
		created := createRealDockerEnvironment(t, factory, item)
		assertOutcomes(t, created, item, run(t, created, "docker logs --tail 3 ledger"))
	})
}

func TestIntegrationRealDockerOrphanSweep(t *testing.T) {
	if os.Getenv("OPSQUEST_DOCKER_TEST") != "1" {
		t.Skip("set OPSQUEST_DOCKER_TEST=1 to run the disposable Docker integration test")
	}
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, _ := catalog.Find("docker-quiet-worker")
	deadPID := 0
	for candidate := 4_000_000; candidate > 3_000_000; candidate-- {
		if !processAlive(candidate) {
			deadPID = candidate
			break
		}
	}
	if deadPID == 0 {
		t.Skip("no unused PID found to simulate an exited owner")
	}
	// Simulate an OpsQuest process that exited without closing its attempt.
	crashed := NewFactory(game.SandboxFactory{})
	crashed.owner.pid = deadPID
	abandoned := createRealDockerEnvironment(t, crashed, item).(*environment)

	janitor := NewFactory(game.SandboxFactory{})
	live := createRealDockerEnvironment(t, janitor, item).(*environment)
	for _, tracked := range abandoned.snapshotContainers() {
		if _, exists, err := abandoned.inspectReferenceUnchecked(context.Background(), tracked.id); err != nil || exists {
			t.Fatalf("setup did not sweep abandoned fixture %s: exists = %v, error = %v", tracked.alias, exists, err)
		}
	}
	if count, err := janitor.OrphanedResources(context.Background()); err != nil || count != 0 {
		t.Fatalf("OrphanedResources() after sweep = %d, %v", count, err)
	}
	for _, tracked := range live.snapshotContainers() {
		if _, exists, err := live.inspectReferenceUnchecked(context.Background(), tracked.id); err != nil || !exists {
			t.Fatalf("sweep removed live fixture %s: exists = %v, error = %v", tracked.alias, exists, err)
		}
	}
}
