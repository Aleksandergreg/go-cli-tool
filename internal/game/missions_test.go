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
		"linux-workspace":       {"mkdir -p reports/daily", "touch reports/daily/summary.txt"},
		"linux-find-logs":       {`find . -name "*.log" -exec grep -l "ERROR" {} \;`},
		"linux-release-shuffle": {"cp incident-104.txt /archive/2026/incident-104.txt", "rm incident-104.txt"},
		"linux-permissions":     {"chmod 750 deploy.sh"},
		"linux-environment":     {"export DEPLOY_ENV=staging"},
		"linux-runaway":         {"ps", "kill 4242"},
		"linux-archive-rescue":  {"tar -xf status-site.tar -C /restore"},
		"linux-pipeline-report": {`grep ERROR incidents.log | awk '{print $3}' | sort | uniq > /reports/error-services.txt`},
		"linux-tail-trouble":    {"tail -n 3 gateway.log"},
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
		"docker-container-census": {"docker ps -a", "docker start api"},
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

func canonicalMissionEnvironment(item mission.Mission) (Environment, error) {
	if item.EffectiveEnvironment() == mission.EnvironmentDocker {
		return newMissionDockerEnvironment(), nil
	}
	return (SandboxFactory{}).Create(context.Background(), item)
}

type missionDockerEnvironment struct {
	running map[string]bool
	count   int
}

func newMissionDockerEnvironment() *missionDockerEnvironment {
	return &missionDockerEnvironment{running: map[string]bool{"api": false, "metrics": true}, count: 2}
}

func (e *missionDockerEnvironment) PromptLabel() string { return "docker" }

func (e *missionDockerEnvironment) Execute(_ context.Context, line string) (Execution, error) {
	switch strings.TrimSpace(line) {
	case "docker ps -a", "docker ps --all", "docker container ls -a", "docker container ls --all":
		return Execution{Output: "api stopped\nmetrics running\n", Commands: []string{"docker"}, PipelineWidth: 1}, nil
	case "docker start api", "docker container start api":
		e.running["api"] = true
		return Execution{Output: "api\n", Commands: []string{"docker"}, PipelineWidth: 1}, nil
	case "docker start metrics", "docker container start metrics":
		return Execution{Output: "metrics\n", Commands: []string{"docker"}, PipelineWidth: 1}, nil
	default:
		return Execution{}, fmt.Errorf("unsupported fake Docker command %q", line)
	}
}

func (e *missionDockerEnvironment) Observe(_ context.Context, condition mission.Condition) (bool, error) {
	switch condition.Type {
	case "docker_container_running":
		return e.running[condition.Container], nil
	case "docker_container_count_equals":
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
		environment := newMissionDockerEnvironment()
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
		environment := newMissionDockerEnvironment()
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
		environment := newMissionDockerEnvironment()
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

func TestSearchMissionRejectsUnfilteredFindOutput(t *testing.T) {
	catalog, _ := mission.LoadCatalog()
	item, _ := catalog.Find("4")
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
