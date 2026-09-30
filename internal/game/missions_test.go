package game

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
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

func canonicalMissionEnvironment(item mission.Mission) (Environment, error) {
	if item.EffectiveEnvironment() == mission.EnvironmentDocker {
		return newMissionDockerEnvironment(item), nil
	}
	return (SandboxFactory{}).Create(context.Background(), item)
}

type missionDockerEnvironment struct {
	aliases   []string
	running   map[string]bool
	removed   map[string]bool
	logs      map[string]string
	exitCodes map[string]int
	oneShot   map[string]bool
	crashLoop map[string]bool
	health    map[string]string
	count     int
	networks  map[string]bool
	endpoints map[string]map[string]bool
	disabled  map[string]bool
}

func newMissionDockerEnvironment(item mission.Mission) *missionDockerEnvironment {
	environment := &missionDockerEnvironment{
		running:   make(map[string]bool),
		removed:   make(map[string]bool),
		logs:      make(map[string]string),
		exitCodes: make(map[string]int),
		oneShot:   make(map[string]bool),
		crashLoop: make(map[string]bool),
		health:    make(map[string]string),
		networks:  make(map[string]bool),
		endpoints: make(map[string]map[string]bool),
		disabled:  make(map[string]bool),
	}
	if item.Docker == nil {
		return environment
	}
	for _, network := range item.Docker.Networks {
		environment.networks[network.Name] = true
	}
	for _, container := range item.Docker.Containers {
		environment.aliases = append(environment.aliases, container.Name)
		environment.running[container.Name] = container.State == mission.DockerStateRunning
		environment.logs[container.Name] = container.Log
		environment.health[container.Name] = container.Health
		environment.endpoints[container.Name] = make(map[string]bool)
		for _, network := range container.Networks {
			environment.endpoints[container.Name][network] = true
		}
		environment.disabled[container.Name] = len(container.Networks) == 0
		if container.ExitCode != nil {
			environment.exitCodes[container.Name] = *container.ExitCode
			environment.oneShot[container.Name] = true
		}
		if container.Restart == mission.DockerRestartOnFailure {
			environment.crashLoop[container.Name] = true
			// Setup waits for a visible loop before play begins.
			environment.logs[container.Name] = strings.Repeat(container.Log+"\n", 3)
		}
	}
	environment.count = len(item.Docker.Containers)
	return environment
}

func (e *missionDockerEnvironment) PromptLabel() string { return "docker" }

func (e *missionDockerEnvironment) status(alias string) string {
	switch {
	case e.running[alias] && e.crashLoop[alias]:
		return "restarting"
	case e.running[alias] && e.health[alias] != "":
		return "running (" + e.health[alias] + ")"
	case e.running[alias]:
		return "running"
	default:
		return "exited"
	}
}

func (e *missionDockerEnvironment) Execute(_ context.Context, line string) (Execution, error) {
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[0] != "docker" {
		return Execution{}, fmt.Errorf("unsupported fake Docker command %q", line)
	}
	fields = fields[1:]
	if fields[0] == "network" {
		return e.executeNetwork(line, fields[1:])
	}
	if fields[0] == "container" {
		fields = fields[1:]
	}
	result := Execution{PracticedCommands: []string{"docker"}, PipelineWidth: 1}
	if len(fields) >= 1 && (fields[0] == "ps" || fields[0] == "ls") {
		all := false
		filters := make(map[string]string)
		for index := 1; index < len(fields); index++ {
			switch fields[index] {
			case "-a", "--all":
				all = true
			case "-f", "--filter":
				if index+1 >= len(fields) {
					return Execution{}, fmt.Errorf("unsupported fake Docker command %q", line)
				}
				index++
				key, value, _ := strings.Cut(fields[index], "=")
				filters[key] = value
			default:
				return Execution{}, fmt.Errorf("unsupported fake Docker command %q", line)
			}
		}
		var output strings.Builder
		for _, alias := range e.aliases {
			if e.removed[alias] {
				continue
			}
			if want, filtered := filters["status"]; filtered && !strings.HasPrefix(e.status(alias), want) {
				continue
			}
			if want, filtered := filters["health"]; filtered && (!e.running[alias] || e.health[alias] != want) {
				continue
			}
			if !all && filters["status"] == "" && !e.running[alias] {
				continue
			}
			fmt.Fprintf(&output, "%s %s\n", alias, e.status(alias))
		}
		result.Output = output.String()
		return result, nil
	}
	tail := -1
	if len(fields) == 4 && fields[0] == "logs" && (fields[1] == "--tail" || fields[1] == "-n") {
		parsed, err := strconv.Atoi(fields[2])
		if err != nil {
			return Execution{}, fmt.Errorf("unsupported fake Docker command %q", line)
		}
		tail = parsed
		fields = []string{"logs", fields[3]}
	}
	if len(fields) != 2 {
		return Execution{}, fmt.Errorf("unsupported fake Docker command %q", line)
	}
	action, alias := fields[0], fields[1]
	if _, exists := e.running[alias]; !exists {
		return Execution{}, fmt.Errorf("unknown fake Docker alias %q", alias)
	}
	if e.removed[alias] {
		return Execution{}, fmt.Errorf("docker: container %q has been removed", alias)
	}
	switch action {
	case "start", "restart":
		e.running[alias] = !e.oneShot[alias] || e.crashLoop[alias]
		result.Output = alias + "\n"
	case "stop":
		e.running[alias] = false
		result.Output = alias + "\n"
	case "rm":
		if e.running[alias] {
			return Execution{}, fmt.Errorf("docker: cannot remove running container %q; stop it first", alias)
		}
		e.removed[alias] = true
		e.count--
		result.Output = alias + "\n"
	case "logs":
		lines := strings.Split(strings.TrimSuffix(e.logs[alias], "\n"), "\n")
		if tail >= 0 && tail < len(lines) {
			lines = lines[len(lines)-tail:]
		}
		result.Output = strings.Join(lines, "\n") + "\n"
	case "inspect":
		result.Output = fmt.Sprintf("{\n  \"Name\": %q,\n  \"State\": {\n    \"Running\": %t,\n    \"Status\": %q,\n    \"ExitCode\": %d\n  }\n}\n", alias, e.running[alias], e.status(alias), e.exitCodes[alias])
	default:
		return Execution{}, fmt.Errorf("unsupported fake Docker command %q", line)
	}
	return result, nil
}

