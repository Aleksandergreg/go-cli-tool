package game

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

// blockingReader reports when a read has started and then blocks until the
// test releases it, like standard input waiting for a line that never comes.
type blockingReader struct {
	started chan struct{}
	release chan struct{}
}

func newBlockingReader() *blockingReader {
	return &blockingReader{started: make(chan struct{}, 1), release: make(chan struct{})}
}

func (r *blockingReader) Read([]byte) (int, error) {
	r.started <- struct{}{}
	<-r.release
	return 0, io.EOF
}

func cancelOnceBlocked(t *testing.T, source *blockingReader, cancel context.CancelFunc) {
	t.Helper()
	go func() {
		<-source.started
		cancel()
	}()
	t.Cleanup(func() { close(source.release) })
}

func TestSessionCancellationWhileWaitingForInputClosesEnvironment(t *testing.T) {
	catalog, item := seamCatalogMission(t, "1")
	ctx, cancel := context.WithCancel(context.Background())
	source := newBlockingReader()
	cancelOnceBlocked(t, source, cancel)
	environment := &seamEnvironment{prompt: "/test"}
	player := profile.New("tester")
	session := Session{
		Mission: item,
		Player:  &player,
		Saver:   profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "tester"),
		In:      source,
		Out:     &bytes.Buffer{},
		ErrOut:  &bytes.Buffer{},
		Catalog: catalog,
		Context: ctx,
		Factory: FactoryFunc(func(context.Context, mission.Mission) (Environment, error) { return environment, nil }),
	}

	finished := make(chan error, 1)
	go func() {
		_, err := session.Run()
		finished <- err
	}()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v, want context cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() stayed blocked on input after the context was canceled")
	}
	if environment.closeCount != 1 {
		t.Fatalf("Close() calls = %d, want 1", environment.closeCount)
	}
}
