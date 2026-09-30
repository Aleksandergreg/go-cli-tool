package dockerlab

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/mission"
)

func testNetworkDockerMission() mission.Mission {
	return mission.Mission{
		ID:          "docker-network-lab",
		Number:      35,
		Track:       mission.TrackDocker,
		Environment: mission.EnvironmentDocker,
		Docker: &mission.DockerSetup{
			Images:   []mission.DockerImageSpec{{Alias: "fixture", Reference: testImageReference}},
			Networks: []mission.DockerNetworkSpec{{Name: "web-net"}, {Name: "db-net"}, {Name: "spare-net"}},
			Containers: []mission.DockerContainerSpec{
				{Name: "web", Image: "fixture", State: mission.DockerStateRunning, Networks: []string{"web-net"}},
				{Name: "api", Image: "fixture", State: mission.DockerStateRunning, Networks: []string{"web-net"}},
				{Name: "db", Image: "fixture", State: mission.DockerStateRunning, Networks: []string{"db-net"}},
				{Name: "job", Image: "fixture", State: mission.DockerStateStopped, Networks: []string{"spare-net", "db-net"}},
				{Name: "iso", Image: "fixture", State: mission.DockerStateRunning},
			},
		},
	}
}

func networkCondition(conditionType mission.ConditionType, first, second string) mission.Condition {
	return mission.Condition{Type: conditionType, Containers: []string{first, second}}
}

func observe(t *testing.T, environment *environment, condition mission.Condition) bool {
	t.Helper()
	got, err := environment.Observe(context.Background(), condition)
	if err != nil {
		t.Fatalf("Observe(%#v) error = %v", condition, err)
	}
	return got
}

func TestNetworksAreInternalLabeledAndCreatedBeforeFixtures(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	environment := createEnvironmentForMission(t, commandRunner, testNetworkDockerMission())
	calls := commandRunner.snapshotCalls()

	networkIndex, create := firstCall(t, calls, "network", "create")
	wantCreate := []string{
		"network", "create",
		"--driver", "bridge",
		"--internal",
		"--label", "com.opsquest.managed=true",
		"--label", "com.opsquest.schema=1",
		"--label", "com.opsquest.session=aaaaaaaaaaaaaaaaaaaaaaaa",
		"--label", "com.opsquest.mission=docker-network-lab",
		"--label", "com.opsquest.alias=web-net",
		"--label", "com.opsquest.owner-pid=4242",
		"--label", "com.opsquest.owner-host=test-host",
		"opsquest-aaaaaaaaaaaaaaaaaaaaaaaa-n01",
	}
	if !reflect.DeepEqual(create, wantCreate) {
		t.Fatalf("network create args:\n got: %q\nwant: %q", create, wantCreate)
	}
	containerIndex, _ := firstCall(t, calls, "container", "create")
	if networkIndex > containerIndex {
		t.Fatal("a fixture was created before its networks")
	}
	for _, call := range calls {
		if hasPrefix(call, "network", "create") && !reflect.DeepEqual(call[2:5], []string{"--driver", "bridge", "--internal"}) {
			t.Errorf("network created without fixed internal options: %q", call)
		}
	}

	webNet, _ := environment.network("web-net")
	dbNet, _ := environment.network("db-net")
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
	containsSequence := func(call []string, want ...string) bool {
		for index := 0; index+len(want) <= len(call); index++ {
			if reflect.DeepEqual(call[index:index+len(want)], want) {
				return true
			}
		}
		return false
	}
	if !containsSequence(creates["api"], "--network", webNet.id, "--network-alias", "api") {
		t.Errorf("api create args = %q", creates["api"])
	}
	if !containsSequence(creates["iso"], "--network", "none") {
		t.Errorf("fixture without networks did not disable networking: %q", creates["iso"])
	}
	job := environment.byAlias["job"]
	connectIndex, connect := firstCall(t, calls, "network", "connect")
	if !reflect.DeepEqual(connect, []string{"network", "connect", "--alias", "job", dbNet.id, job.id}) {
		t.Fatalf("additional fixture network connect = %q", connect)
	}
	for index, call := range calls {
		if hasPrefix(call, "container", "start", job.id) && index < connectIndex {
			t.Fatal("fixture started before joining all declared networks")
		}
	}
}

