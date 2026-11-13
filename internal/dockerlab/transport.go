package dockerlab

import (
	"context"
	"fmt"
	"strings"
)

func (e *environment) run(ctx context.Context, args ...string) (runResult, error) {
	return runDocker(ctx, e.runner, args...)
}

// runDocker runs one fixed Docker invocation with the standard operation
// timeout and a bounded error summary.
func runDocker(ctx context.Context, commandRunner runner, args ...string) (runResult, error) {
	operationCtx, cancel := context.WithTimeout(ctx, dockerOperationTimeout)
	defer cancel()
	result, err := commandRunner.run(operationCtx, args...)
	if err != nil {
		return result, fmt.Errorf("Docker command failed%s", dockerFailureDetail(result, err))
	}
	return result, nil
}

// runRedacted runs one fixed invocation and rewrites a failure so none of
// the given resources' engine identities reach the player.
func (e *environment) runRedacted(ctx context.Context, identities []identity, args ...string) (runResult, error) {
	operationCtx, cancel := context.WithTimeout(ctx, dockerOperationTimeout)
	defer cancel()
	result, err := e.runner.run(operationCtx, args...)
	if err != nil {
		return result, fmt.Errorf("Docker command failed%s", redact(dockerFailureDetail(result, err), identities...))
	}
	return result, nil
}

// identity pairs a resource's exact engine identifiers with the logical names
// players see.
type identity struct {
	id         string
	actualName string
	logicalID  string
	name       string
}

func (t *trackedContainer) identity() identity {
	return identity{id: t.id, actualName: t.actualName, logicalID: t.logicalID, name: t.alias}
}

func (n *trackedNetwork) identity() identity {
	return identity{id: n.id, actualName: n.actualName, logicalID: n.logicalID, name: n.name}
}

// redact replaces exact engine IDs, including their 12-character short form,
// and generated names with logical identities in player-visible text.
func redact(message string, identities ...identity) string {
	for _, resource := range identities {
		if resource.id != "" {
			message = strings.ReplaceAll(message, resource.id, resource.logicalID)
			if len(resource.id) >= 12 {
				message = strings.ReplaceAll(message, resource.id[:12], resource.logicalID)
			}
		}
		message = strings.ReplaceAll(message, resource.actualName, resource.name)
	}
	return message
}

func isMissingContainer(result runResult) bool {
	message := strings.ToLower(result.stderr + "\n" + result.stdout)
	return strings.Contains(message, "no such container") || strings.Contains(message, "no such object")
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

func dockerFailureDetail(result runResult, err error) string {
	message := strings.TrimSpace(result.stderr)
	if message == "" && err != nil {
		message = err.Error()
	}
	if message == "" {
		return ""
	}
	if len(message) > 240 {
		message = message[:240] + "…"
	}
	return ": " + message
}
