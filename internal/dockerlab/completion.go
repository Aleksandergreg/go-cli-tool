package dockerlab

import (
	"sort"
	"strings"

	"github.com/aleksandergregersen/opsquest/internal/game"
)

func (e *environment) CompletionSource() game.CompletionSource {
	return dockerCompletion{environment: e}
}

type dockerCompletion struct {
	environment *environment
}

func (dockerCompletion) CommandNames() []string {
	return []string{"docker", "help"}
}

func (c dockerCompletion) PathCandidates(prefix string) []game.CompletionCandidate {
	values := []string{"--all", "--filter", "--tail", "-a", "connect", "container", "create", "disconnect", "inspect", "logs", "ls", "network", "ps", "restart", "rm", "start", "stop"}
	for _, tracked := range c.environment.snapshotContainers() {
		values = append(values, tracked.alias)
	}
	seen := make(map[string]bool)
	for _, network := range c.environment.snapshotNetworks() {
		if !seen[network.name] {
			seen[network.name] = true
			values = append(values, network.name)
		}
	}
	sort.Strings(values)
	candidates := make([]game.CompletionCandidate, 0)
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			candidates = append(candidates, game.CompletionCandidate{Value: value})
		}
	}
	return candidates
}