func TestNetworkCommandsAndObservations(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	environment := createEnvironmentForMission(t, commandRunner, testNetworkDockerMission())
	run := func(line string) string {
		t.Helper()
		result, err := environment.Execute(context.Background(), line)
		if err != nil {
			t.Fatalf("Execute(%q) error = %v", line, err)
		}
		return result.Output
	}
	fails := func(line, want string) {
		t.Helper()
		_, err := environment.Execute(context.Background(), line)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("Execute(%q) error = %v, want substring %q", line, err, want)
		}
	}

	listed := run("docker network ls")
	for _, want := range []string{"NETWORK ID", "net-01", "web-net", "db-net", "spare-net", "bridge", "true"} {
		if !strings.Contains(listed, want) {
			t.Errorf("network ls missing %q:\n%s", want, listed)
		}
	}
	webNet, _ := environment.network("web-net")
	if strings.Contains(listed, webNet.id[:12]) || strings.Contains(listed, webNet.actualName) {
		t.Fatalf("network ls leaked engine identity:\n%s", listed)
	}
	inspected := run("docker network inspect db-net")
	for _, want := range []string{`"Id": "net-02"`, `"Name": "db-net"`, `"Internal": true`, `"db"`, `"job"`} {
		if !strings.Contains(inspected, want) {
			t.Errorf("network inspect missing %q:\n%s", want, inspected)
		}
	}
	if strings.Contains(inspected, `"api"`) || strings.Contains(inspected, "opsquest-") {
		t.Fatalf("network inspect shows wrong membership or engine names:\n%s", inspected)
	}

	shared := networkCondition(mission.ConditionDockerNetworkShared, "api", "db")
	isolated := networkCondition(mission.ConditionDockerNetworkIsolated, "api", "db")
	if observe(t, environment, shared) || !observe(t, environment, isolated) {
		t.Fatal("api and db started out sharing a network")
	}
	commandRunner.resetCalls()
	if output := run("docker network connect db-net api"); output != "" {
		t.Fatalf("connect output = %q", output)
	}
	dbNet, _ := environment.network("db-net")
	api := environment.byAlias["api"]
	if _, call := firstCall(t, commandRunner.snapshotCalls(), "network", "connect"); !reflect.DeepEqual(call, []string{"network", "connect", "--alias", "api", dbNet.id, api.id}) {
		t.Fatalf("connect transport = %q", call)
	}
	if !observe(t, environment, shared) || observe(t, environment, isolated) {
		t.Fatal("connect did not give api and db a shared network")
	}
	if inspectedAPI := run("docker inspect api"); !strings.Contains(inspectedAPI, `"db-net",`) || !strings.Contains(inspectedAPI, `"web-net"`) {
		t.Fatalf("container inspect networks:\n%s", inspectedAPI)
	}
	fails("docker network connect db-net api", "already connected")

	run("docker network disconnect web-net api")
	if observe(t, environment, networkCondition(mission.ConditionDockerNetworkShared, "web", "api")) {
		t.Fatal("disconnect left web and api sharing a network")
	}
	fails("docker network disconnect web-net api", "is not connected")
	fails("docker network connect web-net iso", "networking disabled")
	if inspectedIso := run("docker inspect iso"); !strings.Contains(inspectedIso, `"none"`) {
		t.Fatalf("isolated container inspect:\n%s", inspectedIso)
	}
	fails("docker network connect web-net ghost", `container "ghost" is not part of this mission`)
	fails("docker network connect ghost-net api", `network "ghost-net" is not part of this mission`)

	fails("docker network rm db-net", "still has containers attached (api, db, job)")
	// Docker would remove a network only stopped containers use; the lab
	// refuses so the stopped job stays startable.
	fails("docker network rm spare-net", "still has containers attached (job)")
	run("docker rm job")
	spareAbsent := mission.Condition{Type: mission.ConditionDockerNetworkAbsent, Network: "spare-net"}
	if observe(t, environment, spareAbsent) {
		t.Fatal("spare-net reported absent before removal")
	}
	if output := run("docker network rm spare-net"); output != "spare-net\n" {
		t.Fatalf("network rm output = %q", output)
	}
	if !observe(t, environment, spareAbsent) {
		t.Fatal("spare-net not absent after removal")
	}
	if strings.Contains(run("docker network ls"), "spare-net") {
		t.Fatal("removed network still listed")
	}
	fails("docker network inspect spare-net", "no longer exists")
	fails("docker network connect spare-net api", "has been removed")
	fails("docker network rm spare-net", "already been removed")

	if output := run("docker network create spare-net"); output != "net-04\n" {
		t.Fatalf("recreated network output = %q", output)
	}
	if observe(t, environment, spareAbsent) {
		t.Fatal("recreated spare-net reported absent")
	}
	fails("docker network create spare-net", "already exists")
	for _, name := range []string{"extra-a", "extra-b", "extra-c"} {
		run("docker network create " + name)
	}
	createsBefore := commandRunner.networkCreates
	fails("docker network create extra-d", "at most 4 networks")
	if commandRunner.networkCreates != createsBefore {
		t.Fatal("network over the player limit reached Docker")
	}

	run("docker stop db")
	run("docker rm db")
	if observe(t, environment, isolated) || observe(t, environment, shared) {
		t.Fatal("a removed container satisfied a network condition")
	}
}

