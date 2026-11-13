package dockerlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// maxPlayerNetworks bounds how many networks one attempt lets the player
// create, including networks later removed, so churn cannot grow without
// limit.
const maxPlayerNetworks = 4

// trackedNetwork maps one logical network name to the exact attempt-owned
// engine network behind it.
type trackedNetwork struct {
	id            string
	logicalID     string
	name          string
	actualName    string
	playerCreated bool
}

type networkInspection struct {
	ID       string            `json:"Id"`
	Name     string            `json:"Name"`
	Created  string            `json:"Created"`
	Driver   string            `json:"Driver"`
	Internal bool              `json:"Internal"`
	Labels   map[string]string `json:"Labels"`
}

func (e *environment) network(name string) (*trackedNetwork, bool) {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	network, exists := e.networkByName[name]
	return network, exists
}

func (e *environment) snapshotNetworks() []*trackedNetwork {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	return append([]*trackedNetwork(nil), e.networks...)
}

// createNetwork creates one internal, labeled network with a generated name.
// The network is tracked before creation so cleanup can find it by name if
// the engine's response is ambiguous; the logical name is bound only once
// the exact ID is known.
func (e *environment) createNetwork(ctx context.Context, name string, playerCreated bool) (*trackedNetwork, error) {
	e.mutex.Lock()
	index := len(e.networks) + 1
	tracked := &trackedNetwork{
		logicalID:     fmt.Sprintf("net-%02d", index),
		name:          name,
		actualName:    fmt.Sprintf("opsquest-%s-n%02d", e.sessionID, index),
		playerCreated: playerCreated,
	}
	e.networks = append(e.networks, tracked)
	e.mutex.Unlock()

	args := []string{
		"network", "create",
		"--driver", "bridge",
		// Internal networks have no route to the host network or the
		// internet; only containers of this attempt can join them.
		"--internal",
	}
	args = append(args, e.ownershipLabels(name)...)
	args = append(args, tracked.actualName)
	result, err := e.run(ctx, args...)
	id := strings.TrimSpace(result.stdout)
	if err != nil || !containerIDPattern.MatchString(id) {
		if err == nil {
			err = fmt.Errorf("Docker returned an invalid network ID")
		}
		createErr := fmt.Errorf("create Docker network %s: %w", name, err)
		return nil, e.reconcileAmbiguousNetwork(tracked, createErr)
	}
	tracked.id = id
	e.mutex.Lock()
	e.networkByName[name] = tracked
	e.mutex.Unlock()
	return tracked, nil
}

// reconcileAmbiguousNetwork recovers the exact ID of a network the engine
// may have created despite reporting an error, so cleanup can remove it
// after verifying ownership. The logical name stays unbound either way.
func (e *environment) reconcileAmbiguousNetwork(tracked *trackedNetwork, createErr error) error {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	inspection, exists, err := inspectNetworkReference(ctx, e.runner, tracked.actualName)
	if err != nil {
		return errors.Join(createErr, fmt.Errorf("reconcile Docker network %s: %w", tracked.name, err))
	}
	if !exists {
		return createErr
	}
	labels := inspection.Labels
	if !e.owns(labels, tracked.name) || !containerIDPattern.MatchString(inspection.ID) {
		return errors.Join(createErr, fmt.Errorf("refusing to recover Docker network %s because its ownership does not match", tracked.name))
	}
	tracked.id = inspection.ID
	return createErr
}

func (e *environment) inspectNetwork(ctx context.Context, network *trackedNetwork) (networkInspection, bool, error) {
	if err := e.ready(ctx); err != nil {
		return networkInspection{}, false, err
	}
	return e.inspectNetworkUnchecked(ctx, network)
}

func (e *environment) inspectNetworkUnchecked(ctx context.Context, network *trackedNetwork) (networkInspection, bool, error) {
	reference := network.id
	if reference == "" {
		reference = network.actualName
	}
	inspection, exists, err := inspectNetworkReference(ctx, e.runner, reference)
	if err != nil || !exists {
		return inspection, exists, err
	}
	if network.id != "" && inspection.ID != network.id {
		return networkInspection{}, false, fmt.Errorf("Docker inspection returned an unexpected network ID")
	}
	return inspection, true, nil
}

// inspectNetworkReference inspects one exact network ID or generated name.
func inspectNetworkReference(ctx context.Context, commandRunner runner, reference string) (networkInspection, bool, error) {
	result, err := runDocker(ctx, commandRunner, "network", "inspect", "--format", "{{json .}}", reference)
	if err != nil {
		if isMissingNetwork(result, reference) {
			return networkInspection{}, false, nil
		}
		return networkInspection{}, false, err
	}
	var inspection networkInspection
	if err := json.Unmarshal([]byte(result.stdout), &inspection); err != nil {
		return networkInspection{}, false, fmt.Errorf("decode Docker network inspection: %w", err)
	}
	return inspection, true, nil
}

// attachedTo reports whether a container inspection shows an endpoint on
// network.
func attachedTo(inspection containerInspection, network *trackedNetwork) bool {
	for name, endpoint := range inspection.NetworkSettings.Networks {
		if name == network.actualName || network.id != "" && endpoint.NetworkID == network.id {
			return true
		}
	}
	return false
}

// networkingDisabled reports whether a fixture was created without
// networking. Docker cannot attach such a container to another network.
func networkingDisabled(inspection containerInspection) bool {
	_, disabled := inspection.NetworkSettings.Networks["none"]
	return disabled
}

// logicalNetworks returns the sorted logical names of every attempt network
// a container is attached to, or none for a fixture without networking.
func (e *environment) logicalNetworks(inspection containerInspection) []string {
	names := make([]string, 0)
	if networkingDisabled(inspection) {
		names = append(names, "none")
	}
	for _, network := range e.snapshotNetworks() {
		if network.id != "" && attachedTo(inspection, network) {
			names = append(names, network.name)
		}
	}
	sort.Strings(names)
	return names
}

// attachedContainers lists the existing mission containers attached to
// network, in fixture order.
func (e *environment) attachedContainers(ctx context.Context, network *trackedNetwork) ([]string, error) {
	var attached []string
	for _, tracked := range e.snapshotContainers() {
		inspection, exists, err := e.inspect(ctx, tracked.id)
		if err != nil {
			return nil, err
		}
		if exists && attachedTo(inspection, network) {
			attached = append(attached, tracked.alias)
		}
	}
	return attached, nil
}
