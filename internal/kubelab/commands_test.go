package kubelab

import (
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/mission"
)

func TestTeachingGrammar(t *testing.T) {
	item := mission.Mission{Kubernetes: []mission.Workload{{Kind: "Pod", Name: "worker", App: "checkout"}, {Kind: "Deployment", Name: "api", App: "checkout"}}}
	for _, line := range []string{
		"kubectl get pods",
		"kubectl get po --selector app=checkout",
		"kubectl logs pod/worker",
		"kubectl get deploy",
		"kubectl scale deployment/api --replicas=1",
		"kubectl scale deployment/api --replicas=3",
		"help kubectl",
	} {
		if _, err := parse(line, item); err != nil {
			t.Errorf("%s: %v", line, err)
		}
	}
	for _, line := range []string{
		"kubectl get pods -A",
		"kubectl get pods -n kube-system",
		"kubectl logs worker --follow",
		"kubectl logs api",
		"kubectl logs worker; id",
		"kubectl get secrets",
		"kubectl --context prod get pods",
		"kubectl scale deployment/api --replicas=4",
		"kubectl scale deployment/api --replicas=0",
		"kubectl scale deployment/other --replicas=1",
		"help kubectl get",
	} {
		if _, err := parse(line, item); err == nil {
			t.Errorf("accepted %q", line)
		}
	}
}

func TestDeploymentReadiness(t *testing.T) {
	var d deployment
	d.Metadata.Generation = 2
	d.Status.ObservedGeneration = 1
	d.Spec.Replicas = 3
	d.Status.Replicas = 3
	d.Status.UpdatedReplicas = 3
	d.Status.ReadyReplicas = 3
	if d.ready(3) {
		t.Fatal("stale generation accepted")
	}
	d.Status.ObservedGeneration = 2
	d.Status.ReadyReplicas = 2
	if d.ready(3) {
		t.Fatal("pending Pod accepted")
	}
	d.Status.ReadyReplicas = 3
	if !d.ready(3) || d.ready(1) {
		t.Fatal("wrong readiness verdict")
	}
}