func TestNetworkParserSubset(t *testing.T) {
	accepted := []struct {
		line string
		want dockerAction
	}{
		{line: "docker network ls", want: dockerAction{kind: actionNetworkList}},
		{line: "docker network list", want: dockerAction{kind: actionNetworkList}},
		{line: "docker network create app-net", want: dockerAction{kind: actionNetworkCreate, network: "app-net"}},
		{line: "docker network rm app-net", want: dockerAction{kind: actionNetworkRemove, network: "app-net"}},
		{line: "docker network remove app-net", want: dockerAction{kind: actionNetworkRemove, network: "app-net"}},
		{line: "docker network inspect app-net", want: dockerAction{kind: actionNetworkInspect, network: "app-net"}},
		{line: "docker network connect app-net api", want: dockerAction{kind: actionNetworkConnect, network: "app-net", alias: "api"}},
		{line: "docker network disconnect app-net api", want: dockerAction{kind: actionNetworkDisconnect, network: "app-net", alias: "api"}},
	}
	for _, test := range accepted {
		got, err := parseAction(test.line)
		if err != nil || !reflect.DeepEqual(got, test.want) {
			t.Errorf("parseAction(%q) = %#v, %v; want %#v", test.line, got, err, test.want)
		}
	}

	commandRunner := newFakeDockerRunner()
	environment := createEnvironmentForMission(t, commandRunner, testNetworkDockerMission())
	commandRunner.resetCalls()
	rejected := []struct {
		line    string
		wantErr string
	}{
		{line: "docker network", wantErr: "usage: docker network"},
		{line: "docker network create --driver host app-net", wantErr: "options are fixed"},
		{line: "docker network create --internal app-net", wantErr: "options are fixed"},
		{line: "docker network create --subnet 10.0.0.0/8 app-net", wantErr: "options are fixed"},
		{line: "docker network connect --alias db web-net api", wantErr: "options are outside"},
		{line: "docker network connect --ip 10.0.0.5 web-net api", wantErr: "options are outside"},
		{line: "docker network disconnect -f web-net api", wantErr: "options are outside"},
		{line: "docker network ls -q", wantErr: "options are outside"},
		{line: "docker network rm host", wantErr: `built-in "host" network`},
		{line: "docker network inspect bridge", wantErr: `built-in "bridge" network`},
		{line: "docker network connect none api", wantErr: `built-in "none" network`},
		{line: "docker network create default", wantErr: `built-in "default" network`},
		{line: "docker network create App_Net", wantErr: "lowercase logical name"},
		{line: "docker network create " + strings.Repeat("a", 10) + "/x", wantErr: "lowercase logical name"},
		{line: "docker network connect web-net", wantErr: "usage: docker network connect NETWORK ALIAS"},
		{line: "docker network connect web-net api db", wantErr: "usage: docker network connect NETWORK ALIAS"},
		{line: "docker network connect web-net $(whoami)", wantErr: "usage: docker network connect NETWORK ALIAS"},
		{line: "docker network rm web-net db-net", wantErr: "usage: docker network rm NETWORK"},
		{line: "docker network prune", wantErr: "outside this mission's teaching subset"},
		{line: "docker container network ls", wantErr: "outside this mission's teaching subset"},
	}
	for _, test := range rejected {
		if _, err := environment.Execute(context.Background(), test.line); err == nil || !strings.Contains(err.Error(), test.wantErr) {
			t.Errorf("Execute(%q) error = %v, want substring %q", test.line, err, test.wantErr)
		}
	}
	if calls := commandRunner.snapshotCalls(); len(calls) != 0 {
		t.Fatalf("rejected network input reached Docker transport: %q", calls)
	}
}

