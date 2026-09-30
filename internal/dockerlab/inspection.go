package dockerlab

import (
	"context"
	"encoding/json"
	"fmt"
)

func (e *environment) logicalInspect(ctx context.Context, alias string) (string, error) {
	tracked, exists := e.container(alias)
	if !exists {
		return "", fmt.Errorf("docker: container %q is not part of this mission", alias)
	}
	inspection, exists, err := e.inspect(ctx, tracked.id)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("docker: container %q no longer exists", alias)
	}
	type logicalHealth struct {
		Status string `json:"Status"`
	}
	logical := struct {
		ID           string `json:"Id"`
		Name         string `json:"Name"`
		Image        string `json:"Image"`
		RestartCount int    `json:"RestartCount"`
		State        struct {
			Running    bool           `json:"Running"`
			Restarting bool           `json:"Restarting"`
			Status     string         `json:"Status"`
			ExitCode   int            `json:"ExitCode"`
			Health     *logicalHealth `json:"Health,omitempty"`
		} `json:"State"`
		HostConfig struct {
			RestartPolicy restartPolicy `json:"RestartPolicy"`
		} `json:"HostConfig"`
		NetworkSettings struct {
			Networks []string `json:"Networks"`
		} `json:"NetworkSettings"`
	}{ID: tracked.logicalID, Name: tracked.alias, Image: tracked.imageAlias, RestartCount: inspection.RestartCount}
	logical.State.Running = inspection.State.Running
	logical.State.Restarting = inspection.State.Restarting
	logical.State.Status = inspection.State.Status
	logical.State.ExitCode = inspection.State.ExitCode
	if health := healthStatus(inspection); health != "none" {
		logical.State.Health = &logicalHealth{Status: health}
	}
	logical.NetworkSettings.Networks = e.logicalNetworks(inspection)
	logical.HostConfig.RestartPolicy = inspection.HostConfig.RestartPolicy
	if logical.HostConfig.RestartPolicy.Name == "" {
		logical.HostConfig.RestartPolicy.Name = "no"
	}
	encoded, err := json.MarshalIndent(logical, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded) + "\n", nil
}

type restartPolicy struct {
	Name              string `json:"Name"`
	MaximumRetryCount int    `json:"MaximumRetryCount"`
}

type healthState struct {
	Status string `json:"Status"`
}

type containerInspection struct {
	ID           string `json:"Id"`
	Name         string `json:"Name"`
	Created      string `json:"Created"`
	RestartCount int    `json:"RestartCount"`
	Config       struct {
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	HostConfig struct {
		RestartPolicy restartPolicy `json:"RestartPolicy"`
	} `json:"HostConfig"`
	State struct {
		Running    bool         `json:"Running"`
		Restarting bool         `json:"Restarting"`
		Status     string       `json:"Status"`
		ExitCode   int          `json:"ExitCode"`
		Health     *healthState `json:"Health"`
	} `json:"State"`
	NetworkSettings struct {
		// Networks is keyed by the engine network name. NetworkID can be
		// empty before a container first starts, so both identify a network.
		Networks map[string]endpointState `json:"Networks"`
	} `json:"NetworkSettings"`
}

type endpointState struct {
	NetworkID string `json:"NetworkID"`
}

func (e *environment) inspect(ctx context.Context, id string) (containerInspection, bool, error) {
	if err := e.ready(ctx); err != nil {
		return containerInspection{}, false, err
	}
	return e.inspectUnchecked(ctx, id)
}

func (e *environment) inspectUnchecked(ctx context.Context, id string) (containerInspection, bool, error) {
	inspection, exists, err := e.inspectReferenceUnchecked(ctx, id)
	if err != nil || !exists {
		return inspection, exists, err
	}
	if inspection.ID != id {
		return containerInspection{}, false, fmt.Errorf("Docker inspection returned an unexpected container ID")
	}
	return inspection, true, nil
}

func (e *environment) inspectReferenceUnchecked(ctx context.Context, reference string) (containerInspection, bool, error) {
	return inspectReference(ctx, e.runner, reference)
}

// inspectReference inspects one exact container ID or generated name. It is
// shared by attempt environments and the orphan janitor.
func inspectReference(ctx context.Context, commandRunner runner, reference string) (containerInspection, bool, error) {
	result, err := runDocker(ctx, commandRunner, "container", "inspect", "--format", "{{json .}}", reference)
	if err != nil {
		if isMissingContainer(result) {
			return containerInspection{}, false, nil
		}
		return containerInspection{}, false, err
	}
	var inspection containerInspection
	if err := json.Unmarshal([]byte(result.stdout), &inspection); err != nil {
		return containerInspection{}, false, fmt.Errorf("decode Docker inspection: %w", err)
	}
	return inspection, true, nil
}
