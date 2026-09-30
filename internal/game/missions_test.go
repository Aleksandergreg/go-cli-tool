package game

import (
	"context"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/sandbox"
)

func TestEveryMissionHasAWorkingOutcome(t *testing.T) {
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	solutions := map[string][]string{
		"linux-orientation":     {"pwd"},
		"linux-config-crawl":    {"cd /srv/web/config/live"},
		"linux-read-handoff":    {"cat handoff.txt"},
		"linux-workspace":       {"mkdir -p reports/daily", "touch reports/daily/summary.txt"},
		"linux-find-logs":       {`find . -name "*.log" -exec grep -l "ERROR" {} \;`},
		"linux-release-shuffle": {"cp incident-104.txt /archive/2026/incident-104.txt", "rm incident-104.txt"},
		"linux-permissions":     {"chmod 750 deploy.sh"},
		"linux-environment":     {"export DEPLOY_ENV=staging"},
		"linux-log-preview":     {"head -n 3 startup.log"},
		"linux-runaway":         {"ps", "kill 4242"},
		"linux-archive-rescue":  {"tar -xf status-site.tar -C /restore"},
		"linux-pipeline-report": {`grep ERROR incidents.log | awk '{print $3}' | sort | uniq > /reports/error-services.txt`},
		"linux-tail-trouble":    {"tail -n 3 gateway.log"},
		"linux-error-headcount": {"grep ERROR deploy.log | wc -l"},
		"linux-alert-counts":    {"sort raw.txt | uniq -c > /reports/alert-counts.txt"},
		"linux-config-surgery":  {`sed -i 's/LOG_LEVEL=debug/LOG_LEVEL=info/' app.env`},
		"linux-ownership":       {"chown web secrets.env", "chmod 640 secrets.env"},
		"linux-disk-usage":      {"du -b * | sort -n | tail -n 1"},
		"linux-production-friday": {
			"tar -xf release.tar -C /deploy",
			`sed -i 's/LOG_LEVEL=debug/LOG_LEVEL=info/' /deploy/app/app.env`,
			"chmod 750 /deploy/app/deploy.sh",
			"kill 9001",
		},
		"docker-container-census":     {"docker ps -a", "docker start api"},
		"docker-last-broadcast":       {"docker logs checkout"},
		"docker-exit-code-detective":  {"docker inspect seed", "docker inspect migrate"},
		"docker-quiet-worker":         {"docker ps", "docker stop worker"},
		"docker-recovery-pair":        {"docker ps -a", "docker start frontend", "docker start backend"},
		"docker-shift-handoff":        {"docker ps -a", "docker start standby", "docker stop retiring"},
		"docker-janitor-duty":         {"docker ps --filter status=exited", "docker rm backup-0914", "docker rm backup-0915", "docker rm report-weekly"},
		"docker-tail-end":             {"docker logs --tail 3 ledger"},
		"docker-running-isnt-healthy": {"docker ps", "docker stop web-b", "docker start web-c"},
		"docker-crash-loop":           {"docker ps", "docker stop payments", "docker logs payments"},
		"docker-postmortem-triage": {
			"docker ps -a",
			"docker stop api-canary",
			"docker rm sync-orders",
			"docker rm sync-users",
		},
		"docker-network-census":    {"docker inspect api", "docker network inspect cache-net"},
		"docker-cant-reach-the-db": {"docker inspect api", "docker network connect db-net api"},
		"docker-need-to-know":      {"docker network inspect payments-net", "docker network disconnect payments-net batch"},
		"docker-private-channel": {
			"docker network create metrics-net",
			"docker network connect metrics-net exporter",
			"docker network connect metrics-net collector",
			"docker network disconnect app-net exporter",
			"docker network disconnect app-net collector",
		},
		"docker-segmentation": {
			"docker network create db-net",
			"docker network connect db-net api",
			"docker network connect db-net db",
			"docker network disconnect flat-net db",
			"docker rm old-cron",
			"docker network rm legacy-net",
		},
		"linux-runbook-runner": {"sh publish-health.sh"},
		"linux-vi-first-aid": {
			`printf 'SERVICE=checkout\nLOG_LEVEL=info\n' > release.env`,
		},
		"linux-report-on-repeat": {
			`sed -i 's/grep WARN/grep ERROR/' error-report.sh`,
			"chmod 750 error-report.sh",
			"./error-report.sh",
		},
		"linux-scope-creep": {
			`sed -i 's/grep INFO/grep ERROR/' night-shift.sh`,
			"chmod 750 night-shift.sh",
			"./night-shift.sh",
		},
	}

	for _, item := range catalog.All() {
		item := item
		t.Run(item.ID, func(t *testing.T) {
			commands := solutions[item.ID]
			if len(commands) == 0 {
				t.Fatalf("mission has no canonical solution entry")
			}
			environment, err := canonicalMissionEnvironment(item)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := environment.Close(); err != nil {
					t.Errorf("Close() error = %v", err)
				}
			}()
			initial, err := evaluateOutcomes(context.Background(), item.Validation, environment, "")
			if err != nil {
				t.Fatal(err)
			}
			if allOutcomesSatisfied(initial) {
				t.Fatal("mission starts complete before its canonical solution")
			}
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
			if !allOutcomesSatisfied(outcomes) {
				t.Fatalf("canonical outcome did not complete mission; last output %q", lastOutput)
			}
		})
	}
}

func TestEveryMissionSuggestsCommandsSupportedByItsEnvironment(t *testing.T) {
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	linuxCommands := make(map[string]bool)
	for _, command := range sandbox.CommandNames() {
		linuxCommands[command] = true
	}
	for _, item := range catalog.All() {
		for _, command := range item.SuggestedCommands {
			supported := linuxCommands[command]
			if item.EffectiveEnvironment() == mission.EnvironmentDocker {
				supported = command == "docker"
			}
			if !supported {
				t.Errorf("mission %s suggests unsupported %s command %q", item.ID, item.EffectiveEnvironment(), command)
			}
		}
	}
}
