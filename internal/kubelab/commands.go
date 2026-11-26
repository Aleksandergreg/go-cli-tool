package kubelab

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
)

const help = "kubectl get pods [-l app=LABEL]\nkubectl logs POD\nkubectl get deployments\nkubectl scale deployment/NAME --replicas=N (1–3)"

type action struct {
	verb, name string
	replicas   int
}

func parse(line string, item mission.Mission) (action, error) {
	if len(line) > 512 {
		return action{}, fmt.Errorf("command exceeds 512 bytes")
	}
	words := strings.Fields(line)
	if slices.Equal(words, []string{"help"}) || slices.Equal(words, []string{"help", "kubectl"}) {
		return action{verb: "help"}, nil
	}
	if len(words) < 3 || words[0] != "kubectl" {
		return action{}, fmt.Errorf("use help kubectl for supported commands")
	}
	switch words[1] {
	case "get":
		switch words[2] {
		case "pod", "pods", "po":
			switch {
			case len(words) == 3:
				return action{verb: "pods"}, nil
			case len(words) == 5 && (words[3] == "-l" || words[3] == "--selector"):
				for _, w := range item.Kubernetes {
					if words[4] == "app="+w.App {
						return action{verb: "pods", name: w.App}, nil
					}
				}
			}
		case "deployments", "deployment", "deploy":
			if len(words) == 3 {
				return action{verb: "deployments"}, nil
			}
		}
	case "logs":
		if len(words) == 3 {
			name := strings.TrimPrefix(words[2], "pod/")
			if item.HasWorkload("Pod", name) {
				return action{verb: "logs", name: name}, nil
			}
		}
	case "scale":
		if len(words) == 4 && strings.HasPrefix(words[2], "deployment/") && strings.HasPrefix(words[3], "--replicas=") {
			name := strings.TrimPrefix(words[2], "deployment/")
			n, err := strconv.Atoi(strings.TrimPrefix(words[3], "--replicas="))
			if err == nil && n >= 1 && n <= mission.MaxReplicas && item.HasWorkload("Deployment", name) {
				return action{verb: "scale", name: name, replicas: n}, nil
			}
		}
	}
	return action{}, fmt.Errorf("unsupported arguments or unknown fixture; use help kubectl")
}

func (e *environment) PromptLabel() string                              { return "k8s/checkout" }
func (e *environment) CompletionSource() game.CompletionSource          { return e }
func (e *environment) CommandNames() []string                           { return []string{"help", "kubectl"} }
func (e *environment) PathCandidates(string) []game.CompletionCandidate { return nil }

func (e *environment) Execute(ctx context.Context, line string) (game.Execution, error) {
	a, err := parse(line, e.item)
	if err != nil {
		return game.Execution{}, err
	}
	var args []string
	switch a.verb {
	case "help":
		return game.Execution{Output: help}, nil
	case "pods":
		args = []string{"get", "pods", "-o", "custom-columns=NAME:.metadata.name,STATUS:.status.phase"}
		if a.name != "" {
			args = append(args, "-l", "app="+a.name)
		}
	case "deployments":
		args = []string{"get", "deployments", "-o", "custom-columns=NAME:.metadata.name,DESIRED:.spec.replicas,READY:.status.readyReplicas"}
	case "logs":
		args = []string{"logs", "pod/" + a.name, "--limit-bytes=1024", "--tail=10"}
	case "scale":
		args = []string{"scale", "deployment/" + a.name, "--replicas=" + strconv.Itoa(a.replicas)}
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := e.kube(ctx, "", true, args...)
	if err != nil {
		return game.Execution{}, err
	}
	return game.Execution{Output: out, PracticedCommands: []string{"kubectl"}}, nil
}

type deployment struct {
	Metadata struct{ Generation int }
	Spec     struct{ Replicas int }
	Status   struct{ ObservedGeneration, Replicas, UpdatedReplicas, ReadyReplicas int }
}

func (d deployment) ready(n int) bool {
	return d.Metadata.Generation > 0 && d.Status.ObservedGeneration == d.Metadata.Generation &&
		d.Spec.Replicas == n && d.Status.Replicas == n &&
		d.Status.UpdatedReplicas == n && d.Status.ReadyReplicas == n
}

func (e *environment) Observe(ctx context.Context, c mission.Condition) (bool, error) {
	if c.Type != mission.ConditionDeploymentReady || c.Count == nil || !e.item.HasWorkload("Deployment", c.Value) {
		return false, fmt.Errorf("invalid Kubernetes observation")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := e.kube(ctx, "", true, "get", "deployment/"+c.Value, "-o", "json")
	if err != nil {
		return false, err
	}
	var d deployment
	if err = json.Unmarshal([]byte(out), &d); err != nil {
		return false, err
	}
	return d.ready(*c.Count), nil
}
