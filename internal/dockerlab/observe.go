package dockerlab

import (
	"context"
	"fmt"
	"strings"

	"github.com/aleksandergregersen/opsquest/internal/mission"
)

func (e *environment) Observe(ctx context.Context, condition mission.Condition) (bool, error) {
	if err := e.ready(ctx); err != nil {
		return false, err
	}
	switch condition.Type {
	case mission.ConditionDockerContainerRunning:
		tracked, exists := e.container(condition.Container)
		if !exists {
			return false, nil
		}
		inspection, exists, err := e.inspect(ctx, tracked.id)
		return exists && inspection.State.Running, err
	case mission.ConditionDockerContainerStopped:
		tracked, exists := e.container(condition.Container)
		if !exists {
			return false, nil
		}
		inspection, exists, err := e.inspect(ctx, tracked.id)
		return exists && !inspection.State.Running, err
	case mission.ConditionDockerContainerAbsent:
		tracked, exists := e.container(condition.Container)
		if !exists {
			return false, nil
		}
		_, exists, err := e.inspect(ctx, tracked.id)
		return err == nil && !exists, err
	case mission.ConditionDockerNetworkShared, mission.ConditionDockerNetworkIsolated:
		if len(condition.Containers) != 2 {
			return false, fmt.Errorf("%s requires exactly two containers", condition.Type)
		}
		shared, bothExist, err := e.shareNetwork(ctx, condition.Containers[0], condition.Containers[1])
		if err != nil || !bothExist {
			return false, err
		}
		return shared == (condition.Type == mission.ConditionDockerNetworkShared), nil
	case mission.ConditionDockerNetworkAbsent:
		return e.networkAbsent(ctx, condition.Network)
	case mission.ConditionDockerContainerCountEqual:
		if condition.Count == nil {
			return false, fmt.Errorf("docker_container_count_equals requires count")
		}
		count, err := e.countOwnedContainers(ctx)
		if err != nil {
			return false, err
		}
		return count == *condition.Count, nil
	default:
		return false, fmt.Errorf("unknown validation type %q for Docker environment", condition.Type)
	}
}

func (e *environment) countOwnedContainers(ctx context.Context) (int, error) {
	result, err := e.run(ctx,
		"container", "ls", "--all", "--quiet", "--no-trunc",
		"--filter", "label="+sessionLabel+"="+e.sessionID,
	)
	if err != nil {
		return 0, err
	}
	ids := strings.Fields(result.stdout)
	count := 0
	for _, id := range ids {
		if !containerIDPattern.MatchString(id) {
			return 0, fmt.Errorf("Docker container listing returned an invalid container ID")
		}
		inspection, exists, err := e.inspect(ctx, id)
		if err != nil {
			return 0, err
		}
		if !exists {
			continue
		}
		labels := inspection.Config.Labels
		if e.ownedByAttempt(labels) {
			count++
		}
	}
	return count, nil
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
