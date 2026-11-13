package dockerlab

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// existingNetwork resolves a logical name to a network that still exists.
func (e *environment) existingNetwork(ctx context.Context, name string) (*trackedNetwork, error) {
	network, exists := e.network(name)
	if !exists {
		return nil, fmt.Errorf("docker: network %q is not part of this mission", name)
	}
	_, exists, err := e.inspectNetwork(ctx, network)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("docker: network %q has been removed", name)
	}
	return network, nil
}

// existingContainer resolves an alias to a container that still exists and
// returns its current inspection.
func (e *environment) existingContainer(ctx context.Context, alias string) (*trackedContainer, containerInspection, error) {
	tracked, exists := e.container(alias)
	if !exists {
		return nil, containerInspection{}, fmt.Errorf("docker: container %q is not part of this mission", alias)
	}
	inspection, exists, err := e.inspect(ctx, tracked.id)
	if err != nil {
		return nil, containerInspection{}, err
	}
	if !exists {
		return nil, containerInspection{}, fmt.Errorf("docker: container %q has been removed", alias)
	}
	return tracked, inspection, nil
}

func (e *environment) listNetworks(ctx context.Context) (string, error) {
	type row struct {
		logicalID string
		name      string
		driver    string
		internal  bool
	}
	var rows []row
	nameWidth := len("NAME")
	for _, network := range e.snapshotNetworks() {
		if network.id == "" {
			continue
		}
		if current, bound := e.network(network.name); !bound || current != network {
			continue
		}
		inspection, exists, err := e.inspectNetwork(ctx, network)
		if err != nil {
			return "", err
		}
		if !exists {
			continue
		}
		nameWidth = max(nameWidth, len(network.name))
		rows = append(rows, row{logicalID: network.logicalID, name: network.name, driver: inspection.Driver, internal: inspection.Internal})
	}
	var output strings.Builder
	fmt.Fprintf(&output, "%-11s %-*s %-7s %s\n", "NETWORK ID", nameWidth, "NAME", "DRIVER", "INTERNAL")
	for _, item := range rows {
		fmt.Fprintf(&output, "%-11s %-*s %-7s %t\n", item.logicalID, nameWidth, item.name, item.driver, item.internal)
	}
	return output.String(), nil
}

func (e *environment) createPlayerNetwork(ctx context.Context, name string) (string, error) {
	if existing, bound := e.network(name); bound {
		_, exists, err := e.inspectNetwork(ctx, existing)
		if err != nil {
			return "", err
		}
		if exists {
			return "", fmt.Errorf("docker: network %q already exists", name)
		}
	}
	created := 0
	for _, network := range e.snapshotNetworks() {
		if network.playerCreated {
			created++
		}
	}
	if created >= maxPlayerNetworks {
		return "", fmt.Errorf("docker: this lab lets you create at most %d networks per attempt", maxPlayerNetworks)
	}
	network, err := e.createNetwork(ctx, name, true)
	if err != nil {
		return "", err
	}
	return network.logicalID + "\n", nil
}

func (e *environment) removeNetwork(ctx context.Context, name string) error {
	network, exists := e.network(name)
	if !exists {
		return fmt.Errorf("docker: network %q is not part of this mission", name)
	}
	_, exists, err := e.inspectNetwork(ctx, network)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("docker: network %q has already been removed", name)
	}
	// Docker would remove a network that only stopped containers use and
	// leave them unable to start. The lab refuses instead, so every
	// container stays startable.
	attached, err := e.attachedContainers(ctx, network)
	if err != nil {
		return err
	}
	if len(attached) > 0 {
		return fmt.Errorf("docker: network %q still has containers attached (%s); disconnect or remove them first", name, strings.Join(attached, ", "))
	}
	return e.runNetworkAction(ctx, network, nil, "network", "rm", network.id)
}

func (e *environment) logicalNetworkInspect(ctx context.Context, name string) (string, error) {
	network, exists := e.network(name)
	if !exists {
		return "", fmt.Errorf("docker: network %q is not part of this mission", name)
	}
	inspection, exists, err := e.inspectNetwork(ctx, network)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("docker: network %q no longer exists", name)
	}
	attached, err := e.attachedContainers(ctx, network)
	if err != nil {
		return "", err
	}
	sort.Strings(attached)
	logical := struct {
		ID         string   `json:"Id"`
		Name       string   `json:"Name"`
		Driver     string   `json:"Driver"`
		Internal   bool     `json:"Internal"`
		Containers []string `json:"Containers"`
	}{ID: network.logicalID, Name: network.name, Driver: inspection.Driver, Internal: inspection.Internal, Containers: append([]string{}, attached...)}
	encoded, err := json.MarshalIndent(logical, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded) + "\n", nil
}

func (e *environment) connectNetwork(ctx context.Context, name, alias string) error {
	network, err := e.existingNetwork(ctx, name)
	if err != nil {
		return err
	}
	tracked, inspection, err := e.existingContainer(ctx, alias)
	if err != nil {
		return err
	}
	if networkingDisabled(inspection) {
		return fmt.Errorf("docker: container %q runs with networking disabled in this lab and cannot join a network", alias)
	}
	if attachedTo(inspection, network) {
		return fmt.Errorf("docker: container %q is already connected to network %q", alias, name)
	}
	return e.runNetworkAction(ctx, network, tracked, "network", "connect", "--alias", alias, network.id, tracked.id)
}

func (e *environment) disconnectNetwork(ctx context.Context, name, alias string) error {
	network, err := e.existingNetwork(ctx, name)
	if err != nil {
		return err
	}
	tracked, inspection, err := e.existingContainer(ctx, alias)
	if err != nil {
		return err
	}
	if !attachedTo(inspection, network) {
		return fmt.Errorf("docker: container %q is not connected to network %q", alias, name)
	}
	return e.runNetworkAction(ctx, network, tracked, "network", "disconnect", network.id, tracked.id)
}

// runNetworkAction runs one fixed network invocation and rewrites engine
// errors so exact network and container identities never reach the player.
func (e *environment) runNetworkAction(ctx context.Context, network *trackedNetwork, container *trackedContainer, args ...string) error {
	identities := []identity{network.identity()}
	if container != nil {
		identities = append(identities, container.identity())
	}
	_, err := e.runRedacted(ctx, identities, args...)
	return err
}
