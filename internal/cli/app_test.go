package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/buildinfo"
	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/profile"
	"github.com/aleksandergregersen/opsquest/internal/ui"
)

func TestNewUsesSafeContextAndFactoryDefaults(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, _, _ := testApp(t, "", store)
	if app.ctx == nil {
		t.Fatal("default CLI context is nil")
	}
	if _, ok := app.factory.(game.SandboxFactory); !ok {
		t.Fatalf("default factory = %T, want game.SandboxFactory", app.factory)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	factory := &cliDockerFactory{}
	configured := New(Config{
		Context: ctx,
		In:      strings.NewReader(""),
		Out:     &bytes.Buffer{},
		ErrOut:  &bytes.Buffer{},
		Catalog: app.catalog,
		Store:   store,
		Factory: factory,
	})
	if configured.ctx != ctx || configured.factory != factory {
		t.Fatalf("configured dependencies were not retained: context match %v, factory = %T", configured.ctx == ctx, configured.factory)
	}
}

func TestVersionUsesBuildInfo(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "", store)
	if err := app.Run([]string{"version"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}
	if got, want := out.String(), "OpsQuest "+buildinfo.Version+"\n"; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestNonTerminalOutputRemainsPlain(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "", store)
	if err := app.Run([]string{"list"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}
	if strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("buffered output contains ANSI escapes:\n%q", out.String())
	}
}

func TestForcedColorPreservesCLIText(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	plainApp, plainOut, plainErr := testApp(t, "", store)
	if err := plainApp.Run([]string{"show", "3"}); err != nil {
		t.Fatalf("plain Run() error = %v; stderr = %s", err, plainErr.String())
	}

	colorApp, colorOut, colorErr := testApp(t, "", store)
	colorApp.style = ui.New(true)
	colorApp.errorStyle = ui.New(true)
	if err := colorApp.Run([]string{"show", "3"}); err != nil {
		t.Fatalf("color Run() error = %v; stderr = %s", err, colorErr.String())
	}
	if !strings.Contains(colorOut.String(), "\x1b[") {
		t.Fatalf("forced-color output contains no ANSI styling:\n%q", colorOut.String())
	}
	if got := sgrPattern.ReplaceAllString(colorOut.String(), ""); got != plainOut.String() {
		t.Fatalf("color changed CLI text\ncolored without SGR:\n%q\nplain:\n%q", got, plainOut.String())
	}
}

func TestSubcommandHelpIsSuccessful(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, _, errOut := testApp(t, "", store)
	if err := app.Run([]string{"play", "--help"}); err != nil {
		t.Fatalf("play --help returned error: %v", err)
	}
	if !strings.Contains(errOut.String(), "Usage: opsquest play") {
		t.Fatalf("help output = %s", errOut.String())
	}
}