func TestCloseRemovesNetworksAfterContainersAndVerifiesOwnership(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	environment := createEnvironmentForMission(t, commandRunner, testNetworkDockerMission())
	if _, err := environment.Execute(context.Background(), "docker network create extra-net"); err != nil {
		t.Fatal(err)
	}
	commandRunner.resetCalls()
	if err := environment.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if commandRunner.containerCount() != 0 || commandRunner.networkCount() != 0 {
		t.Fatalf("resources after Close: %d containers, %d networks", commandRunner.containerCount(), commandRunner.networkCount())
	}
	lastContainerRemoval, firstNetworkRemoval := -1, -1
	for index, call := range commandRunner.snapshotCalls() {
		if hasPrefix(call, "container", "rm") {
			lastContainerRemoval = index
		}
		if hasPrefix(call, "network", "rm") && firstNetworkRemoval < 0 {
			firstNetworkRemoval = index
		}
	}
	if firstNetworkRemoval < lastContainerRemoval {
		t.Fatal("a network was removed before every container")
	}

	commandRunner = newFakeDockerRunner()
	environment = createEnvironmentForMission(t, commandRunner, testNetworkDockerMission())
	webNet, _ := environment.network("web-net")
	commandRunner.mutex.Lock()
	commandRunner.networks[webNet.id].labels[sessionLabel] = strings.Repeat("b", 24)
	commandRunner.mutex.Unlock()
	if err := environment.Close(); err == nil || !strings.Contains(err.Error(), "refusing to remove Docker network web-net") {
		t.Fatalf("Close() with foreign network error = %v", err)
	}
	if !commandRunner.hasNetwork(webNet.id) || commandRunner.networkCount() != 1 {
		t.Fatalf("cleanup removed a network with foreign ownership or left others: %d", commandRunner.networkCount())
	}
	commandRunner.mutex.Lock()
	commandRunner.networks[webNet.id].labels[sessionLabel] = environment.sessionID
	commandRunner.mutex.Unlock()
	if err := environment.Close(); err != nil || commandRunner.networkCount() != 0 {
		t.Fatalf("Close() retry = %v, networks left %d", err, commandRunner.networkCount())
	}
}

