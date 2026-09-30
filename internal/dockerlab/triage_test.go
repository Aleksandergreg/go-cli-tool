package dockerlab

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/mission"
)

func testTriageDockerMission() mission.Mission {
	success := 0
	crash := 70
	return mission.Mission{
		ID:          "docker-triage",
		Number:      30,
		Track:       mission.TrackDocker,
		Environment: mission.EnvironmentDocker,
		Docker: &mission.DockerSetup{
			Images: []mission.DockerImageSpec{{Alias: "fixture", Reference: testImageReference}},
			Containers: []mission.DockerContainerSpec{
				{Name: "web-a", Image: "fixture", State: mission.DockerStateRunning, Health: mission.DockerHealthHealthy},
				{Name: "web-b", Image: "fixture", State: mission.DockerStateRunning, Health: mission.DockerHealthUnhealthy, Log: "INFO boot\nWARN pool 80%\nERROR pool exhausted"},
				{Name: "web-c", Image: "fixture", State: mission.DockerStateStopped, Health: mission.DockerHealthHealthy},
				{Name: "report", Image: "fixture", State: mission.DockerStateStopped, Log: "report complete", ExitCode: &success},
				{Name: "payments", Image: "fixture", State: mission.DockerStateRunning, Log: "FATAL key missing", ExitCode: &crash, Restart: mission.DockerRestartOnFailure},
			},
		},
	}
}

func TestParserAcceptsTriageSubset(t *testing.T) {
	tests := []struct {
		line string
		want dockerAction
	}{
		{line: "docker rm report", want: dockerAction{kind: actionRemove, alias: "report"}},
		{line: "docker container rm report", want: dockerAction{kind: actionRemove, alias: "report"}},
		{line: "docker logs web-b", want: dockerAction{kind: actionLogs, alias: "web-b", tail: -1}},
		{line: "docker logs --tail 3 web-b", want: dockerAction{kind: actionLogs, alias: "web-b", tail: 3}},
		{line: "docker logs -n 0 web-b", want: dockerAction{kind: actionLogs, alias: "web-b", tail: 0}},
		{line: "docker container logs --tail=10000 web-b", want: dockerAction{kind: actionLogs, alias: "web-b", tail: 10000}},
		{line: "docker logs --tail all web-b", want: dockerAction{kind: actionLogs, alias: "web-b", tail: -1}},
		{line: "docker ps --filter status=exited", want: dockerAction{kind: actionList, filters: []listFilter{{key: "status", value: "exited"}}}},
		{line: "docker ps -a -f health=unhealthy", want: dockerAction{kind: actionList, all: true, filters: []listFilter{{key: "health", value: "unhealthy"}}}},
		{line: "docker container ls --filter=status=running --all", want: dockerAction{kind: actionList, all: true, filters: []listFilter{{key: "status", value: "running"}}}},
		{line: "docker ps -f status=exited -f status=created -f health=none", want: dockerAction{kind: actionList, filters: []listFilter{{key: "status", value: "exited"}, {key: "status", value: "created"}, {key: "health", value: "none"}}}},
	}
	for _, test := range tests {
		got, err := parseAction(test.line)
		if err != nil {
			t.Errorf("parseAction(%q) error = %v", test.line, err)
			continue
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("parseAction(%q) = %#v, want %#v", test.line, got, test.want)
		}
	}
}

