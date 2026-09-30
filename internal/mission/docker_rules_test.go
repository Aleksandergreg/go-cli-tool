package mission

import (
	"strings"
	"testing"
)

func TestMissionValidationRejectsInvalidDockerDefinitions(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Mission)
		wantErr string
	}{
		{
			name: "missing setup",
			mutate: func(item *Mission) {
				item.Docker = nil
			},
			wantErr: "requires docker setup",
		},
		{
			name: "mixed simulated setup",
			mutate: func(item *Mission) {
				item.Setup.Files = []FileSpec{{Path: "/host-shaped", Content: "no"}}
			},
			wantErr: "cannot define simulated setup",
		},
		{
			name: "duplicate image alias",
			mutate: func(item *Mission) {
				item.Docker.Images = append(item.Docker.Images, item.Docker.Images[0])
			},
			wantErr: "duplicate docker image alias",
		},
		{
			name: "unpinned image",
			mutate: func(item *Mission) {
				item.Docker.Images[0].Reference = "docker.io/library/busybox:latest"
			},
			wantErr: "pinned by sha256 digest",
		},
		{
			name: "duplicate container",
			mutate: func(item *Mission) {
				item.Docker.Containers = append(item.Docker.Containers, item.Docker.Containers[0])
			},
			wantErr: "duplicate docker container name",
		},
		{
			name: "unknown image alias",
			mutate: func(item *Mission) {
				item.Docker.Containers[0].Image = "missing"
			},
			wantErr: "references unknown image alias",
		},
		{
			name: "unknown state",
			mutate: func(item *Mission) {
				item.Docker.Containers[0].State = "paused"
			},
			wantErr: "unknown state",
		},
		{
			name: "diagnostic exit code without log",
			mutate: func(item *Mission) {
				exitCode := 1
				item.Docker.Containers[0].ExitCode = &exitCode
			},
			wantErr: "exit_code requires a log",
		},
		{
			name: "service log too large",
			mutate: func(item *Mission) {
				item.Docker.Containers[1].Log = strings.Repeat("x", maxDockerFixtureLogBytes+1)
			},
			wantErr: "log exceeds",
		},
		{
			name: "unknown health",
			mutate: func(item *Mission) {
				item.Docker.Containers[1].Health = "starting"
			},
			wantErr: "unknown health",
		},
		{
			name: "health on exiting fixture",
			mutate: func(item *Mission) {
				exitCode := 0
				item.Docker.Containers[0].Log = "complete"
				item.Docker.Containers[0].ExitCode = &exitCode
				item.Docker.Containers[0].Health = DockerHealthHealthy
			},
			wantErr: "cannot declare health",
		},
		{
			name: "unknown restart behavior",
			mutate: func(item *Mission) {
				item.Docker.Containers[1].Restart = "always"
			},
			wantErr: "unknown restart behavior",
		},
		{
			name: "restart without crash",
			mutate: func(item *Mission) {
				item.Docker.Containers[1].Restart = DockerRestartOnFailure
			},
			wantErr: "requires log and a non-zero exit_code",
		},
		{
			name: "restart with successful exit",
			mutate: func(item *Mission) {
				exitCode := 0
				item.Docker.Containers[1].Log = "complete"
				item.Docker.Containers[1].ExitCode = &exitCode
				item.Docker.Containers[1].Restart = DockerRestartOnFailure
			},
			wantErr: "requires log and a non-zero exit_code",
		},
		{
			name: "crash loop declared stopped",
			mutate: func(item *Mission) {
				exitCode := 1
				item.Docker.Containers[0].Log = "FATAL"
				item.Docker.Containers[0].ExitCode = &exitCode
				item.Docker.Containers[0].Restart = DockerRestartOnFailure
			},
			wantErr: "crash-loop fixture must use running state",
		},
		{
			name: "reserved network name",
			mutate: func(item *Mission) {
				item.Docker.Networks = []DockerNetworkSpec{{Name: "host"}}
			},
			wantErr: "other than bridge, default, host, or none",
		},
		{
			name: "duplicate network",
			mutate: func(item *Mission) {
				item.Docker.Networks = []DockerNetworkSpec{{Name: "app-net"}, {Name: "app-net"}}
			},
			wantErr: "duplicate docker network name",
		},
		{
			name: "too many networks",
			mutate: func(item *Mission) {
				for index := 0; index <= maxDockerNetworksPerMission; index++ {
					item.Docker.Networks = append(item.Docker.Networks, DockerNetworkSpec{Name: "net-" + strings.Repeat("a", index+1)})
				}
			},
			wantErr: "network limit",
		},
		{
			name: "container joins unknown network",
			mutate: func(item *Mission) {
				item.Docker.Containers[0].Networks = []string{"missing"}
			},
			wantErr: "references unknown network",
		},
		{
			name: "container joins network twice",
			mutate: func(item *Mission) {
				item.Docker.Networks = []DockerNetworkSpec{{Name: "app-net"}}
				item.Docker.Containers[0].Networks = []string{"app-net", "app-net"}
			},
			wantErr: "joins network \"app-net\" twice",
		},
		{
			name: "container joins too many networks",
			mutate: func(item *Mission) {
				for _, name := range []string{"a-net", "b-net", "c-net", "d-net", "e-net"} {
					item.Docker.Networks = append(item.Docker.Networks, DockerNetworkSpec{Name: name})
					item.Docker.Containers[0].Networks = append(item.Docker.Containers[0].Networks, name)
				}
			},
			wantErr: "exceeds the 4-network limit",
		},
		{
			name: "shared network with one container",
			mutate: func(item *Mission) {
				item.Validation.All[0] = Condition{Type: ConditionDockerNetworkShared, Containers: []string{"api"}}
			},
			wantErr: "exactly two different containers",
		},
		{
			name: "isolated container compared with itself",
			mutate: func(item *Mission) {
				item.Validation.All[0] = Condition{Type: ConditionDockerNetworkIsolated, Containers: []string{"api", "api"}}
			},
			wantErr: "exactly two different containers",
		},
		{
			name: "shared network with unknown container",
			mutate: func(item *Mission) {
				item.Validation.All[0] = Condition{Type: ConditionDockerNetworkShared, Containers: []string{"api", "missing"}}
			},
			wantErr: "unknown docker container",
		},
		{
			name: "shared network condition with network field",
			mutate: func(item *Mission) {
				item.Docker.Networks = []DockerNetworkSpec{{Name: "app-net"}}
				item.Validation.All[0] = Condition{Type: ConditionDockerNetworkShared, Containers: []string{"api", "metrics"}, Network: "app-net"}
			},
			wantErr: "does not support network",
		},
		{
			name: "unknown absent network",
			mutate: func(item *Mission) {
				item.Validation.All[0] = Condition{Type: ConditionDockerNetworkAbsent, Network: "missing"}
			},
			wantErr: "unknown docker network",
		},
		{
			name: "reserved absent network",
			mutate: func(item *Mission) {
				item.Validation.All[0] = Condition{Type: ConditionDockerNetworkAbsent, Network: "none"}
			},
			wantErr: "other than bridge, default, host, or none",
		},
		{
			name: "unknown absent validation container",
			mutate: func(item *Mission) {
				item.Validation.All[0] = Condition{Type: ConditionDockerContainerAbsent, Container: "missing"}
			},
			wantErr: "unknown docker container",
		},
		{
			name: "absent condition with count",
			mutate: func(item *Mission) {
				count := 1
				item.Validation.All[0] = Condition{Type: ConditionDockerContainerAbsent, Container: "api", Count: &count}
			},
			wantErr: "does not support count",
		},
		{
			name: "diagnostic exit code outside range",
			mutate: func(item *Mission) {
				exitCode := 256
				item.Docker.Containers[0].Log = "diagnostic"
				item.Docker.Containers[0].ExitCode = &exitCode
			},
			wantErr: "between 0 and 255",
		},
		{
			name: "diagnostic negative exit code",
			mutate: func(item *Mission) {
				exitCode := -1
				item.Docker.Containers[0].Log = "diagnostic"
				item.Docker.Containers[0].ExitCode = &exitCode
			},
			wantErr: "between 0 and 255",
		},
		{
			name: "diagnostic log too large",
			mutate: func(item *Mission) {
				exitCode := 1
				item.Docker.Containers[0].Log = strings.Repeat("x", maxDockerFixtureLogBytes+1)
				item.Docker.Containers[0].ExitCode = &exitCode
			},
			wantErr: "log exceeds",
		},
		{
			name: "diagnostic log contains NUL",
			mutate: func(item *Mission) {
				exitCode := 1
				item.Docker.Containers[0].Log = "bad\x00log"
				item.Docker.Containers[0].ExitCode = &exitCode
			},
			wantErr: "cannot contain NUL",
		},
		{
			name: "diagnostic fixture declared running",
			mutate: func(item *Mission) {
				exitCode := 0
				item.Docker.Containers[1].Log = "complete"
				item.Docker.Containers[1].ExitCode = &exitCode
			},
			wantErr: "must use stopped state",
		},
		{
			name: "unknown validation container",
			mutate: func(item *Mission) {
				item.Validation.All[0].Container = "missing"
			},
			wantErr: "unknown docker container",
		},
		{
			name: "unknown stopped validation container",
			mutate: func(item *Mission) {
				item.Validation.All[0] = Condition{Type: ConditionDockerContainerStopped, Container: "missing"}
			},
			wantErr: "unknown docker container",
		},
		{
			name: "missing count",
			mutate: func(item *Mission) {
				item.Validation.All[2].Count = nil
			},
			wantErr: "count must be a non-negative integer",
		},
		{
			name: "negative count",
			mutate: func(item *Mission) {
				count := -1
				item.Validation.All[2].Count = &count
			},
			wantErr: "count must be a non-negative integer",
		},
		{
			name: "running condition with unrelated value",
			mutate: func(item *Mission) {
				item.Validation.All[0].Value = "api"
			},
			wantErr: "does not support value",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			catalog, err := LoadCatalog()
			if err != nil {
				t.Fatal(err)
			}
			item, _ := catalog.Find("docker-container-census")
			test.mutate(&item)
			err = validateMission(item)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validateMission() error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestMissionValidationAcceptsDockerFixtureBehaviors(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, _ := catalog.Find("docker-container-census")
	failure := 3
	item.Docker.Containers = []DockerContainerSpec{
		{Name: "api", Image: "fixture", State: DockerStateStopped, Log: "INFO ready", Health: DockerHealthHealthy},
		{Name: "metrics", Image: "fixture", State: DockerStateRunning, Health: DockerHealthUnhealthy},
		{Name: "payments", Image: "fixture", State: DockerStateRunning, Log: "FATAL key missing", ExitCode: &failure, Restart: DockerRestartOnFailure},
	}
	item.Validation.All = append(item.Validation.All, Condition{Type: ConditionDockerContainerAbsent, Container: "payments"})
	if err := validateMission(item); err != nil {
		t.Fatalf("validateMission() error = %v", err)
	}
}

func TestMissionValidationAcceptsDockerNetworks(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, _ := catalog.Find("docker-container-census")
	item.Docker.Networks = []DockerNetworkSpec{{Name: "app-net"}, {Name: "db-net"}}
	item.Docker.Containers[0].Networks = []string{"app-net", "db-net"}
	item.Docker.Containers[1].Networks = []string{"app-net"}
	item.Validation.All = append(item.Validation.All,
		Condition{Type: ConditionDockerNetworkShared, Containers: []string{"api", "metrics"}},
		Condition{Type: ConditionDockerNetworkIsolated, Containers: []string{"metrics", "api"}},
		Condition{Type: ConditionDockerNetworkAbsent, Network: "db-net"},
	)
	if err := validateMission(item); err != nil {
		t.Fatalf("validateMission() error = %v", err)
	}
	for _, name := range []string{"bridge", "default", "host", "none"} {
		if ValidDockerNetworkName(name) || !ReservedDockerNetworkName(name) {
			t.Errorf("network name %q was not treated as reserved", name)
		}
	}
}

func TestDockerNetworkFieldsAreDeepCopied(t *testing.T) {
	item := Mission{
		Docker: &DockerSetup{
			Networks:   []DockerNetworkSpec{{Name: "app-net"}},
			Containers: []DockerContainerSpec{{Name: "api", Networks: []string{"app-net"}}},
		},
		Validation: Validation{All: []Condition{{Type: ConditionDockerNetworkShared, Containers: []string{"api", "db"}}}},
	}
	cloned := cloneMission(item)
	cloned.Docker.Networks[0].Name = "changed"
	cloned.Docker.Containers[0].Networks[0] = "changed"
	cloned.Validation.All[0].Containers[0] = "changed"
	if item.Docker.Networks[0].Name != "app-net" || item.Docker.Containers[0].Networks[0] != "app-net" || item.Validation.All[0].Containers[0] != "api" {
		t.Fatalf("cloneMission shared network data: %#v", item)
	}
}

func TestMissionValidationRejectsDockerConditionOnSimulatedMission(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, _ := catalog.Find("linux-orientation")
	for _, condition := range []Condition{
		{Type: ConditionDockerContainerRunning, Container: "api"},
		{Type: ConditionDockerContainerAbsent, Container: "api"},
		{Type: ConditionDockerNetworkShared, Containers: []string{"api", "db"}},
		{Type: ConditionDockerNetworkIsolated, Containers: []string{"api", "db"}},
		{Type: ConditionDockerNetworkAbsent, Network: "app-net"},
	} {
		item.Validation.All = []Condition{condition}
		if err := validateMission(item); err == nil || !strings.Contains(err.Error(), "requires a docker environment") {
			t.Fatalf("validateMission(%s) error = %v", condition.Type, err)
		}
	}
}

func TestDockerSetupResourceLimits(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, _ := catalog.Find("docker-container-census")

	tooManyImages := *item.Docker
	tooManyImages.Images = make([]DockerImageSpec, maxDockerImagesPerMission+1)
	if err := ValidateDockerSetup(tooManyImages); err == nil || !strings.Contains(err.Error(), "image limit") {
		t.Fatalf("ValidateDockerSetup(images) error = %v", err)
	}

	tooManyContainers := *item.Docker
	tooManyContainers.Containers = make([]DockerContainerSpec, maxDockerContainersPerMission+1)
	if err := ValidateDockerSetup(tooManyContainers); err == nil || !strings.Contains(err.Error(), "container limit") {
		t.Fatalf("ValidateDockerSetup(containers) error = %v", err)
	}
}
