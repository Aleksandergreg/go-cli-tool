package game

import (
	"context"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/mission"
)

func TestContainerCensusAcceptsAlternativeAndRejectsIncompleteOutcomes(t *testing.T) {
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, _ := catalog.Find("docker-container-census")

	t.Run("alternative container syntax", func(t *testing.T) {
		environment := newMissionDockerEnvironment(item)
		for _, command := range []string{"docker container ls --all", "docker container start api"} {
			if _, err := environment.Execute(context.Background(), command); err != nil {
				t.Fatal(err)
			}
		}
		outcomes, err := evaluateOutcomes(context.Background(), item.Validation, environment, "")
		if err != nil || !allOutcomesSatisfied(outcomes) {
			t.Fatalf("alternative outcome complete = %v, error = %v", allOutcomesSatisfied(outcomes), err)
		}
	})

	t.Run("healthy container only is incomplete", func(t *testing.T) {
		environment := newMissionDockerEnvironment(item)
		_, _ = environment.Execute(context.Background(), "docker start metrics")
		outcomes, err := evaluateOutcomes(context.Background(), item.Validation, environment, "")
		if err != nil {
			t.Fatal(err)
		}
		if allOutcomesSatisfied(outcomes) {
			t.Fatal("starting only the already-healthy container completed the mission")
		}
	})

	t.Run("replacement count is incomplete", func(t *testing.T) {
		environment := newMissionDockerEnvironment(item)
		environment.running["api"] = true
		environment.count = 3
		outcomes, err := evaluateOutcomes(context.Background(), item.Validation, environment, "")
		if err != nil {
			t.Fatal(err)
		}
		if allOutcomesSatisfied(outcomes) {
			t.Fatal("an extra replacement container completed the mission")
		}
	})
}

