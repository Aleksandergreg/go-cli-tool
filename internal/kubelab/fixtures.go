package kubelab

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aleksandergregersen/opsquest/internal/mission"
)

const localFixture = "docker.io/opsquest/fixture:lab"

type object = map[string]any

func fixture(w mission.Workload) object {
	labels := object{"app": w.App, "workload": w.Name}
	container := object{
		"name":            "fixture",
		"image":           localFixture,
		"imagePullPolicy": "Never",
		"command":         []string{"sh", "-c", `printf '%s\n' "$1"; exec sleep 86400`, "fixture", w.Log},
		"resources":       object{"limits": object{"cpu": "50m", "memory": "16Mi"}},
		"securityContext": object{
			"allowPrivilegeEscalation": false,
			"readOnlyRootFilesystem":   true,
			"capabilities":             object{"drop": []string{"ALL"}},
		},
	}
	podSpec := object{
		"containers":                    []object{container},
		"automountServiceAccountToken":  false,
		"terminationGracePeriodSeconds": 1,
		"securityContext": object{
			"runAsNonRoot": true, "runAsUser": 65534,
			"seccompProfile": object{"type": "RuntimeDefault"},
		},
	}
	obj := object{
		"apiVersion": "v1", "kind": w.Kind,
		"metadata": object{"name": w.Name, "labels": labels}, "spec": podSpec,
	}
	if w.Kind == "Deployment" {
		obj["apiVersion"] = "apps/v1"
		obj["spec"] = object{
			"replicas": 1,
			"selector": object{"matchLabels": object{"workload": w.Name}},
			"template": object{"metadata": object{"labels": labels}, "spec": podSpec},
		}
	}
	return obj
}

func permissions(deployments []string) []object {
	const api = "rbac.authorization.k8s.io"
	rules := []object{
		{"apiGroups": []string{""}, "resources": []string{"pods", "pods/log"}, "verbs": []string{"get", "list"}},
		{"apiGroups": []string{"apps"}, "resources": []string{"deployments"}, "verbs": []string{"get", "list"}},
	}
	if len(deployments) > 0 {
		rules = append(rules, object{
			"apiGroups": []string{"apps"}, "resources": []string{"deployments/scale"},
			"verbs": []string{"get", "patch", "update"}, "resourceNames": deployments,
		})
	}
	metadata := object{"name": "operator"}
	return []object{
		{"apiVersion": "v1", "kind": "ServiceAccount", "metadata": metadata, "automountServiceAccountToken": false},
		{"apiVersion": api + "/v1", "kind": "Role", "metadata": metadata, "rules": rules},
		{
			"apiVersion": api + "/v1", "kind": "RoleBinding", "metadata": metadata,
			"subjects": []object{{"kind": "ServiceAccount", "name": "operator", "namespace": namespace}},
			"roleRef":  object{"apiGroup": api, "kind": "Role", "name": "operator"},
		},
	}
}

func (e *environment) seed(ctx context.Context) error {
	if _, err := e.kube(ctx, "", false, "create", "namespace", namespace); err != nil {
		return err
	}
	objects := []object{{
		"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy", "metadata": object{"name": "isolated"},
		"spec": object{"podSelector": object{}, "policyTypes": []string{"Ingress", "Egress"}},
	}}
	var deployments []string
	for _, w := range e.item.Kubernetes {
		objects = append(objects, fixture(w))
		if w.Kind == "Deployment" {
			deployments = append(deployments, w.Name)
		}
	}
	objects = append(objects, permissions(deployments)...)
	data, err := json.Marshal(object{"apiVersion": "v1", "kind": "List", "items": objects})
	if err != nil {
		return err
	}
	if _, err = e.kube(ctx, string(data), false, "create", "-f", "-"); err != nil {
		return err
	}
	for _, w := range e.item.Kubernetes {
		args := []string{"wait", "--for=condition=Ready", "pod/" + w.Name, "--timeout=60s"}
		if w.Kind == "Deployment" {
			args = []string{"rollout", "status", "deployment/" + w.Name, "--timeout=60s"}
		}
		if _, err = e.kube(ctx, "", false, args...); err != nil {
			return fmt.Errorf("prepare %s: %w", w.Name, err)
		}
	}
	return nil
}