func (e *missionDockerEnvironment) executeNetwork(line string, fields []string) (Execution, error) {
	result := Execution{PracticedCommands: []string{"docker"}, PipelineWidth: 1}
	attached := func(network string) []string {
		var aliases []string
		for _, alias := range e.aliases {
			if !e.removed[alias] && e.endpoints[alias][network] {
				aliases = append(aliases, alias)
			}
		}
		sort.Strings(aliases)
		return aliases
	}
	switch {
	case len(fields) == 1 && fields[0] == "ls":
		var names []string
		for name, exists := range e.networks {
			if exists {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		result.Output = strings.Join(names, "\n") + "\n"
	case len(fields) == 2 && fields[0] == "inspect":
		if !e.networks[fields[1]] {
			return Execution{}, fmt.Errorf("docker: network %q no longer exists", fields[1])
		}
		encoded, err := json.MarshalIndent(struct {
			Name       string   `json:"Name"`
			Containers []string `json:"Containers"`
		}{Name: fields[1], Containers: attached(fields[1])}, "", "  ")
		if err != nil {
			return Execution{}, err
		}
		result.Output = string(encoded) + "\n"
	case len(fields) == 2 && fields[0] == "create":
		if e.networks[fields[1]] {
			return Execution{}, fmt.Errorf("docker: network %q already exists", fields[1])
		}
		e.networks[fields[1]] = true
		result.Output = fields[1] + "\n"
	case len(fields) == 2 && fields[0] == "rm":
		if !e.networks[fields[1]] {
			return Execution{}, fmt.Errorf("docker: network %q has already been removed", fields[1])
		}
		if users := attached(fields[1]); len(users) > 0 {
			return Execution{}, fmt.Errorf("docker: network %q still has containers attached (%s)", fields[1], strings.Join(users, ", "))
		}
		e.networks[fields[1]] = false
		result.Output = fields[1] + "\n"
	case len(fields) == 3 && (fields[0] == "connect" || fields[0] == "disconnect"):
		network, alias := fields[1], fields[2]
		if !e.networks[network] {
			return Execution{}, fmt.Errorf("docker: network %q has been removed", network)
		}
		if _, exists := e.running[alias]; !exists || e.removed[alias] {
			return Execution{}, fmt.Errorf("docker: container %q is not available", alias)
		}
		if fields[0] == "connect" {
			if e.disabled[alias] || e.endpoints[alias][network] {
				return Execution{}, fmt.Errorf("docker: cannot connect %q to %q", alias, network)
			}
			e.endpoints[alias][network] = true
		} else {
			if !e.endpoints[alias][network] {
				return Execution{}, fmt.Errorf("docker: container %q is not connected to network %q", alias, network)
			}
			delete(e.endpoints[alias], network)
		}
	default:
		return Execution{}, fmt.Errorf("unsupported fake Docker command %q", line)
	}
	return result, nil
}

func (e *missionDockerEnvironment) shareNetwork(first, second string) (shared, bothExist bool) {
	for _, alias := range []string{first, second} {
		if _, exists := e.running[alias]; !exists || e.removed[alias] {
			return false, false
		}
	}
	for network := range e.endpoints[first] {
		if e.networks[network] && e.endpoints[second][network] {
			return true, true
		}
	}
	return false, true
}

func (e *missionDockerEnvironment) Observe(_ context.Context, condition mission.Condition) (bool, error) {
	switch condition.Type {
	case mission.ConditionDockerNetworkShared, mission.ConditionDockerNetworkIsolated:
		shared, bothExist := e.shareNetwork(condition.Containers[0], condition.Containers[1])
		return bothExist && shared == (condition.Type == mission.ConditionDockerNetworkShared), nil
	case mission.ConditionDockerNetworkAbsent:
		return !e.networks[condition.Network], nil
	case mission.ConditionDockerContainerRunning:
		return !e.removed[condition.Container] && e.running[condition.Container], nil
	case mission.ConditionDockerContainerStopped:
		_, exists := e.running[condition.Container]
		return exists && !e.removed[condition.Container] && !e.running[condition.Container], nil
	case mission.ConditionDockerContainerAbsent:
		return e.removed[condition.Container], nil
	case mission.ConditionDockerContainerCountEqual:
		return condition.Count != nil && e.count == *condition.Count, nil
	default:
		return false, fmt.Errorf("unsupported fake Docker condition %q", condition.Type)
	}
}

func (e *missionDockerEnvironment) CompletionSource() CompletionSource { return nil }
func (e *missionDockerEnvironment) Close() error                       { return nil }

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

func TestNewLinuxMissionsAcceptAlternativesAndRejectIncompleteOutcomes(t *testing.T) {
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		id          string
		alternative []string
		incomplete  []string
	}{
		{id: "linux-read-handoff", alternative: []string{"less handoff.txt"}, incomplete: []string{"cat handoff.old"}},
		{id: "linux-log-preview", alternative: []string{"head -3 /var/log/ledger/startup.log"}, incomplete: []string{"tail -n 3 startup.log"}},
		{id: "linux-error-headcount", alternative: []string{"grep -c ERROR deploy.log"}, incomplete: []string{"grep -c WARN deploy.log"}},
		{id: "linux-runbook-runner", alternative: []string{"chmod 750 publish-health.sh", "./publish-health.sh"}, incomplete: []string{"cat publish-health.sh"}},
	}
	for _, test := range tests {
		item, found := catalog.Find(test.id)
		if !found {
			t.Fatalf("mission %q not found", test.id)
		}
		run := func(t *testing.T, commands []string) bool {
			t.Helper()
			box, err := sandbox.New(item.Setup, item.StartDir)
			if err != nil {
				t.Fatal(err)
			}
			lastOutput := ""
			for _, command := range commands {
				result, err := box.Execute(command)
				if err != nil {
					t.Fatalf("Execute(%q) error = %v", command, err)
				}
				lastOutput = result.Output
			}
			complete, err := Validate(item.Validation, box, lastOutput)
			if err != nil {
				t.Fatal(err)
			}
			return complete
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

func TestSearchMissionRejectsUnfilteredFindOutput(t *testing.T) {
	catalog, _ := mission.LoadCatalog()
	item, _ := catalog.Find("linux-find-logs")
	box, err := sandbox.New(item.Setup, item.StartDir)
	if err != nil {
		t.Fatal(err)
	}
	result, err := box.Execute(`find . -name "*.log"`)
	if err != nil {
		t.Fatal(err)
	}
	complete, err := Validate(item.Validation, box, result.Output)
	if err != nil {
		t.Fatal(err)
	}
	if complete {
		t.Fatalf("unfiltered output unexpectedly completed mission: %q", result.Output)
	}
}

func TestRebalancedMissionOutcomesRemainRouteIndependent(t *testing.T) {
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}

	run := func(t *testing.T, missionID string, commands ...string) (mission.Mission, *sandbox.Sandbox, string) {
		t.Helper()
		item, found := catalog.Find(missionID)
		if !found {
			t.Fatalf("mission %q not found", missionID)
		}
		box, err := sandbox.New(item.Setup, item.StartDir)
		if err != nil {
			t.Fatal(err)
		}
		lastOutput := ""
		for _, command := range commands {
			result, err := box.Execute(command)
			if err != nil {
				t.Fatalf("Execute(%q) error = %v", command, err)
			}
			lastOutput = result.Output
		}
		return item, box, lastOutput
	}
	assertComplete := func(t *testing.T, item mission.Mission, box *sandbox.Sandbox, output string, want bool) {
		t.Helper()
		complete, err := Validate(item.Validation, box, output)
		if err != nil {
			t.Fatal(err)
		}
		if complete != want {
			t.Fatalf("mission complete = %v, want %v", complete, want)
		}
	}

	t.Run("search accepts an explicit log-file grep route", func(t *testing.T) {
		item, box, output := run(t, "linux-find-logs", `grep -l ERROR api/*.log worker/*.log assets/*.log`)
		assertComplete(t, item, box, output, true)
	})

	t.Run("release move is complete", func(t *testing.T) {
		item, box, output := run(t, "linux-release-shuffle", "mv incident-104.txt /archive/2026/incident-104.txt")
		assertComplete(t, item, box, output, true)
	})

	t.Run("release copy without source removal is incomplete", func(t *testing.T) {
		item, box, output := run(t, "linux-release-shuffle", "cp incident-104.txt /archive/2026/incident-104.txt")
		assertComplete(t, item, box, output, false)
	})

	t.Run("pipeline accepts cut and sort unique", func(t *testing.T) {
		item, box, output := run(t, "linux-pipeline-report", `grep ERROR incidents.log | cut -d " " -f 3 | sort -u > /reports/error-services.txt`)
		assertComplete(t, item, box, output, true)
	})

	t.Run("pipeline without deduplication is incomplete", func(t *testing.T) {
		item, box, output := run(t, "linux-pipeline-report", `grep ERROR incidents.log | awk '{print $3}' | sort > /reports/error-services.txt`)
		assertComplete(t, item, box, output, false)
	})

	t.Run("production boss accepts an alternative route", func(t *testing.T) {
		item, box, output := run(t, "linux-production-friday",
			"tar -xf release.tar -C /deploy",
			`printf 'SERVICE=checkout\nLOG_LEVEL=info\n' > /deploy/app/app.env`,
			"chmod 0750 /deploy/app/deploy.sh",
			"kill -TERM 9001",
		)
		assertComplete(t, item, box, output, true)
	})

	t.Run("production boss rejects collateral process damage", func(t *testing.T) {
		item, box, output := run(t, "linux-production-friday",
			"tar -xf release.tar -C /deploy",
			`sed -i 's/LOG_LEVEL=debug/LOG_LEVEL=info/' /deploy/app/app.env`,
			"chmod 750 /deploy/app/deploy.sh",
			"kill 9001 9002",
		)
		assertComplete(t, item, box, output, false)
	})
}

func TestScriptMissionsAcceptAlternativesAndRejectIncompleteOutcomes(t *testing.T) {
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}

	run := func(t *testing.T, missionID string, commands ...string) (mission.Mission, *sandbox.Sandbox) {
		t.Helper()
		item, found := catalog.Find(missionID)
		if !found {
			t.Fatalf("mission %q not found", missionID)
		}
		box, err := sandbox.New(item.Setup, item.StartDir)
		if err != nil {
			t.Fatal(err)
		}
		for _, command := range commands {
			if _, err := box.Execute(command); err != nil {
				t.Fatalf("Execute(%q) error = %v", command, err)
			}
		}
		return item, box
	}
	assertComplete := func(t *testing.T, item mission.Mission, box *sandbox.Sandbox, want bool) {
		t.Helper()
		complete, err := Validate(item.Validation, box, "")
		if err != nil {
			t.Fatal(err)
		}
		if complete != want {
			t.Fatalf("mission complete = %v, want %v", complete, want)
		}
	}

	t.Run("different pipeline and sh execution are accepted", func(t *testing.T) {
		item, box := run(t, "linux-report-on-repeat",
			`printf '#!/bin/sh\ngrep ERROR /workspace/incidents.log | cut -d " " -f 3 | sort -u > /reports/error-services.txt\n' > error-report.sh`,
			"chmod 750 error-report.sh",
			"sh error-report.sh",
		)
		assertComplete(t, item, box, true)
	})

	t.Run("repair without execution is incomplete", func(t *testing.T) {
		item, box := run(t, "linux-report-on-repeat",
			`sed -i 's/grep WARN/grep ERROR/' error-report.sh`,
			"chmod 750 error-report.sh",
		)
		assertComplete(t, item, box, false)
	})

	t.Run("manual boss commands leak caller state", func(t *testing.T) {
		item, box := run(t, "linux-scope-creep",
			`sed -i 's/grep INFO/grep ERROR/' night-shift.sh`,
			"chmod 750 night-shift.sh",
			"cd /srv/night-shift",
			"export REPORT_LABEL=night-shift",
			`grep ERROR events.log | awk '{print $3}' | sort | uniq > /reports/night-services.txt`,
			`echo "$REPORT_LABEL" > /reports/run-label.txt`,
		)
		assertComplete(t, item, box, false)
	})

	t.Run("sh preserves boss child scope", func(t *testing.T) {
		item, box := run(t, "linux-scope-creep",
			`sed -i 's/grep INFO/grep ERROR/' night-shift.sh`,
			"chmod 750 night-shift.sh",
			"sh night-shift.sh",
		)
		assertComplete(t, item, box, true)
	})
}
