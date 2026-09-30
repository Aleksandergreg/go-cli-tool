package dockerlab

import (
	"context"
	"errors"
	"fmt"
)

func (e *environment) Close() error {
	e.cleanupMutex.Lock()
	defer e.cleanupMutex.Unlock()

	e.mutex.Lock()
	if e.closed {
		e.mutex.Unlock()
		return nil
	}
	if !e.closing {
		e.closing = true
		e.cleanupPending = append([]*trackedContainer(nil), e.containers...)
		e.networksPending = append([]*trackedNetwork(nil), e.networks...)
	}
	containers := append([]*trackedContainer(nil), e.cleanupPending...)
	networks := append([]*trackedNetwork(nil), e.networksPending...)
	e.mutex.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	var cleanupErrors []error
	unresolved := make([]*trackedContainer, 0, len(containers))
	for index := len(containers) - 1; index >= 0; index-- {
		tracked := containers[index]
		inspection, exists, err := e.inspectTrackedForCleanup(ctx, tracked)
		if err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("inspect Docker fixture %s before cleanup: %w", tracked.alias, err))
			unresolved = append(unresolved, tracked)
			continue
		}
		if !exists {
			continue
		}
		labels := inspection.Config.Labels
		if !e.owns(labels, tracked.alias) {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("refusing to remove Docker fixture %s because its ownership labels do not match", tracked.alias))
			unresolved = append(unresolved, tracked)
			continue
		}
		tracked.id = inspection.ID
		if _, err := e.run(ctx, "container", "rm", "--force", inspection.ID); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove Docker fixture %s: %w", tracked.alias, err))
			unresolved = append(unresolved, tracked)
		}
	}

	// Networks go last: Docker refuses to remove a network while any of
	// its containers still exist.
	unresolvedNetworks, networkErrors := e.closeNetworks(ctx, networks)
	cleanupErrors = append(cleanupErrors, networkErrors...)

	e.mutex.Lock()
	e.cleanupPending = unresolved
	e.networksPending = unresolvedNetworks
	e.closed = len(unresolved) == 0 && len(unresolvedNetworks) == 0
	e.mutex.Unlock()
	return errors.Join(cleanupErrors...)
}

func (e *environment) inspectTrackedForCleanup(ctx context.Context, tracked *trackedContainer) (containerInspection, bool, error) {
	if tracked.id != "" {
		return e.inspectUnchecked(ctx, tracked.id)
	}
	inspection, exists, err := e.inspectReferenceUnchecked(ctx, tracked.actualName)
	if err != nil || !exists {
		return inspection, exists, err
	}
	if !containerIDPattern.MatchString(inspection.ID) {
		return containerInspection{}, false, fmt.Errorf("Docker inspection returned an invalid container ID")
	}
	return inspection, true, nil
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
		if !e.owns(labels, network.name) || !containerIDPattern.MatchString(inspection.ID) {
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