func TestNetworkSetupFailureCleansUp(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	commandRunner.failNetworkCreate = true
	if _, err := testFactory(commandRunner).Create(context.Background(), testNetworkDockerMission()); err == nil || !strings.Contains(err.Error(), "create Docker network web-net") {
		t.Fatalf("Create() error = %v", err)
	}
	for _, call := range commandRunner.snapshotCalls() {
		if hasPrefix(call, "container", "create") {
			t.Fatal("fixtures were created after network setup failed")
		}
	}
	if commandRunner.containerCount() != 0 || commandRunner.networkCount() != 0 {
		t.Fatalf("resources left after failed setup: %d containers, %d networks", commandRunner.containerCount(), commandRunner.networkCount())
	}
}

func TestJanitorSweepsOrphanedNetworksAfterContainers(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	session := strings.Repeat("1", 24)
	orphanNetwork := "feed" + strings.Repeat("1", 60)
	commandRunner.seedNetwork(orphanNetwork, &fakeNetwork{labels: orphanLabels(session, "999", testOwnerHost), name: "opsquest-" + session + "-n01", created: testNow, internal: true})
	orphanContainer := strings.Repeat("1", 64)
	commandRunner.seedContainer(orphanContainer, &fakeContainer{labels: orphanLabels(session, "999", testOwnerHost), name: "opsquest-" + session + "-c01", created: testNow, running: true, endpoints: map[string]string{"opsquest-" + session + "-n01": orphanNetwork}})
	liveSession := strings.Repeat("2", 24)
	liveNetwork := "feed" + strings.Repeat("2", 60)
	commandRunner.seedNetwork(liveNetwork, &fakeNetwork{labels: orphanLabels(liveSession, "4242", testOwnerHost), name: "opsquest-" + liveSession + "-n01", created: testNow})
	userNetwork := "feed" + strings.Repeat("3", 60)
	commandRunner.seedNetwork(userNetwork, &fakeNetwork{labels: orphanLabels(strings.Repeat("3", 24), "999", testOwnerHost), name: "production-db", created: testNow})

	factory := testFactory(commandRunner)
	if count, err := factory.OrphanedResources(context.Background()); err != nil || count != 2 {
		t.Fatalf("OrphanedResources() = %d, %v; want container plus network", count, err)
	}
	commandRunner.resetCalls()
	removed, err := factory.RemoveOrphanedResources(context.Background())
	if err != nil || removed != 2 {
		t.Fatalf("RemoveOrphanedResources() = %d, %v", removed, err)
	}
	containerRemoval, _ := firstCall(t, commandRunner.snapshotCalls(), "container", "rm", "--force", orphanContainer)
	networkRemoval, _ := firstCall(t, commandRunner.snapshotCalls(), "network", "rm", orphanNetwork)
	if networkRemoval < containerRemoval {
		t.Fatal("orphaned network removed before its container")
	}
	if commandRunner.hasNetwork(orphanNetwork) || !commandRunner.hasNetwork(liveNetwork) || !commandRunner.hasNetwork(userNetwork) {
		t.Fatal("janitor removed the wrong networks")
	}
}

func TestNetworkErrorsRedactExactIdentity(t *testing.T) {
	network := &trackedNetwork{id: "feed" + strings.Repeat("9", 60), logicalID: "net-01", name: "db-net", actualName: "opsquest-" + strings.Repeat("a", 24) + "-n01"}
	message := "network " + network.id + " (" + network.id[:12] + ") named " + network.actualName + " failed"
	got := redactNetworkIdentifiers(message, network)
	if strings.Contains(got, network.id[:12]) || strings.Contains(got, network.actualName) || !strings.Contains(got, "net-01") || !strings.Contains(got, "db-net") {
		t.Fatalf("redactNetworkIdentifiers() = %q", got)
	}
}