func TestParserRejectsOutsideTriageSubsetBeforeTransport(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	environment := createTestEnvironment(t, commandRunner)
	commandRunner.resetCalls()
	tests := []struct {
		line    string
		wantErr string
	}{
		{line: "docker rm -f api", wantErr: "--force is outside"},
		{line: "docker rm api --force", wantErr: "--force is outside"},
		{line: "docker rm api metrics", wantErr: "usage: docker rm ALIAS"},
		{line: "docker rm", wantErr: "usage: docker rm ALIAS"},
		{line: "docker rm $(docker ps -aq)", wantErr: "usage: docker rm ALIAS"},
		{line: "docker logs --tail api", wantErr: "usage: docker logs"},
		{line: "docker logs --tail -1 api", wantErr: "whole number"},
		{line: "docker logs --tail 3x api", wantErr: "whole number"},
		{line: "docker logs --tail 10001 api", wantErr: "at most 10000"},
		{line: "docker logs --tail 99999999999999999999 api", wantErr: "at most 10000"},
		{line: "docker logs --tail 3 --tail 4 api", wantErr: "usage: docker logs"},
		{line: "docker logs --since 1m api", wantErr: "usage: docker logs"},
		{line: "docker logs --follow api", wantErr: "usage: docker logs"},
		{line: "docker logs api --tail 3", wantErr: "usage: docker logs"},
		{line: "docker ps --filter name=api", wantErr: "outside this teaching subset"},
		{line: "docker ps --filter label=com.opsquest.managed=true", wantErr: "outside this teaching subset"},
		{line: "docker ps --filter status=paused", wantErr: "status accepts"},
		{line: "docker ps --filter status", wantErr: "outside this teaching subset"},
		{line: "docker ps --filter", wantErr: "requires KEY=VALUE"},
		{line: "docker ps -a -a", wantErr: "usage: docker ps"},
		{line: "docker ps --format '{{.ID}}'", wantErr: "usage: docker ps"},
		{line: "docker ps -f status=exited -f status=created -f status=running -f status=restarting -f health=none", wantErr: "at most 4 filters"},
	}
	for _, test := range tests {
		if _, err := environment.Execute(context.Background(), test.line); err == nil || !strings.Contains(err.Error(), test.wantErr) {
			t.Errorf("Execute(%q) error = %v, want substring %q", test.line, err, test.wantErr)
		}
	}
	if calls := commandRunner.snapshotCalls(); len(calls) != 0 {
		t.Fatalf("rejected player input reached Docker transport: %q", calls)
	}
}

func TestRemoveOnlyStoppedContainersAndHideRemovedIdentity(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	environment := createTestEnvironment(t, commandRunner)
	api := environment.byAlias["api"]
	metricsID := environment.byAlias["metrics"].id
	commandRunner.resetCalls()

	if _, err := environment.Execute(context.Background(), "docker rm metrics"); err == nil || !strings.Contains(err.Error(), "stop it first") {
		t.Fatalf("removing running container error = %v", err)
	}
	for _, call := range commandRunner.snapshotCalls() {
		if hasPrefix(call, "container", "rm") {
			t.Fatalf("running container removal reached Docker: %q", call)
		}
	}
	if !commandRunner.hasContainer(metricsID) {
		t.Fatal("running container was removed")
	}

	absent := mission.Condition{Type: mission.ConditionDockerContainerAbsent, Container: "api"}
	if got, err := environment.Observe(context.Background(), absent); err != nil || got {
		t.Fatalf("absent before rm = %v, %v", got, err)
	}
	removed, err := environment.Execute(context.Background(), "docker container rm api")
	if err != nil || removed.Output != "api\n" {
		t.Fatalf("rm output = %q, error = %v", removed.Output, err)
	}
	if _, call := firstCall(t, commandRunner.snapshotCalls(), "container", "rm"); !reflect.DeepEqual(call, []string{"container", "rm", api.id}) {
		t.Fatalf("rm transport = %q, want exact ID without --force", call)
	}
	if got, err := environment.Observe(context.Background(), absent); err != nil || !got {
		t.Fatalf("absent after rm = %v, %v", got, err)
	}
	stopped := mission.Condition{Type: mission.ConditionDockerContainerStopped, Container: "api"}
	if got, err := environment.Observe(context.Background(), stopped); err != nil || got {
		t.Fatalf("removed container reported stopped = %v, %v", got, err)
	}
	count := 1
	if got, err := environment.Observe(context.Background(), mission.Condition{Type: mission.ConditionDockerContainerCountEqual, Count: &count}); err != nil || !got {
		t.Fatalf("count after rm = %v, %v", got, err)
	}
	listed, err := environment.Execute(context.Background(), "docker ps -a")
	if err != nil || strings.Contains(listed.Output, "api") || !strings.Contains(listed.Output, "metrics") {
		t.Fatalf("list after rm = %q, error = %v", listed.Output, err)
	}

	for _, line := range []string{"docker start api", "docker stop api", "docker restart api", "docker logs api", "docker logs --tail 1 api"} {
		_, err := environment.Execute(context.Background(), line)
		if err == nil || !strings.Contains(err.Error(), `container "api" has been removed`) {
			t.Errorf("Execute(%q) error = %v", line, err)
			continue
		}
		if strings.Contains(err.Error(), api.id) || strings.Contains(err.Error(), api.id[:12]) || strings.Contains(err.Error(), api.actualName) {
			t.Errorf("Execute(%q) leaked engine identity: %v", line, err)
		}
	}
	if _, err := environment.Execute(context.Background(), "docker rm api"); err == nil || !strings.Contains(err.Error(), "already been removed") {
		t.Fatalf("second rm error = %v", err)
	}
	if _, err := environment.Execute(context.Background(), "docker inspect api"); err == nil || !strings.Contains(err.Error(), "no longer exists") {
		t.Fatalf("inspect removed error = %v", err)
	}
	if err := environment.Close(); err != nil {
		t.Fatalf("Close() after player removal = %v", err)
	}
	if commandRunner.containerCount() != 0 {
		t.Fatalf("containers remaining after Close = %d", commandRunner.containerCount())
	}
}

