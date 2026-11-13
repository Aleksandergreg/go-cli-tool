package dockerlab

import (
	"context"
	"errors"
	"fmt"
	"sync"
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
	unresolved, cleanupErrors := e.closeContainers(ctx, containers)

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

// cleanupParallelism bounds how many Docker CLI processes Close runs at
// once. Removing resources one after another let a large mission exhaust the
// shared cleanup budget and leave its last fixtures behind.
const cleanupParallelism = 8

// cleanupConcurrently runs cleanup for every item in reverse creation order
// with bounded parallelism. It returns the items whose cleanup failed, to be
// retried by a later Close, together with their errors in a stable order.
func cleanupConcurrently[T any](items []T, cleanup func(T) error) ([]T, []error) {
	failures := make([]error, len(items))
	limit := make(chan struct{}, cleanupParallelism)
	var group sync.WaitGroup
	for index := range items {
		group.Add(1)
		limit <- struct{}{}
		go func() {
			defer group.Done()
			defer func() { <-limit }()
			failures[index] = cleanup(items[index])
		}()
	}
	group.Wait()

	var unresolved []T
	var cleanupErrors []error
	for index := len(items) - 1; index >= 0; index-- {
		if failures[index] != nil {
			unresolved = append(unresolved, items[index])
			cleanupErrors = append(cleanupErrors, failures[index])
		}
	}
	return unresolved, cleanupErrors
}

// closeContainers removes attempt containers after verifying ownership
// labels on each exact ID. Containers that cannot be resolved yet are
// returned for a later retry.
func (e *environment) closeContainers(ctx context.Context, containers []*trackedContainer) ([]*trackedContainer, []error) {
	return cleanupConcurrently(containers, func(tracked *trackedContainer) error {
		inspection, exists, err := e.inspectTrackedForCleanup(ctx, tracked)
		if err != nil {
			return fmt.Errorf("inspect Docker fixture %s before cleanup: %w", tracked.alias, err)
		}
		if !exists {
			return nil
		}
		if !e.owns(inspection.Config.Labels, tracked.alias) {
			return fmt.Errorf("refusing to remove Docker fixture %s because its ownership labels do not match", tracked.alias)
		}
		tracked.id = inspection.ID
		if _, err := e.run(ctx, "container", "rm", "--force", inspection.ID); err != nil {
			return fmt.Errorf("remove Docker fixture %s: %w", tracked.alias, err)
		}
		return nil
	})
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
	return cleanupConcurrently(networks, func(network *trackedNetwork) error {
		inspection, exists, err := e.inspectNetworkUnchecked(ctx, network)
		if err != nil {
			return fmt.Errorf("inspect Docker network %s before cleanup: %w", network.name, err)
		}
		if !exists {
			return nil
		}
		if !e.owns(inspection.Labels, network.name) || !containerIDPattern.MatchString(inspection.ID) {
			return fmt.Errorf("refusing to remove Docker network %s because its ownership labels do not match", network.name)
		}
		network.id = inspection.ID
		if result, err := e.run(ctx, "network", "rm", inspection.ID); err != nil && !isMissingNetwork(result, inspection.ID) {
			return fmt.Errorf("remove Docker network %s: %w", network.name, err)
		}
		return nil
	})
}