func TestNewDockerMissionsAcceptAlternativesAndRejectIncompleteOutcomes(t *testing.T) {
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		id          string
		alternative []string
		incomplete  []string
	}{
		{id: "docker-last-broadcast", alternative: []string{"docker container logs checkout"}, incomplete: []string{"docker inspect checkout"}},
		{id: "docker-exit-code-detective", alternative: []string{"docker container inspect migrate"}, incomplete: []string{"docker inspect seed"}},
		{id: "docker-quiet-worker", alternative: []string{"docker container stop worker"}, incomplete: []string{"docker stop metrics"}},
		{id: "docker-recovery-pair", alternative: []string{"docker container start backend", "docker container start frontend"}, incomplete: []string{"docker start frontend"}},
		{id: "docker-shift-handoff", alternative: []string{"docker container stop retiring", "docker container start standby"}, incomplete: []string{"docker start standby"}},
		{id: "docker-janitor-duty", alternative: []string{"docker container rm report-weekly", "docker container rm backup-0915", "docker container rm backup-0914"}, incomplete: []string{"docker rm backup-0914", "docker rm backup-0915"}},
		{id: "docker-janitor-duty", alternative: []string{"docker rm backup-0914", "docker rm backup-0915", "docker rm report-weekly", "docker ps -a"}, incomplete: []string{"docker stop worker", "docker rm worker", "docker rm backup-0914", "docker rm backup-0915", "docker rm report-weekly"}},
		{id: "docker-tail-end", alternative: []string{"docker container logs -n 3 ledger"}, incomplete: []string{"docker logs ledger"}},
		{id: "docker-tail-end", alternative: []string{"docker logs ledger", "docker logs --tail 3 ledger"}, incomplete: []string{"docker logs --tail 4 ledger"}},
		{id: "docker-running-isnt-healthy", alternative: []string{"docker ps --filter health=unhealthy", "docker container start web-c", "docker container stop web-b"}, incomplete: []string{"docker stop web-a", "docker start web-c"}},
		{id: "docker-running-isnt-healthy", alternative: []string{"docker stop web-b", "docker start web-c", "docker ps"}, incomplete: []string{"docker stop web-b", "docker rm web-b", "docker start web-c"}},
		{id: "docker-crash-loop", alternative: []string{"docker inspect payments", "docker container stop payments", "docker logs --tail 1 payments"}, incomplete: []string{"docker logs payments"}},
		{id: "docker-crash-loop", alternative: []string{"docker stop payments", "docker ps -a", "docker container logs payments"}, incomplete: []string{"docker stop payments", "docker logs payments", "docker rm payments"}},
		{id: "docker-postmortem-triage", alternative: []string{"docker ps --filter status=exited", "docker rm sync-users", "docker container rm sync-orders", "docker ps --filter health=unhealthy", "docker container stop api-canary"}, incomplete: []string{"docker stop api-canary", "docker rm sync-orders", "docker rm sync-users", "docker rm sync-invoices"}},
		{id: "docker-postmortem-triage", alternative: []string{"docker stop api-canary", "docker rm sync-orders", "docker rm sync-users", "docker logs sync-invoices"}, incomplete: []string{"docker stop api-canary", "docker rm api-canary", "docker rm sync-orders", "docker rm sync-users"}},
		{id: "docker-network-census", alternative: []string{"docker network ls", "docker network inspect cache-net"}, incomplete: []string{"docker network inspect web-net"}},
		{id: "docker-network-census", alternative: []string{"docker network inspect db-net", "docker network inspect cache-net"}, incomplete: []string{"docker network inspect cache-net", "docker inspect api"}},
		{id: "docker-cant-reach-the-db", alternative: []string{"docker network create api-db", "docker network connect api-db api", "docker network connect api-db db"}, incomplete: []string{"docker network connect web-net db"}},
		{id: "docker-cant-reach-the-db", alternative: []string{"docker network connect db-net api", "docker network inspect db-net"}, incomplete: []string{"docker network disconnect web-net api", "docker network connect db-net api"}},
		{id: "docker-need-to-know", alternative: []string{"docker stop batch", "docker network disconnect payments-net batch", "docker start batch"}, incomplete: []string{"docker network disconnect payments-net ledger-db"}},
		{id: "docker-need-to-know", alternative: []string{"docker network inspect app-net", "docker network disconnect payments-net batch"}, incomplete: []string{"docker network disconnect app-net batch"}},
		{id: "docker-private-channel", alternative: []string{"docker network create edge-net", "docker network connect edge-net gateway", "docker network connect edge-net api", "docker network disconnect app-net gateway", "docker network disconnect app-net api"}, incomplete: []string{"docker network create metrics-net", "docker network connect metrics-net exporter", "docker network connect metrics-net collector"}},
		{id: "docker-private-channel", alternative: []string{"docker network create metrics-net", "docker network connect metrics-net collector", "docker network connect metrics-net exporter", "docker network disconnect app-net collector", "docker network disconnect app-net exporter"}, incomplete: []string{"docker network create metrics-net", "docker network connect metrics-net exporter", "docker network connect metrics-net collector", "docker network disconnect app-net exporter", "docker network disconnect app-net collector", "docker network disconnect app-net gateway"}},
		{id: "docker-segmentation", alternative: []string{"docker network create web-net", "docker network connect web-net web", "docker network connect web-net api", "docker network disconnect flat-net web", "docker network disconnect legacy-net old-cron", "docker network rm legacy-net", "docker rm old-cron"}, incomplete: []string{"docker network create db-net", "docker network connect db-net api", "docker network connect db-net db", "docker network disconnect flat-net db", "docker rm old-cron"}},
		{id: "docker-segmentation", alternative: []string{"docker network ls", "docker rm old-cron", "docker network rm legacy-net", "docker network create db-net", "docker network connect db-net db", "docker network connect db-net api", "docker network disconnect flat-net db"}, incomplete: []string{"docker network disconnect flat-net db", "docker rm old-cron", "docker network rm legacy-net"}},
	}
	for _, test := range tests {
		item, found := catalog.Find(test.id)
		if !found {
			t.Fatalf("mission %q not found", test.id)
		}
		run := func(t *testing.T, commands []string) bool {
			t.Helper()
			environment := newMissionDockerEnvironment(item)
			lastOutput := ""
			for _, command := range commands {
				result, err := environment.Execute(context.Background(), command)
				if err != nil {
					t.Fatalf("Execute(%q) error = %v", command, err)
				}
				lastOutput = result.Output
			}
			outcomes, err := evaluateOutcomes(context.Background(), item.Validation, environment, lastOutput)
			if err != nil {
				t.Fatal(err)
			}
			return allOutcomesSatisfied(outcomes)
		}
		t.Run(test.id+" alternative", func(t *testing.T) {
			if !run(t, test.alternative) {
				t.Fatal("alternative observable outcome did not complete mission")
			}
		})
		t.Run(test.id+" incomplete", func(t *testing.T) {
			if run(t, test.incomplete) {
				t.Fatal("incomplete observable outcome completed mission")
			}
		})
	}
}