func TestMissingNetworkClassificationIsReferenceSpecific(t *testing.T) {
	name := "opsquest-" + strings.Repeat("a", 24) + "-n01"
	id := "feed" + strings.Repeat("0", 59) + "1"
	tests := []struct {
		name      string
		stderr    string
		reference string
		want      bool
	}{
		{name: "daemon missing name", stderr: "Error response from daemon: network " + name + " not found", reference: name, want: true},
		{name: "daemon missing ID", stderr: "Error response from daemon: network " + id + " not found", reference: id, want: true},
		{name: "legacy CLI wording", stderr: "Error: No such network: " + name, reference: name, want: true},
		{name: "context not found", stderr: contextNotFoundMessage, reference: name},
		{name: "different network", stderr: "Error response from daemon: network other-net not found", reference: name},
		{name: "daemon unreachable", stderr: "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?", reference: name},
		{name: "empty reference", stderr: "Error response from daemon: network  not found", reference: ""},
	}
	for _, test := range tests {
		if got := isMissingNetwork(runResult{stderr: test.stderr}, test.reference); got != test.want {
			t.Errorf("%s: isMissingNetwork() = %v, want %v", test.name, got, test.want)
		}
	}
}

func TestContextFailureNeitherPassesValidationNorDropsNetworkCleanup(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	environment := createEnvironmentForMission(t, commandRunner, testNetworkDockerMission())
	networks := commandRunner.networkCount()
	if networks != 3 {
		t.Fatalf("fixture networks = %d, want 3", networks)
	}

	commandRunner.setContextFailure(true)
	absent := mission.Condition{Type: mission.ConditionDockerNetworkAbsent, Network: "db-net"}
	if got, err := environment.Observe(context.Background(), absent); err == nil || got || !strings.Contains(err.Error(), "context not found") {
		t.Fatalf("Observe(network absent) during context failure = %v, %v; want error", got, err)
	}
	shared := networkCondition(mission.ConditionDockerNetworkShared, "web", "api")
	if got, err := environment.Observe(context.Background(), shared); err == nil || got {
		t.Fatalf("Observe(shared) during context failure = %v, %v; want error", got, err)
	}
	if err := environment.Close(); err == nil {
		t.Fatal("Close() during context failure reported success")
	}
	if commandRunner.networkCount() != networks {
		t.Fatalf("networks changed during failed Close: %d", commandRunner.networkCount())
	}

	commandRunner.setContextFailure(false)
	if err := environment.Close(); err != nil {
		t.Fatalf("Close() after context recovery = %v", err)
	}
	if commandRunner.networkCount() != 0 || commandRunner.containerCount() != 0 {
		t.Fatalf("resources leaked after recovered Close: %d networks, %d containers", commandRunner.networkCount(), commandRunner.containerCount())
	}
}

func TestJanitorPropagatesNetworkRemovalFailures(t *testing.T) {
	commandRunner := newFakeDockerRunner()
	session := strings.Repeat("1", 24)
	orphan := "feed" + strings.Repeat("1", 60)
	commandRunner.seedNetwork(orphan, &fakeNetwork{labels: orphanLabels(session, "999", testOwnerHost), name: "opsquest-" + session + "-n01", created: testNow})
	factory := testFactory(commandRunner)

	commandRunner.mutex.Lock()
	commandRunner.contextFailureOnNetworkRemove = true
	commandRunner.mutex.Unlock()
	removed, err := factory.RemoveOrphanedResources(context.Background())
	if err == nil || !strings.Contains(err.Error(), "context not found") || removed != 0 {
		t.Fatalf("RemoveOrphanedResources() during context failure = %d, %v; want error and nothing counted", removed, err)
	}
	if !commandRunner.hasNetwork(orphan) {
		t.Fatal("orphaned network disappeared during a failed removal")
	}

	commandRunner.mutex.Lock()
	commandRunner.contextFailureOnNetworkRemove = false
	commandRunner.mutex.Unlock()
	if removed, err := factory.RemoveOrphanedResources(context.Background()); err != nil || removed != 1 || commandRunner.hasNetwork(orphan) {
		t.Fatalf("RemoveOrphanedResources() after recovery = %d, %v", removed, err)
	}
}
