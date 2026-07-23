package game

import (
	"context"
	"fmt"
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
		"docker-container-census":    {"docker ps -a", "docker start api"},
		"docker-last-broadcast":      {"docker logs checkout"},
		"docker-exit-code-detective": {"docker inspect seed", "docker inspect migrate"},
		"docker-quiet-worker":        {"docker ps", "docker stop worker"},
		"docker-recovery-pair":       {"docker ps -a", "docker start frontend", "docker start backend"},
		"docker-shift-handoff":       {"docker ps -a", "docker start standby", "docker stop retiring"},
		"linux-runbook-runner":       {"sh publish-health.sh"},
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
	logs      map[string]string
	exitCodes map[string]int
	oneShot   map[string]bool
	count     int
}

func newMissionDockerEnvironment(item mission.Mission) *missionDockerEnvironment {
	environment := &missionDockerEnvironment{
		running:   make(map[string]bool),
		logs:      make(map[string]string),
		exitCodes: make(map[string]int),
		oneShot:   make(map[string]bool),
	}
	if item.Docker == nil {
		return environment
	}
	for _, container := range item.Docker.Containers {
		environment.aliases = append(environment.aliases, container.Name)
		environment.running[container.Name] = container.State == mission.DockerStateRunning
		environment.logs[container.Name] = container.Log
		if container.ExitCode != nil {
			environment.exitCodes[container.Name] = *container.ExitCode
			environment.oneShot[container.Name] = true
		}
	}
	environment.count = len(item.Docker.Containers)
	return environment
}

func (e *missionDockerEnvironment) PromptLabel() string { return "docker" }

func (e *missionDockerEnvironment) Execute(_ context.Context, line string) (Execution, error) {
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[0] != "docker" {
		return Execution{}, fmt.Errorf("unsupported fake Docker command %q", line)
	}
	fields = fields[1:]
	if fields[0] == "container" {
		fields = fields[1:]
	}
	result := Execution{PracticedCommands: []string{"docker"}, PipelineWidth: 1}
	if len(fields) >= 1 && (fields[0] == "ps" || fields[0] == "ls") {
		all := len(fields) == 2 && (fields[1] == "-a" || fields[1] == "--all")
		if len(fields) > 2 || len(fields) == 2 && !all {
			return Execution{}, fmt.Errorf("unsupported fake Docker command %q", line)
		}
		var output strings.Builder
		for _, alias := range e.aliases {
			if !all && !e.running[alias] {
				continue
			}
			status := "stopped"
			if e.running[alias] {
				status = "running"
			}
			fmt.Fprintf(&output, "%s %s\n", alias, status)
		}
		result.Output = output.String()
		return result, nil
	}
	if len(fields) != 2 {
		return Execution{}, fmt.Errorf("unsupported fake Docker command %q", line)
	}
	action, alias := fields[0], fields[1]
	if _, exists := e.running[alias]; !exists {
		return Execution{}, fmt.Errorf("unknown fake Docker alias %q", alias)
	}
	switch action {
	case "start", "restart":
		e.running[alias] = !e.oneShot[alias]
		result.Output = alias + "\n"
	case "stop":
		e.running[alias] = false
		result.Output = alias + "\n"
	case "logs":
		result.Output = e.logs[alias] + "\n"
	case "inspect":
		status := "exited"
		if e.running[alias] {
			status = "running"
		}
		result.Output = fmt.Sprintf("{\n  \"Name\": %q,\n  \"State\": {\n    \"Running\": %t,\n    \"Status\": %q,\n    \"ExitCode\": %d\n  }\n}\n", alias, e.running[alias], status, e.exitCodes[alias])
	default:
		return Execution{}, fmt.Errorf("unsupported fake Docker command %q", line)
	}
	return result, nil
}

func (e *missionDockerEnvironment) Observe(_ context.Context, condition mission.Condition) (bool, error) {
	switch condition.Type {
	case mission.ConditionDockerContainerRunning:
		return e.running[condition.Container], nil
	case mission.ConditionDockerContainerStopped:
		_, exists := e.running[condition.Container]
		return exists && !e.running[condition.Container], nil
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
