package dockerlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
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
		"--label", managedLabel + "=true",
		"--label", schemaLabel + "=1",
		"--label", sessionLabel + "=" + e.sessionID,
		"--label", missionLabel + "=" + e.missionID,
		"--label", aliasLabel + "=" + name,
	}
	if e.ownerHost != "" && e.ownerPID > 0 {
		args = append(args,
			"--label", ownerPIDLabel+"="+strconv.Itoa(e.ownerPID),
			"--label", ownerHostLabel+"="+e.ownerHost,
		)
	}
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
	if labels[managedLabel] != "true" || labels[sessionLabel] != e.sessionID || labels[missionLabel] != e.missionID || labels[aliasLabel] != tracked.name || !containerIDPattern.MatchString(inspection.ID) {
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

// isMissingNetwork reports whether Docker said that exactly reference does
// not exist. It accepts only the engine's network-specific wording that names
// the reference, so CLI, context, and transport failures such as
// `context "x": context not found` propagate as errors instead of looking
// like a removed network.
func isMissingNetwork(result runResult, reference string) bool {
	if reference == "" {
		return false
	}
	message := strings.ToLower(result.stderr + "\n" + result.stdout)
	reference = strings.ToLower(reference)
	return strings.Contains(message, "network "+reference+" not found") ||
		strings.Contains(message, "no such network: "+reference)
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
	operationCtx, cancel := context.WithTimeout(ctx, dockerOperationTimeout)
	defer cancel()
	result, err := e.runner.run(operationCtx, args...)
	if err == nil {
		return nil
	}
	message := dockerFailureDetail(result, err)
	message = redactNetworkIdentifiers(message, network)
	if container != nil {
		message = redactIdentifiers(message, container)
	}
	return fmt.Errorf("Docker command failed%s", message)
}

func redactNetworkIdentifiers(message string, network *trackedNetwork) string {
	if network.id != "" {
		message = strings.ReplaceAll(message, network.id, network.logicalID)
		if len(network.id) >= 12 {
			message = strings.ReplaceAll(message, network.id[:12], network.logicalID)
		}
	}
	return strings.ReplaceAll(message, network.actualName, network.name)
}

// shareNetwork reports whether both containers exist and have at least one
// attempt network in common. Both results are false when either is gone.
func (e *environment) shareNetwork(ctx context.Context, first, second string) (shared, bothExist bool, err error) {
	var inspections [2]containerInspection
	for index, alias := range []string{first, second} {
		tracked, exists := e.container(alias)
		if !exists {
			return false, false, nil
		}
		inspection, exists, err := e.inspect(ctx, tracked.id)
		if err != nil || !exists {
			return false, false, err
		}
		inspections[index] = inspection
	}
	for _, network := range e.snapshotNetworks() {
		if network.id != "" && attachedTo(inspections[0], network) && attachedTo(inspections[1], network) {
			return true, true, nil
		}
	}
	return false, true, nil
}

func (e *environment) networkAbsent(ctx context.Context, name string) (bool, error) {
	network, exists := e.network(name)
	if !exists {
		return false, nil
	}
	_, exists, err := e.inspectNetwork(ctx, network)
	return err == nil && !exists, err
}

// closeNetworks removes attempt networks after their containers, verifying
// ownership labels on each exact ID. Networks that cannot be resolved yet
// are returned for a later retry.
func (e *environment) closeNetworks(ctx context.Context, networks []*trackedNetwork) ([]*trackedNetwork, []error) {
	var unresolved []*trackedNetwork
	var cleanupErrors []error
	for index := len(networks) - 1; index >= 0; index-- {
		network := networks[index]
		inspection, exists, err := e.inspectNetworkUnchecked(ctx, network)
		if err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("inspect Docker network %s before cleanup: %w", network.name, err))
			unresolved = append(unresolved, network)
			continue
		}
		if !exists {
			continue
		}
		labels := inspection.Labels
		if labels[managedLabel] != "true" || labels[schemaLabel] != "1" || labels[sessionLabel] != e.sessionID || labels[missionLabel] != e.missionID || labels[aliasLabel] != network.name || !containerIDPattern.MatchString(inspection.ID) {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("refusing to remove Docker network %s because its ownership labels do not match", network.name))
			unresolved = append(unresolved, network)
			continue
		}
		network.id = inspection.ID
		if result, err := e.run(ctx, "network", "rm", inspection.ID); err != nil && !isMissingNetwork(result, inspection.ID) {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove Docker network %s: %w", network.name, err))
			unresolved = append(unresolved, network)
		}
	}
	return unresolved, cleanupErrors
}
