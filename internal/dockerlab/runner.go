package dockerlab

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

const maxDockerCommandOutputBytes = 2 * 1024 * 1024

type runResult struct {
	stdout string
	stderr string
}

// runner is intentionally private. Higher layers issue typed Docker actions;
// only this package can translate those actions into fixed process arguments.
type runner interface {
	run(context.Context, ...string) (runResult, error)
}

type execRunner struct {
	binary string
}

func (r execRunner) run(ctx context.Context, args ...string) (runResult, error) {
	// Player input never reaches this call. Every argument is constructed from
	// a validated mission fixture or an exact resource ID tracked by this
	// package, and no host shell participates in execution.
	command := exec.CommandContext(ctx, r.binary, args...)
	stdout := newLimitedBuffer(maxDockerCommandOutputBytes)
	stderr := newLimitedBuffer(maxDockerCommandOutputBytes)
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	result := runResult{stdout: stdout.String(), stderr: stderr.String()}
	if stdout.exceeded || stderr.exceeded {
		return result, fmt.Errorf("docker command output exceeds the %d MiB limit", maxDockerCommandOutputBytes/(1024*1024))
	}
	return result, err
}

type limitedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
}

func newLimitedBuffer(limit int) *limitedBuffer {
	return &limitedBuffer{limit: limit}
}

func (b *limitedBuffer) Write(value []byte) (int, error) {
	originalLength := len(value)
	remaining := b.limit - b.buffer.Len()
	if remaining < len(value) {
		b.exceeded = true
		if remaining < 0 {
			remaining = 0
		}
		value = value[:remaining]
	}
	_, _ = b.buffer.Write(value)
	// Report the whole input as consumed so an overly talkative Docker process
	// cannot turn the bounded capture into an unrelated broken-pipe failure.
	return originalLength, nil
}

func (b *limitedBuffer) String() string {
	return b.buffer.String()
}
