// Package kubelab provisions one local kind cluster per Kubernetes mission.
package kubelab

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
)

const (
	nodeImage    = "kindest/node:v1.35.8@sha256:07b2536e30b803ed61d1677a79df6115f798ce64c80f9e22f6ed45afd09323c0"
	fixtureImage = "docker.io/library/busybox@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662"
	namespace    = "checkout"
)

type Factory struct{}

type environment struct {
	item            mission.Mission
	dir, name, host string
}

func run(ctx context.Context, env []string, input io.Reader, program string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = input
	cmd.WaitDelay = 2 * time.Second
	output, err := cmd.Output()
	if err != nil {
		var failure *exec.ExitError
		if errors.As(err, &failure) {
			return "", fmt.Errorf("%s: %w: %.2000s", program, err, strings.TrimSpace(string(failure.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// Freeze the endpoint so changing the active context during play cannot
// redirect later setup or cleanup operations.
func prerequisites(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for _, name := range []string{"docker", "kind", "kubectl"} {
		if _, err := exec.LookPath(name); err != nil {
			return "", fmt.Errorf("install %s to play Kubernetes labs", name)
		}
	}
	host := os.Getenv("DOCKER_HOST")
	if host == "" || os.Getenv("DOCKER_CONTEXT") != "" {
		var err error
		host, err = run(ctx, nil, nil, "docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}")
		if err != nil {
			return "", err
		}
	}
	if !strings.HasPrefix(host, "unix:///") && !strings.HasPrefix(host, "npipe:////./pipe/") {
		return "", errors.New("Kubernetes labs require a local Docker endpoint")
	}
	for _, image := range []string{nodeImage, fixtureImage} {
		if _, err := run(ctx, []string{"DOCKER_CONTEXT=", "DOCKER_HOST=" + host}, nil, "docker", "image", "inspect", image, "--format", "{{.Id}}"); err != nil {
			return "", fmt.Errorf("start local Docker and preload the fixture with docker pull %s: %w", image, err)
		}
	}
	return host, nil
}

func (f Factory) Availability(ctx context.Context, item mission.Mission) game.Availability {
	if item.EffectiveEnvironment() != mission.EnvironmentKubernetes {
		return game.Availability{Detail: fmt.Sprintf("Kubernetes factory does not support environment %q", item.EffectiveEnvironment())}
	}
	if _, err := prerequisites(ctx); err != nil {
		return game.Availability{Detail: err.Error()}
	}
	return game.Availability{Available: true, Detail: "local kind cluster prerequisites ready"}
}

func (f Factory) Create(ctx context.Context, item mission.Mission) (game.Environment, error) {
	if item.EffectiveEnvironment() != mission.EnvironmentKubernetes {
		return nil, fmt.Errorf("Kubernetes factory does not support environment %q", item.EffectiveEnvironment())
	}
	if err := mission.ValidateWorkloads(item.Kubernetes); err != nil {
		return nil, err
	}
	host, err := prerequisites(ctx)
	if err != nil {
		return nil, err
	}
	name := clusterName()
	dir, err := newAttemptDir(ctx, name, host)
	if err != nil {
		return nil, err
	}
	e := &environment{item: item, dir: dir, name: name, host: host}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	return e, e.setup(ctx)
}

func (e *environment) command(ctx context.Context, input, name string, args ...string) (string, error) {
	config := filepath.Join(e.dir, "config")
	env := []string{
		"KIND_EXPERIMENTAL_PROVIDER=docker", "KIND_EXPERIMENTAL_DOCKER_NETWORK=" + e.name,
		"KUBECONFIG=" + config, "KUBECTL_KUBERC=false", "DOCKER_CONTEXT=", "DOCKER_HOST=" + e.host,
	}
	return run(ctx, env, strings.NewReader(input), name, args...)
}

func (e *environment) kube(ctx context.Context, input string, player bool, args ...string) (string, error) {
	base := []string{"--kubeconfig", filepath.Join(e.dir, "config"), "--context", "kind-" + e.name, "--cache-dir", filepath.Join(e.dir, "cache"), "--request-timeout=15s", "--namespace", namespace}
	if player {
		base = append(base, "--as=system:serviceaccount:checkout:operator")
	}
	return e.command(ctx, input, "kubectl", append(base, args...)...)
}

func (e *environment) setup(ctx context.Context) error {
	if _, err := e.command(ctx, "", "docker", "network", "create", "--driver", "bridge", "--label", "com.opsquest.kind="+e.name, e.name); err != nil {
		return err
	}
	const config = "kind: Cluster\napiVersion: kind.x-k8s.io/v1alpha4\nnetworking:\n  apiServerAddress: 127.0.0.1\n"
	if _, err := e.command(ctx, config, "kind", "create", "cluster", "--config", "-", "--retain", "--name", e.name, "--image", nodeImage, "--kubeconfig", filepath.Join(e.dir, "config"), "--wait", "2m"); err != nil {
		return err
	}
	if err := e.loadFixture(ctx); err != nil {
		return err
	}
	return e.seed(ctx)
}

func (e *environment) loadFixture(ctx context.Context) error {
	archive := filepath.Join(e.dir, "fixture.tar")
	platform, err := e.command(ctx, "", "docker", "image", "inspect", "--format", "{{.Os}}/{{.Architecture}}", fixtureImage)
	if err != nil {
		return err
	}
	if _, err = e.command(ctx, "", "docker", "image", "save", "--platform", platform, "-o", archive, fixtureImage); err != nil {
		return err
	}
	input, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer input.Close()
	node, err := e.nodeID(ctx)
	if err != nil {
		return err
	}
	if len(node) != 64 {
		return errors.New("missing attempt-owned kind node")
	}
	// kind's image-archive loader requests all platforms. A local Docker
	// store may contain only the host architecture from a pinned image index.
	if _, err = run(ctx, []string{"DOCKER_CONTEXT=", "DOCKER_HOST=" + e.host}, input, "docker", "exec", "-i", node, "ctr", "--namespace=k8s.io", "images", "import", "--platform", platform, "--index-name", localFixture, "-"); err != nil {
		return err
	}
	return nil
}

// Filter by both the generated identity and provider labels. Remove only IDs
// returned by Docker, including on a partial setup or a retry after failure.
func (e *environment) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	ids, err := e.nodeID(ctx)
	if err != nil {
		return err
	}
	for _, id := range strings.Fields(ids) {
		if _, err = e.command(ctx, "", "docker", "rm", "-fv", id); err != nil {
			return err
		}
	}
	ids, err = e.command(ctx, "", "docker", "network", "ls", "-q", "--no-trunc", "--filter", "label=com.opsquest.kind="+e.name, "--filter", "name=^"+e.name+"$")
	if err != nil {
		return err
	}
	for _, id := range strings.Fields(ids) {
		if _, err = e.command(ctx, "", "docker", "network", "rm", id); err != nil {
			return err
		}
	}
	return os.RemoveAll(e.dir)
}

func (e *environment) nodeID(ctx context.Context) (string, error) {
	return e.command(ctx, "", "docker", "ps", "-aq", "--no-trunc", "--filter", "label=io.x-k8s.kind.cluster="+e.name, "--filter", "label=io.x-k8s.kind.role=control-plane", "--filter", "name=^/"+e.name+"-control-plane$")
}

func clusterName() string {
	var nonce [16]byte
	rand.Read(nonce[:])
	return "opsquest-" + hex.EncodeToString(nonce[:])
}
