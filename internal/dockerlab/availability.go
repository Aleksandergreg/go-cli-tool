package dockerlab

import (
	"context"
	"fmt"
	"strings"

	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
)

// Availability checks only the Docker resources needed by item and never
// pulls an image or otherwise mutates the daemon.
func (f *Factory) Availability(ctx context.Context, item mission.Mission) game.Availability {
	if ctx == nil {
		ctx = context.Background()
	}
	if item.EffectiveEnvironment() != mission.EnvironmentDocker {
		return game.EnvironmentAvailability(ctx, f.fallback, item)
	}
	if item.Docker == nil {
		return game.Availability{Available: false, Detail: fmt.Sprintf("Docker mission %s has no Docker setup", item.ID)}
	}
	if err := mission.ValidateDockerSetup(*item.Docker); err != nil {
		return game.Availability{Available: false, Detail: fmt.Sprintf("Docker mission %s is invalid: %v", item.ID, err)}
	}
	if f.lookupErr != nil || f.runner == nil {
		return game.Availability{
			Available: false,
			Detail:    "Docker labs unavailable: docker executable not found in PATH; install Docker Engine, Docker Desktop, or OrbStack, then run 'opsquest doctor'. Linux missions remain available.",
		}
	}
	if err := ctx.Err(); err != nil {
		return game.Availability{Available: false, Detail: fmt.Sprintf("Docker availability check canceled: %v", err)}
	}
	runtime := f.detectDockerRuntime(ctx)
	operationCtx, cancel := context.WithTimeout(ctx, dockerOperationTimeout)
	result, err := f.runner.run(operationCtx, "version", "--format", "{{.Server.Version}}")
	cancel()
	if err != nil || strings.TrimSpace(result.stdout) == "" {
		detail := dockerFailureDetail(result, err)
		instruction := "start Docker or OrbStack and check the active Docker context"
		if runtime == dockerRuntimeOrbStack {
			instruction = "start OrbStack and check the 'orbstack' Docker context"
		}
		return game.Availability{
			Available: false,
			Detail:    "Docker labs unavailable: the Docker-compatible engine could not be reached" + detail + "; " + instruction + ", then run 'opsquest doctor'. Linux missions remain available.",
		}
	}
	seen := make(map[string]bool, len(item.Docker.Images))
	for _, image := range item.Docker.Images {
		if seen[image.Reference] {
			continue
		}
		seen[image.Reference] = true
		operationCtx, cancel = context.WithTimeout(ctx, dockerOperationTimeout)
		result, err = f.runner.run(operationCtx, "image", "inspect", "--format", "{{.Id}}", image.Reference)
		cancel()
		if err != nil || strings.TrimSpace(result.stdout) == "" {
			pullCommand := "docker pull " + image.Reference
			if runtime == dockerRuntimeOrbStack {
				pullCommand = "DOCKER_CONTEXT=orbstack " + pullCommand
			}
			return game.Availability{
				Available: false,
				Detail: fmt.Sprintf(
					"Docker lab image %s is not available locally; run '%s' and try again. OpsQuest never pulls images automatically.",
					image.Reference,
					pullCommand,
				),
			}
		}
	}
	if runtime == dockerRuntimeOrbStack {
		return game.Availability{Available: true, Detail: "OrbStack is ready for this mission through the 'orbstack' Docker context."}
	}
	return game.Availability{Available: true, Detail: "Docker is ready for this mission."}
}

type dockerRuntime uint8

const (
	dockerRuntimeDefault dockerRuntime = iota
	dockerRuntimeOrbStack
)

// detectDockerRuntime identifies known Docker-compatible providers only when
// the Docker CLI reports their official context name. Detection is advisory:
// an older CLI or a broken local context store must not make an otherwise
// reachable Docker engine unavailable.
func (f *Factory) detectDockerRuntime(ctx context.Context) dockerRuntime {
	operationCtx, cancel := context.WithTimeout(ctx, dockerOperationTimeout)
	defer cancel()
	result, err := f.runner.run(operationCtx, "context", "show")
	if err == nil && strings.TrimSpace(result.stdout) == orbStackContext {
		return dockerRuntimeOrbStack
	}
	return dockerRuntimeDefault
}
