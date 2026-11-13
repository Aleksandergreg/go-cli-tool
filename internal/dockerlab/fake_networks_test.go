package dockerlab

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type fakeNetwork struct {
	labels   map[string]string
	name     string
	created  time.Time
	internal bool
}

// runNetwork fakes the network subcommands the adapter issues.
func (r *fakeDockerRunner) runNetwork(args []string) (runResult, error) {
	switch {
	case hasPrefix(args, "network", "create"):
		r.networkCreates++
		if r.failNetworkCreate {
			return runResult{stderr: "network creation failed"}, errors.New("exit status 1")
		}
		network := &fakeNetwork{labels: make(map[string]string), name: args[len(args)-1], created: testNow}
		for index := 2; index < len(args)-1; index++ {
			switch args[index] {
			case "--internal":
				network.internal = true
			case "--label":
				labelName, value, _ := strings.Cut(args[index+1], "=")
				network.labels[labelName] = value
				index++
			case "--driver":
				index++
			}
		}
		id := fmt.Sprintf("feed%060x", r.networkCreates)
		r.networks[id] = network
		r.networkOrder = append(r.networkOrder, id)
		return runResult{stdout: id + "\n"}, nil
	case hasPrefix(args, "network", "inspect", "--format"):
		reference := args[len(args)-1]
		id := r.resolveNetwork(reference)
		network, exists := r.networks[id]
		if !exists {
			return runResult{stderr: "Error response from daemon: network " + reference + " not found"}, errors.New("exit status 1")
		}
		encoded, err := json.Marshal(networkInspection{ID: id, Name: network.name, Created: network.created.Format(time.RFC3339Nano), Driver: "bridge", Internal: network.internal, Labels: cloneStrings(network.labels)})
		if err != nil {
			return runResult{}, err
		}
		return runResult{stdout: string(encoded) + "\n"}, nil
	case hasPrefix(args, "network", "ls", "--quiet", "--no-trunc"):
		filter := strings.TrimPrefix(args[len(args)-1], "label=")
		name, value, _ := strings.Cut(filter, "=")
		var output strings.Builder
		for _, id := range r.networkOrder {
			if network, exists := r.networks[id]; exists && network.labels[name] == value {
				output.WriteString(id + "\n")
			}
		}
		return runResult{stdout: output.String()}, nil
	case hasPrefix(args, "network", "rm") && r.contextFailureOnNetworkRemove:
		return runResult{stderr: contextNotFoundMessage}, errors.New("exit status 1")
	case hasPrefix(args, "network", "rm"):
		id := args[len(args)-1]
		network, exists := r.networks[id]
		if !exists {
			return runResult{stderr: "Error response from daemon: network " + id + " not found"}, errors.New("exit status 1")
		}
		for _, container := range r.containers {
			if _, attached := container.endpoints[network.name]; attached && container.running {
				return runResult{stderr: "Error response from daemon: error while removing network: network " + network.name + " id " + id + " has active endpoints"}, errors.New("exit status 1")
			}
		}
		delete(r.networks, id)
		return runResult{stdout: id + "\n"}, nil
	case hasPrefix(args, "network", "connect") || hasPrefix(args, "network", "disconnect"):
		networkID, containerID := args[len(args)-2], args[len(args)-1]
		network, exists := r.networks[networkID]
		if !exists {
			return runResult{stderr: "Error response from daemon: network " + networkID + " not found"}, errors.New("exit status 1")
		}
		container, exists := r.containers[containerID]
		if !exists {
			return missingContainerResult(containerID)
		}
		_, attached := container.endpoints[network.name]
		if args[1] == "disconnect" {
			if !attached {
				return runResult{stderr: "Error response from daemon: container " + containerID + " is not connected to network " + network.name}, errors.New("exit status 1")
			}
			delete(container.endpoints, network.name)
			return runResult{}, nil
		}
		if _, disabled := container.endpoints["none"]; disabled {
			return runResult{stderr: "Error response from daemon: container cannot be connected to multiple networks with one of the networks in private (none) mode"}, errors.New("exit status 1")
		}
		if attached {
			return runResult{stderr: "Error response from daemon: endpoint with name " + container.name + " already exists in network " + network.name}, errors.New("exit status 1")
		}
		if container.endpoints == nil {
			container.endpoints = make(map[string]string)
		}
		container.endpoints[network.name] = networkID
		return runResult{}, nil
	default:
		return runResult{stderr: "unexpected fake Docker invocation"}, fmt.Errorf("unexpected Docker args: %q", args)

	}
}

func (r *fakeDockerRunner) resolveNetwork(reference string) string {
	if _, exists := r.networks[reference]; exists {
		return reference
	}
	for id, network := range r.networks {
		if network.name == reference {
			return id
		}
	}
	return reference
}

func (r *fakeDockerRunner) seedNetwork(id string, network *fakeNetwork) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.networks[id] = network
	r.networkOrder = append(r.networkOrder, id)
}

func (r *fakeDockerRunner) hasNetwork(id string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	_, exists := r.networks[id]
	return exists
}

func (r *fakeDockerRunner) networkCount() int {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return len(r.networks)
}
