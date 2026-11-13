package mission

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	dockerLogicalNamePattern    = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
	dockerImageReferencePattern = regexp.MustCompile(`^[a-z0-9]+(?:[._/-][a-z0-9]+)*(?::[A-Za-z0-9_.-]+)?@sha256:[a-f0-9]{64}$`)
)

const (
	maxDockerImagesPerMission     = 16
	maxDockerContainersPerMission = 32
	maxDockerFixtureLogBytes      = 8 * 1024
	maxDockerNetworksPerMission   = 8
	maxDockerNetworksPerContainer = 4
)

// reservedDockerNetworkNames are Docker's built-in network names. Lab
// networks cannot use them, so a logical name never suggests host or default
// bridge networking.
var reservedDockerNetworkNames = map[string]bool{"bridge": true, "default": true, "host": true, "none": true}

// ValidDockerNetworkName reports whether value is a safe logical network name
// that does not shadow one of Docker's built-in networks.
func ValidDockerNetworkName(value string) bool {
	return ValidDockerLogicalName(value) && !reservedDockerNetworkNames[value]
}

// ReservedDockerNetworkName reports whether value names a built-in Docker
// network that labs never expose.
func ReservedDockerNetworkName(value string) bool {
	return reservedDockerNetworkNames[value]
}

// ValidDockerLogicalName reports whether value is safe to use as a stable
// mission alias. Runtime adapters use this same rule so catalog-valid content
// cannot become unplayable at environment setup time.
func ValidDockerLogicalName(value string) bool {
	return dockerLogicalNamePattern.MatchString(value)
}

// ValidDockerImageReference accepts only explicit repository references
// pinned to a sha256 digest.
func ValidDockerImageReference(value string) bool {
	return dockerImageReferencePattern.MatchString(value)
}

// ValidateDockerSetup validates the declarative Docker fixture contract shared
// by catalog loading and the runtime adapter. It does not contact Docker.
func ValidateDockerSetup(setup DockerSetup) error {
	if len(setup.Images) == 0 {
		return fmt.Errorf("docker setup requires at least one image")
	}
	if len(setup.Images) > maxDockerImagesPerMission {
		return fmt.Errorf("docker setup exceeds the %d-image limit", maxDockerImagesPerMission)
	}
	if len(setup.Containers) == 0 {
		return fmt.Errorf("docker setup requires at least one container")
	}
	if len(setup.Containers) > maxDockerContainersPerMission {
		return fmt.Errorf("docker setup exceeds the %d-container limit", maxDockerContainersPerMission)
	}
	if len(setup.Networks) > maxDockerNetworksPerMission {
		return fmt.Errorf("docker setup exceeds the %d-network limit", maxDockerNetworksPerMission)
	}
	networks := make(map[string]bool, len(setup.Networks))
	for _, network := range setup.Networks {
		if !ValidDockerNetworkName(network.Name) {
			return fmt.Errorf("docker network name %q must be a lowercase logical name other than bridge, default, host, or none", network.Name)
		}
		if networks[network.Name] {
			return fmt.Errorf("duplicate docker network name %q", network.Name)
		}
		networks[network.Name] = true
	}
	images := make(map[string]bool, len(setup.Images))
	for _, image := range setup.Images {
		if !ValidDockerLogicalName(image.Alias) {
			return fmt.Errorf("docker image alias %q must be a lowercase logical name", image.Alias)
		}
		if images[image.Alias] {
			return fmt.Errorf("duplicate docker image alias %q", image.Alias)
		}
		if !ValidDockerImageReference(image.Reference) {
			return fmt.Errorf("docker image %q reference must be pinned by sha256 digest", image.Alias)
		}
		images[image.Alias] = true
	}
	containers := make(map[string]bool, len(setup.Containers))
	for _, container := range setup.Containers {
		if !ValidDockerLogicalName(container.Name) {
			return fmt.Errorf("docker container name %q must be a lowercase logical name", container.Name)
		}
		if containers[container.Name] {
			return fmt.Errorf("duplicate docker container name %q", container.Name)
		}
		if !images[container.Image] {
			return fmt.Errorf("docker container %q references unknown image alias %q", container.Name, container.Image)
		}
		switch container.State {
		case DockerStateRunning, DockerStateStopped:
		default:
			return fmt.Errorf("docker container %q has unknown state %q", container.Name, container.State)
		}
		if len(container.Log) > maxDockerFixtureLogBytes {
			return fmt.Errorf("docker container %q log exceeds the %d-byte limit", container.Name, maxDockerFixtureLogBytes)
		}
		if strings.ContainsRune(container.Log, 0) {
			return fmt.Errorf("docker container %q log cannot contain NUL", container.Name)
		}
		if err := validateDockerFixtureBehavior(container); err != nil {
			return err
		}
		if len(container.Networks) > maxDockerNetworksPerContainer {
			return fmt.Errorf("docker container %q exceeds the %d-network limit", container.Name, maxDockerNetworksPerContainer)
		}
		joined := make(map[string]bool, len(container.Networks))
		for _, network := range container.Networks {
			if !networks[network] {
				return fmt.Errorf("docker container %q references unknown network %q", container.Name, network)
			}
			if joined[network] {
				return fmt.Errorf("docker container %q joins network %q twice", container.Name, network)
			}
			joined[network] = true
		}
		containers[container.Name] = true
	}
	return nil
}

// validateDockerFixtureBehavior accepts only the fixed fixture behaviors the
// Docker adapter implements: a long-lived service with optional startup log
// and health probe, a one-shot diagnostic job, or a bounded crash loop.
func validateDockerFixtureBehavior(container DockerContainerSpec) error {
	switch container.Health {
	case "", DockerHealthHealthy, DockerHealthUnhealthy:
	default:
		return fmt.Errorf("docker container %q has unknown health %q", container.Name, container.Health)
	}
	switch container.Restart {
	case "", DockerRestartOnFailure:
	default:
		return fmt.Errorf("docker container %q has unknown restart behavior %q", container.Name, container.Restart)
	}
	if container.ExitCode == nil {
		if container.Restart != "" {
			return fmt.Errorf("docker container %q restart behavior requires log and a non-zero exit_code", container.Name)
		}
		return nil
	}
	if container.Log == "" {
		return fmt.Errorf("docker container %q exit_code requires a log", container.Name)
	}
	if *container.ExitCode < 0 || *container.ExitCode > 255 {
		return fmt.Errorf("docker container %q exit_code must be between 0 and 255", container.Name)
	}
	if container.Health != "" {
		return fmt.Errorf("docker container %q exiting fixture cannot declare health", container.Name)
	}
	if container.Restart == DockerRestartOnFailure {
		if *container.ExitCode == 0 {
			return fmt.Errorf("docker container %q restart behavior requires log and a non-zero exit_code", container.Name)
		}
		if container.State != DockerStateRunning {
			return fmt.Errorf("docker container %q crash-loop fixture must use running state", container.Name)
		}
		return nil
	}
	if container.State != DockerStateStopped {
		return fmt.Errorf("docker container %q diagnostic fixture must use stopped state", container.Name)
	}
	return nil
}

func dockerSetupHasNetwork(setup *DockerSetup, name string) bool {
	if setup == nil {
		return false
	}
	for _, network := range setup.Networks {
		if network.Name == name {
			return true
		}
	}
	return false
}

func dockerSetupHasContainer(setup *DockerSetup, name string) bool {
	if setup == nil {
		return false
	}
	for _, container := range setup.Containers {
		if container.Name == name {
			return true
		}
	}
	return false
}
