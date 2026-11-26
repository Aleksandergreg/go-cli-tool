package kubelab

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/mission"
)

func TestKindLifecycle(t *testing.T) {
	if os.Getenv("OPSQUEST_KIND_TEST") != "1" {
		t.Skip("set OPSQUEST_KIND_TEST=1 for a disposable local cluster")
	}
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range catalog.InTrack(mission.TrackKubernetes) {
		t.Run(item.ID, func(t *testing.T) {
			start := time.Now()
			ctx := context.Background()
			env, err := (Factory{}).Create(ctx, item)
			if env != nil {
				t.Cleanup(func() {
					if err := env.Close(); err != nil {
						t.Error(err)
					}
				})
			}
			if err != nil {
				t.Fatal(err)
			}
			e := env.(*environment)
			if _, err := e.kube(ctx, "", true, "get", "secrets"); err == nil || !strings.Contains(err.Error(), "Forbidden") {
				t.Fatalf("namespace RBAC check: %v", err)
			}
			if _, err := e.kube(ctx, "", true, "get", "pods", "--namespace=kube-system"); err == nil || !strings.Contains(err.Error(), "Forbidden") {
				t.Fatalf("cross-namespace RBAC check: %v", err)
			}
			t.Logf("setup %s", time.Since(start).Round(time.Second))
			command := func(line string) string {
				t.Helper()
				res, err := env.Execute(ctx, line)
				if err != nil {
					t.Fatal(err)
				}
				return res.Output
			}
			switch item.Number {
			case 43:
				all := command("kubectl get pods")
				filtered := command("kubectl get pods --selector app=checkout")
				if !strings.Contains(all, "inventory") || strings.Contains(filtered, "inventory") || !strings.Contains(filtered, "checkout-api") || !strings.Contains(filtered, "checkout-worker") {
					t.Fatalf("listings %q / %q", all, filtered)
				}
				verifyRecovery(t, e)
			case 44:
				if strings.Contains(command("kubectl logs checkout-api"), "receipt queue paused") || !strings.Contains(command("kubectl logs checkout-worker"), "receipt queue paused") {
					t.Fatal("unexpected fixture logs")
				}
			case 45:
				worker := item.Validation.All[0]
				if ok, err := env.Observe(ctx, worker); err != nil || ok {
					t.Fatalf("initial worker %v %v", ok, err)
				}
				command("kubectl scale deployment/checkout-worker --replicas=3")
				deadline := time.Now().Add(time.Minute)
				for {
					ok, err := env.Observe(ctx, worker)
					if err != nil {
						t.Fatal(err)
					}
					if ok {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("rollout timed out")
					}
					time.Sleep(time.Second)
				}
				if ok, err := env.Observe(ctx, item.Validation.All[1]); err != nil || !ok {
					t.Fatalf("API %v %v", ok, err)
				}
			}
		})
	}
}

func verifyRecovery(t *testing.T, e *environment) {
	t.Helper()
	// Isolate this simulated dead-owner record from concurrent real attempts.
	root := t.TempDir()
	dir := filepath.Join(root, e.name)
	if err := os.Rename(e.dir, dir); err != nil {
		t.Fatal(err)
	}
	e.dir = dir
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	current := owner{os.Getpid(), host, e.host}
	if err := recoverAttempts(context.Background(), root, current); err != nil {
		t.Fatal(err)
	}
	if id, err := e.nodeID(context.Background()); err != nil || id == "" {
		t.Fatal("live attempt swept", err)
	}
	child := exec.Command("docker", "--version")
	if err := child.Run(); err != nil {
		t.Fatal(err)
	}
	stale := current
	stale.PID = child.Process.Pid
	data, err := json.Marshal(stale)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "owner.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err = recoverAttempts(context.Background(), root, current); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("orphan record retained", err)
	}
	if id, err := e.nodeID(context.Background()); err != nil || id != "" {
		t.Fatal("orphan node retained", id, err)
	}
}

func TestKindLifecycleCancellation(t *testing.T) {
	if os.Getenv("OPSQUEST_KIND_TEST") != "1" {
		t.Skip("requires local kind")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	host, err := prerequisites(ctx)
	if err != nil {
		t.Fatal(err)
	}
	name := clusterName()
	dir := t.TempDir()
	e := environment{name: name, dir: dir, host: host}
	finished := make(chan error, 1)
	go func() { finished <- e.setup(ctx) }()
	// Wait until setup has acquired a real node before interrupting the attempt.
	observed := false
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
wait:
	for {
		select {
		case <-ticker.C:
			if node, _ := e.nodeID(ctx); node != "" {
				observed = true
				cancel()
				break wait
			}
		case <-ctx.Done():
			break wait
		case err := <-finished:
			if cleanErr := e.Close(); cleanErr != nil {
				t.Error(cleanErr)
			}
			t.Fatalf("setup finished before interruption: %v", err)
		}
	}
	err = <-finished
	if cleanErr := e.Close(); cleanErr != nil {
		t.Fatal(cleanErr)
	}
	if !observed || err == nil {
		t.Fatalf("expected interrupted setup after node creation: %v", err)
	}
	if id, err := e.nodeID(context.Background()); err != nil || id != "" {
		t.Fatalf("remaining node %s, %v", id, err)
	}
}
