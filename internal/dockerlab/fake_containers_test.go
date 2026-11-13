package dockerlab

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type fakeContainer struct {
	labels        map[string]string
	name          string
	created       time.Time
	running       bool
	restarting    bool
	status        string
	log           string
	exitCode      int
	oneShot       bool
	restartPolicy string
	restartCount  int
	healthCmd     string
	// endpoints maps an attached network's engine name to its ID.
	endpoints map[string]string
}

// runContainer fakes the container subcommands the adapter issues.
func (r *fakeDockerRunner) runContainer(args []string) (runResult, error) {
	switch {
	case hasPrefix(args, "container", "create"):
		r.createAttempts++
		if r.failCreateAt == r.createAttempts {
			return runResult{stderr: "fixture creation failed"}, errors.New("exit status 1")
		}
		// Created IDs use a prefix no seeded test container shares.
		id := fmt.Sprintf("c0ffee%058x", r.createAttempts)
		labels := make(map[string]string)
		name := ""
		restartPolicy := ""
		healthCmd := ""
		endpoints := make(map[string]string)
		for index := 0; index+1 < len(args); index++ {
			switch args[index] {
			case "--network":
				if args[index+1] == "none" {
					endpoints["none"] = "none-network-id"
				} else if network, exists := r.networks[args[index+1]]; exists {
					endpoints[network.name] = args[index+1]
				} else {
					return runResult{stderr: "Error response from daemon: network " + args[index+1] + " not found"}, errors.New("exit status 1")
				}
			case "--name":
				name = args[index+1]
			case "--label":
				labelName, value, found := strings.Cut(args[index+1], "=")
				if found {
					labels[labelName] = value
				}
			case "--restart":
				restartPolicy = args[index+1]
			case "--health-cmd":
				healthCmd = args[index+1]
			}
		}
		container := &fakeContainer{labels: labels, name: name, created: testNow, status: "created", restartPolicy: restartPolicy, healthCmd: healthCmd, endpoints: endpoints}
		if len(args) >= 3 && args[len(args)-3] == "opsquest-service" {
			container.log = args[len(args)-2] + "\n"
		}
		if len(args) >= 3 && args[len(args)-3] == "opsquest-diagnostic" {
			container.oneShot = true
			container.log = args[len(args)-2] + "\n"
			exitCode, err := strconv.Atoi(args[len(args)-1])
			if err != nil {
				return runResult{}, err
			}
			container.exitCode = exitCode
		}
		r.containers[id] = container
		r.containerOrder = append(r.containerOrder, id)
		if r.failAfterCreateAt == r.createAttempts {
			return runResult{stderr: "connection lost after create"}, errors.New("exit status 1")
		}
		return runResult{stdout: id + "\n"}, nil
	case hasPrefix(args, "container", "start") || hasPrefix(args, "container", "restart"):
		if r.failNextStart {
			r.failNextStart = false
			return runResult{stderr: "start failed"}, errors.New("exit status 1")
		}
		id := args[len(args)-1]
		container, exists := r.containers[id]
		if !exists {
			return missingContainerResult(id)
		}
		if container.oneShot && strings.HasPrefix(container.restartPolicy, "on-failure") {
			// Model a crash loop observed between restarts: Docker reports a
			// restarting container as running and appends each attempt's log.
			container.running = true
			container.restarting = true
			container.status = "restarting"
			container.restartCount += crashLoopReadyRestarts
			container.log = strings.Repeat(strings.SplitAfter(container.log, "\n")[0], container.restartCount+1)
		} else if container.oneShot {
			container.running = false
			container.status = "exited"
		} else {
			container.running = true
			container.status = "running"
			container.exitCode = 0
		}
		return runResult{stdout: id + "\n"}, nil
	case hasPrefix(args, "container", "stop"):
		id := args[len(args)-1]
		container, exists := r.containers[id]
		if !exists {
			return missingContainerResult(id)
		}
		container.running = false
		container.restarting = false
		container.status = "exited"
		return runResult{stdout: id + "\n"}, nil
	case hasPrefix(args, "container", "wait"):
		id := args[len(args)-1]
		container, exists := r.containers[id]
		if !exists {
			return missingContainerResult(id)
		}
		return runResult{stdout: strconv.Itoa(container.exitCode) + "\n"}, nil
	case hasPrefix(args, "container", "logs"):
		id := args[len(args)-1]
		container, exists := r.containers[id]
		if !exists {
			return missingContainerResult(id)
		}
		if len(args) == 5 && args[2] == "--tail" {
			tail, err := strconv.Atoi(args[3])
			if err != nil {
				return runResult{}, err
			}
			lines := strings.SplitAfter(container.log, "\n")
			if lines[len(lines)-1] == "" {
				lines = lines[:len(lines)-1]
			}
			if tail < len(lines) {
				lines = lines[len(lines)-tail:]
			}
			return runResult{stdout: strings.Join(lines, "")}, nil
		}
		return runResult{stdout: container.log}, nil
	case hasPrefix(args, "container", "inspect", "--format"):
		reference := args[len(args)-1]
		if err := r.failNextInspect[reference]; err != nil {
			delete(r.failNextInspect, reference)
			return runResult{stderr: err.Error()}, err
		}
		if r.hideNextInspect[reference] {
			delete(r.hideNextInspect, reference)
			return missingContainerResult(reference)
		}
		id := r.resolveContainer(reference)
		container, exists := r.containers[id]
		if !exists {
			return missingContainerResult(id)
		}
		inspection := containerInspection{ID: id, Name: "/" + container.name, Created: container.created.Format(time.RFC3339Nano), RestartCount: container.restartCount}
		inspection.Config.Labels = cloneStrings(container.labels)
		inspection.State.Running = container.running
		inspection.State.Restarting = container.restarting
		inspection.State.Status = container.status
		inspection.State.ExitCode = container.exitCode
		if policy, retries, found := strings.Cut(container.restartPolicy, ":"); found {
			inspection.HostConfig.RestartPolicy.Name = policy
			inspection.HostConfig.RestartPolicy.MaximumRetryCount, _ = strconv.Atoi(retries)
		} else {
			inspection.HostConfig.RestartPolicy.Name = container.restartPolicy
		}
		if len(container.endpoints) > 0 {
			inspection.NetworkSettings.Networks = make(map[string]endpointState)
			for name, networkID := range container.endpoints {
				// Docker can leave NetworkID empty before a first start.
				if container.status == "created" {
					networkID = ""
				}
				inspection.NetworkSettings.Networks[name] = endpointState{NetworkID: networkID}
			}
		}
		if container.healthCmd != "" && container.running && !r.stripHealth {
			health := "unhealthy"
			if container.healthCmd == "true" {
				health = "healthy"
			}
			inspection.State.Health = &healthState{Status: health}
		}
		encoded, err := json.Marshal(inspection)
		if err != nil {
			return runResult{}, err
		}
		return runResult{stdout: string(encoded) + "\n"}, nil
	case hasPrefix(args, "container", "ls", "--all", "--quiet", "--no-trunc") && r.failManagedList && args[len(args)-1] == "label="+managedLabel+"=true":
		return runResult{stderr: "list failed"}, errors.New("exit status 1")
	case hasPrefix(args, "container", "ls", "--all", "--quiet", "--no-trunc"):
		filters := make(map[string]string)
		for index := 0; index+1 < len(args); index++ {
			if args[index] != "--filter" {
				continue
			}
			filter := strings.TrimPrefix(args[index+1], "label=")
			name, value, found := strings.Cut(filter, "=")
			if found {
				filters[name] = value
			}
		}
		var output strings.Builder
		for _, id := range r.containerOrder {
			container, exists := r.containers[id]
			if !exists || !labelsMatch(container.labels, filters) {
				continue
			}
			output.WriteString(id)
			output.WriteByte('\n')
		}
		return runResult{stdout: output.String()}, nil
	case len(args) == 3 && hasPrefix(args, "container", "rm"):
		id := args[len(args)-1]
		container, exists := r.containers[id]
		if !exists {
			return missingContainerResult(id)
		}
		if container.running {
			return runResult{stderr: "Error response from daemon: cannot remove container: container is running: stop the container before removing or force remove"}, errors.New("exit status 1")
		}
		delete(r.containers, id)
		return runResult{stdout: id + "\n"}, nil
	case hasPrefix(args, "container", "rm", "--force"):
		id := args[len(args)-1]
		if err := r.failNextRemove[id]; err != nil {
			delete(r.failNextRemove, id)
			return runResult{stderr: err.Error()}, err
		}
		if _, exists := r.containers[id]; !exists {
			return missingContainerResult(id)
		}
		delete(r.containers, id)
		return runResult{stdout: id + "\n"}, nil
	default:
		return runResult{stderr: "unexpected fake Docker invocation"}, fmt.Errorf("unexpected Docker args: %q", args)

	}
}

func (r *fakeDockerRunner) resolveContainer(reference string) string {
	if _, exists := r.containers[reference]; exists {
		return reference
	}
	for id, container := range r.containers {
		if container.name == reference {
			return id
		}
	}
	return reference
}

func (r *fakeDockerRunner) seedContainer(id string, container *fakeContainer) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.containers[id] = container
	r.containerOrder = append(r.containerOrder, id)
}

func (r *fakeDockerRunner) hasContainer(id string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	_, exists := r.containers[id]
	return exists
}

func (r *fakeDockerRunner) containerCount() int {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return len(r.containers)
}

func (r *fakeDockerRunner) corruptLabel(id, name, value string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.containers[id].labels[name] = value
}

func (r *fakeDockerRunner) failRemoveOnce(id string, err error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.failNextRemove[id] = err
}

func (r *fakeDockerRunner) removeContainer(id string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	delete(r.containers, id)
}

func missingContainerResult(id string) (runResult, error) {
	return runResult{stderr: "Error: No such container: " + id}, errors.New("exit status 1")
}
