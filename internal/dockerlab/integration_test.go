package dockerlab

import (
	"context"
	"os"
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
	created, err := factory.Create(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := created.Close(); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	}()

	condition := mission.Condition{Type: "docker_container_running", Container: "api"}
	if running, err := created.Observe(context.Background(), condition); err != nil || running {
		t.Fatalf("initial api running = %v, %v", running, err)
	}
	if _, err := created.Execute(context.Background(), "docker start api"); err != nil {
		t.Fatal(err)
	}
	if running, err := created.Observe(context.Background(), condition); err != nil || !running {
		t.Fatalf("api running after start = %v, %v", running, err)
	}
}
