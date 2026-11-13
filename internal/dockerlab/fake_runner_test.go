package dockerlab

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type fakeDockerRunner struct {
	mutex sync.Mutex

	calls             [][]string
	containers        map[string]*fakeContainer
	containerOrder    []string
	contextName       string
	contextErr        error
	daemonErr         error
	imageAvailable    bool
	failCreateAt      int
	failAfterCreateAt int
	createAttempts    int
	failNextStart     bool
	failNextInspect   map[string]error
	hideNextInspect   map[string]bool
	failNextRemove    map[string]error
	failManagedList   bool
	stripHealth       bool
	networks          map[string]*fakeNetwork
	networkOrder      []string
	networkCreates    int
	failNetworkCreate bool
	// contextFailure makes every invocation fail the way the Docker CLI does
	// when the selected context cannot be resolved.
	contextFailure bool
	// contextFailureOnNetworkRemove fails only network removal that way.
	contextFailureOnNetworkRemove bool
}

// contextNotFoundMessage is the Docker CLI's output when the selected
// context is missing, captured from Docker 29.7.2.
const contextNotFoundMessage = `Failed to initialize: unable to resolve docker endpoint: context "opsquest-missing-context": context not found: open /home/player/.docker/contexts/meta/fdff/meta.json: no such file or directory`

func (r *fakeDockerRunner) setContextFailure(failing bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.contextFailure = failing
}

func newFakeDockerRunner() *fakeDockerRunner {
	return &fakeDockerRunner{
		containers:      make(map[string]*fakeContainer),
		networks:        make(map[string]*fakeNetwork),
		contextName:     "default",
		imageAvailable:  true,
		failNextInspect: make(map[string]error),
		hideNextInspect: make(map[string]bool),
		failNextRemove:  make(map[string]error),
	}
}

func (r *fakeDockerRunner) run(ctx context.Context, args ...string) (runResult, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if err := ctx.Err(); err != nil {
		return runResult{}, err
	}
	r.calls = append(r.calls, append([]string(nil), args...))
	if r.contextFailure {
		return runResult{stderr: contextNotFoundMessage}, errors.New("exit status 1")
	}

	switch args[0] {
	case "container":
		return r.runContainer(args)
	case "network":
		return r.runNetwork(args)
	default:
		return r.runEngine(args)
	}
}

// runEngine fakes engine-level queries: context, version, and image checks.
func (r *fakeDockerRunner) runEngine(args []string) (runResult, error) {
	switch {
	case hasPrefix(args, "context", "show"):
		if r.contextErr != nil {
			return runResult{stderr: r.contextErr.Error()}, r.contextErr
		}
		return runResult{stdout: r.contextName + "\n"}, nil
	case hasPrefix(args, "version", "--format"):
		if r.daemonErr != nil {
			return runResult{stderr: r.daemonErr.Error()}, r.daemonErr
		}
		return runResult{stdout: "27.0.0\n"}, nil
	case hasPrefix(args, "image", "inspect", "--format"):
		if !r.imageAvailable {
			return runResult{stderr: "Error: No such image"}, errors.New("exit status 1")
		}
		return runResult{stdout: "sha256:test-image\n"}, nil
	default:
		return runResult{stderr: "unexpected fake Docker invocation"}, fmt.Errorf("unexpected Docker args: %q", args)

	}
}

func labelsMatch(labels, filters map[string]string) bool {
	for name, value := range filters {
		if labels[name] != value {
			return false
		}
	}
	return true
}

func (r *fakeDockerRunner) resetCalls() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.calls = nil
}

func (r *fakeDockerRunner) snapshotCalls() [][]string {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	result := make([][]string, len(r.calls))
	for index := range r.calls {
		result[index] = append([]string(nil), r.calls[index]...)
	}
	return result
}

func cloneStrings(source map[string]string) map[string]string {
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}
