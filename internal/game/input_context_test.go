package game

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
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

func TestContextReaderPassesDataThroughUntilCanceled(t *testing.T) {
	if source := strings.NewReader("pwd\n"); NewContextReader(context.Background(), source) != io.Reader(source) {
		t.Fatal("a context that cannot end should not wrap the source")
	}

	ctx, cancel := context.WithCancel(context.Background())
	reader := NewContextReader(ctx, strings.NewReader("pwd\nls\n"))
	content, err := io.ReadAll(reader)
	if err != nil || string(content) != "pwd\nls\n" {
		t.Fatalf("ReadAll() = %q, %v", content, err)
	}
	cancel()
	if _, err := reader.Read(make([]byte, 8)); !errors.Is(err, context.Canceled) {
		t.Fatalf("Read() after cancellation error = %v, want context cancellation", err)
	}
}

func TestContextReaderAbandonsABlockedRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	source := newBlockingReader()
	cancelOnceBlocked(t, source, cancel)

	finished := make(chan error, 1)
	go func() {
		_, err := NewContextReader(ctx, source).Read(make([]byte, 8))
		finished <- err
	}()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Read() error = %v, want context cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Read() stayed blocked after the context was canceled")
	}
}

func TestScriptedLineReaderStopsWaitingWhenContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	source := newBlockingReader()
	cancelOnceBlocked(t, source, cancel)

	reader := NewCommandLineReaderContext(ctx, source, &bytes.Buffer{})
	if _, err := reader.ReadLine("opsquest:/work$ ", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("ReadLine() error = %v, want context cancellation", err)
	}
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
