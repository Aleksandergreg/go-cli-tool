package dockerlab

import (
	"context"
	"os"
	"strings"
	"testing"

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
