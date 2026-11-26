package game

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/aleksandergregersen/opsquest/internal/mission"
)

// Catalog tests exercise authored objectives without provisioning a cluster.
// The adapter's integration suite runs the same incidents on kind.
type missionKubernetesEnvironment struct {
	item     mission.Mission
	replicas map[string]int
}

func newMissionKubernetesEnvironment(item mission.Mission) *missionKubernetesEnvironment {
	e := &missionKubernetesEnvironment{item: item, replicas: map[string]int{}}
	for _, w := range item.Kubernetes {
		if w.Kind == "Deployment" {
			e.replicas[w.Name] = 1
		}
	}
	return e
}

func (e *missionKubernetesEnvironment) PromptLabel() string                { return "checkout" }
func (e *missionKubernetesEnvironment) Close() error                       { return nil }
func (e *missionKubernetesEnvironment) CompletionSource() CompletionSource { return nil }
func (e *missionKubernetesEnvironment) Observe(_ context.Context, c mission.Condition) (bool, error) {
	return c.Type == mission.ConditionDeploymentReady && c.Count != nil && e.replicas[c.Value] == *c.Count, nil
}

func (e *missionKubernetesEnvironment) Execute(_ context.Context, line string) (Execution, error) {
	words := strings.Fields(line)
	var rows []string
	for _, w := range e.item.Kubernetes {
		switch {
		case len(words) == 5 && words[1] == "get" && words[4] == "app="+w.App:
			rows = append(rows, w.Name+" Running")
		case len(words) == 3 && words[1] == "logs" && words[2] == w.Name:
			return Execution{Output: w.Log}, nil
		case len(words) == 4 && words[1] == "scale" && words[2] == "deployment/"+w.Name:
			n, err := strconv.Atoi(strings.TrimPrefix(words[3], "--replicas="))
			if err != nil {
				return Execution{}, err
			}
			e.replicas[w.Name] = n
			return Execution{}, nil
		}
	}
	if len(rows) > 0 {
		return Execution{Output: strings.Join(rows, "\n")}, nil
	}
	return Execution{}, fmt.Errorf("unsupported test command %q", line)
}