func TestEngineErrorsRedactExactIdentity(t *testing.T) {
	tracked := &trackedContainer{id: strings.Repeat("c", 64), logicalID: "lab-01", alias: "api", actualName: "opsquest-" + strings.Repeat("a", 24) + "-c01"}
	message := "Error: cannot start " + tracked.id + " (" + tracked.id[:12] + ") named " + tracked.actualName
	got := redact(message, tracked.identity())
	if strings.Contains(got, tracked.id[:12]) || strings.Contains(got, tracked.actualName) || !strings.Contains(got, "lab-01") || !strings.Contains(got, "api") {
		t.Fatalf("redactIdentifiers() = %q", got)
	}
}

func TestTriageFixturesUseFixedBehaviors(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	environment := createEnvironmentForMission(t, commandRunner, testTriageDockerMission())
	calls := commandRunner.snapshotCalls()
	creates := make(map[string][]string)
	for _, call := range calls {
		if hasPrefix(call, "container", "create") {
			for index := range call {
				if call[index] == "--label" && strings.HasPrefix(call[index+1], aliasLabel+"=") {
					creates[strings.TrimPrefix(call[index+1], aliasLabel+"=")] = call
				}
			}
		}
	}
	assertArgs := func(alias string, want ...string) {
		t.Helper()
		call := creates[alias]
		for index := 0; index+len(want) <= len(call); index++ {
			if reflect.DeepEqual(call[index:index+len(want)], want) {
				return
			}
		}
		t.Errorf("%s create args %q do not contain %q", alias, call, want)
	}
	refuteArg := func(alias, flag string) {
		t.Helper()
		for _, arg := range creates[alias] {
			if arg == flag {
				t.Errorf("%s create args unexpectedly contain %q: %q", alias, flag, creates[alias])
			}
		}
	}
	assertArgs("web-a", "--health-cmd", "true", "--health-interval", "1s", "--health-timeout", "1s", "--health-retries", "1")
	assertArgs("web-b", "--health-cmd", "false")
	assertArgs("web-b", "--entrypoint", "/bin/sh", testImageReference, "-c", `printf '%s\n' "$1"; exec sleep "$2"`, "opsquest-service", "INFO boot\nWARN pool 80%\nERROR pool exhausted", "86400")
	assertArgs("web-a", "--entrypoint", "/bin/sleep", testImageReference, "86400")
	assertArgs("web-a", "--restart", "no")
	assertArgs("payments", "--restart", "on-failure:50")
	assertArgs("payments", "opsquest-diagnostic", "FATAL key missing", "70")
	refuteArg("payments", "--health-cmd")
	refuteArg("report", "--health-cmd")
	for alias, call := range creates {
		for _, forbidden := range []string{"--privileged", "--volume", "--mount", "-p", "--publish", "--network=host"} {
			for _, arg := range call {
				if arg == forbidden {
					t.Errorf("%s create args contain unsafe %q", alias, forbidden)
				}
			}
		}
	}
	for _, call := range calls {
		if hasPrefix(call, "container", "wait", environment.byAlias["payments"].id) {
			t.Fatal("setup waited for a crash-loop fixture to stop")
		}
	}

	listed, err := environment.Execute(context.Background(), "docker ps")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"running (healthy)", "running (unhealthy)", "restarting", "web-a", "web-b", "payments"} {
		if !strings.Contains(listed.Output, want) {
			t.Errorf("ps output missing %q:\n%s", want, listed.Output)
		}
	}
	if strings.Contains(listed.Output, "web-c") || strings.Contains(listed.Output, "report") {
		t.Errorf("ps without --all listed stopped containers:\n%s", listed.Output)
	}

	unhealthy, err := environment.Execute(context.Background(), "docker ps --filter health=unhealthy")
	if err != nil || !strings.Contains(unhealthy.Output, "web-b") || strings.Contains(unhealthy.Output, "web-a") || strings.Contains(unhealthy.Output, "payments") {
		t.Fatalf("health filter output = %q, error = %v", unhealthy.Output, err)
	}
	exited, err := environment.Execute(context.Background(), "docker ps --filter status=exited")
	if err != nil || !strings.Contains(exited.Output, "report") || strings.Contains(exited.Output, "web-a") || strings.Contains(exited.Output, "web-c") {
		t.Fatalf("status filter without --all output = %q, error = %v", exited.Output, err)
	}
	created, err := environment.Execute(context.Background(), "docker ps -f status=created -f status=exited")
	if err != nil || !strings.Contains(created.Output, "report") || !strings.Contains(created.Output, "web-c") || strings.Contains(created.Output, "web-a") {
		t.Fatalf("alternative status filters output = %q, error = %v", created.Output, err)
	}
	none, err := environment.Execute(context.Background(), "docker ps --filter status=running --filter health=none")
	if err != nil || strings.Contains(none.Output, "web-") || !strings.Contains(none.Output, "CONTAINER ID") {
		t.Fatalf("combined filters output = %q, error = %v", none.Output, err)
	}

	inspected, err := environment.Execute(context.Background(), "docker inspect web-b")
	if err != nil || !strings.Contains(inspected.Output, `"Health": {`) || !strings.Contains(inspected.Output, `"Status": "unhealthy"`) || !strings.Contains(inspected.Output, `"Name": "no"`) {
		t.Fatalf("health inspection = %q, error = %v", inspected.Output, err)
	}
	inspected, err = environment.Execute(context.Background(), "docker inspect payments")
	if err != nil || !strings.Contains(inspected.Output, `"RestartCount": 2`) || !strings.Contains(inspected.Output, `"Name": "on-failure"`) || !strings.Contains(inspected.Output, `"MaximumRetryCount": 50`) || !strings.Contains(inspected.Output, `"Restarting": true`) {
		t.Fatalf("crash-loop inspection = %q, error = %v", inspected.Output, err)
	}
	if strings.Contains(inspected.Output, environment.byAlias["payments"].id) || strings.Contains(inspected.Output, "Labels") || strings.Contains(inspected.Output, "Created") {
		t.Fatalf("inspection leaked engine metadata:\n%s", inspected.Output)
	}

	logs, err := environment.Execute(context.Background(), "docker logs --tail 2 web-b")
	if err != nil || logs.Output != "WARN pool 80%\nERROR pool exhausted\n" {
		t.Fatalf("tail output = %q, error = %v", logs.Output, err)
	}
	if _, call := firstCall(t, commandRunner.snapshotCalls(), "container", "logs", "--tail"); !reflect.DeepEqual(call, []string{"container", "logs", "--tail", "2", environment.byAlias["web-b"].id}) {
		t.Fatalf("tail transport = %q", call)
	}
	loopLogs, err := environment.Execute(context.Background(), "docker logs payments")
	if err != nil || strings.Count(loopLogs.Output, "FATAL key missing") < 2 {
		t.Fatalf("crash-loop logs = %q, error = %v", loopLogs.Output, err)
	}

	stopped := mission.Condition{Type: mission.ConditionDockerContainerStopped, Container: "payments"}
	if got, err := environment.Observe(context.Background(), stopped); err != nil || got {
		t.Fatalf("restarting container reported stopped = %v, %v", got, err)
	}
	if _, err := environment.Execute(context.Background(), "docker stop payments"); err != nil {
		t.Fatal(err)
	}
	if got, err := environment.Observe(context.Background(), stopped); err != nil || !got {
		t.Fatalf("stopped crash loop = %v, %v", got, err)
	}
}

