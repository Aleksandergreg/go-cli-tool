package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
)

// stalledInput never delivers the confirmation line, like a prompt left
// waiting when the player presses Ctrl-C.
type stalledInput struct {
	started chan struct{}
	release chan struct{}
}

func (r *stalledInput) Read([]byte) (int, error) {
	r.started <- struct{}{}
	<-r.release
	return 0, io.EOF
}

func TestResetPromptStopsWaitingAndKeepsProgressWhenInterrupted(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	seedCompletedMissions(t, store, "linux-orientation")
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	input := &stalledInput{started: make(chan struct{}, 1), release: make(chan struct{})}
	t.Cleanup(func() { close(input.release) })
	go func() {
		<-input.started
		cancel()
	}()
	app := New(Config{Context: ctx, In: input, Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}, Catalog: catalog, Store: store})

	finished := make(chan error, 1)
	go func() { finished <- app.Run([]string{"reset"}) }()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("reset error = %v, want context cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reset stayed blocked on its prompt after the context was canceled")
	}
	player, err := store.Load()
	if err != nil || !player.IsComplete("linux-orientation") {
		t.Fatalf("interrupted reset changed progress: %#v, %v", player.Completed, err)
	}
}
