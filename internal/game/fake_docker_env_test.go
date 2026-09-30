package game

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/aleksandergregersen/opsquest/internal/mission"
)

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

func (e *missionDockerEnvironment) Close() error { return nil }