func TestFixtureReadinessTimesOutAndCleansUp(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	item := testTriageDockerMission()
	// A healthy probe on a fixture that never reports health cannot settle.
	item.Docker.Containers = []mission.DockerContainerSpec{{Name: "web-a", Image: "fixture", State: mission.DockerStateRunning, Health: mission.DockerHealthHealthy}}
	factory := testFactory(commandRunner)
	factory.readyTimeout = 20 * time.Millisecond
	commandRunner.stripHealth = true
	if _, err := factory.Create(context.Background(), item); err == nil || !strings.Contains(err.Error(), "did not reach its teaching state") {
		t.Fatalf("Create() error = %v", err)
	}
	if commandRunner.containerCount() != 0 {
		t.Fatalf("containers remaining after failed readiness = %d", commandRunner.containerCount())
	}
}

func TestPlainListingKeepsOriginalLayout(t *testing.T) {
	environment := createTestEnvironment(t, newFakeDockerRunner())
	listed, err := environment.Execute(context.Background(), "docker ps")
	if err != nil {
		t.Fatal(err)
	}
	want := "CONTAINER ID  IMAGE     STATUS   NAMES\nlab-02        fixture   running  metrics\n"
	if listed.Output != want {
		t.Fatalf("docker ps output:\n%q\nwant:\n%q", listed.Output, want)
	}
}
