package dockerlab

import (
	"context"
	"fmt"
	"strconv"
)

func (e *environment) startContainer(ctx context.Context, alias string) error {
	_, err := e.runOnAlias(ctx, alias, "container", "start")
	return err
}

func (e *environment) restartContainer(ctx context.Context, alias string) error {
	_, err := e.runOnAlias(ctx, alias, "container", "restart")
	return err
}

func (e *environment) stopContainer(ctx context.Context, alias string) error {
	_, err := e.runOnAlias(ctx, alias, "container", "stop")
	return err
}

// removeContainer deletes one stopped mission container. Running containers
// are refused rather than forced so the lesson keeps stop and remove distinct.
func (e *environment) removeContainer(ctx context.Context, alias string) error {
	tracked, exists := e.container(alias)
	if !exists {
		return fmt.Errorf("docker: container %q is not part of this mission", alias)
	}
	inspection, exists, err := e.inspect(ctx, tracked.id)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("docker: container %q has already been removed", alias)
	}
	if inspection.State.Running {
		return fmt.Errorf("docker: cannot remove running container %q; stop it first", alias)
	}
	_, err = e.runOnAlias(ctx, alias, "container", "rm")
	return err
}

// runOnAlias runs one fixed Docker action against the exact container ID
// behind alias. Engine errors about a missing container are translated so
// removed fixtures never reveal their real container ID.
func (e *environment) runOnAlias(ctx context.Context, alias string, args ...string) (runResult, error) {
	tracked, exists := e.container(alias)
	if !exists {
		return runResult{}, fmt.Errorf("docker: container %q is not part of this mission", alias)
	}
	result, err := e.runRedacted(ctx, []identity{tracked.identity()}, append(args, tracked.id)...)
	if err != nil && isMissingContainer(result) {
		return result, fmt.Errorf("docker: container %q has been removed", alias)
	}
	return result, err
}

func (e *environment) containerLogs(ctx context.Context, alias string, tail int) (string, error) {
	args := []string{"container", "logs"}
	if tail >= 0 {
		args = append(args, "--tail", strconv.Itoa(tail))
	}
	result, err := e.runOnAlias(ctx, alias, args...)
	if err != nil {
		return "", err
	}
	return result.stdout, nil
}
