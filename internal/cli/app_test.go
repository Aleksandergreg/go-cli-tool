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

	for _, args := range [][]string{{"guide", "--help"}, {"guide", "-h"}, {"version", "--help"}, {"--version", "-h"}, {"show", "3", "--help"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
			app, out, errOut := testApp(t, "", store)
			if err := app.Run(args); err != nil {
				t.Fatalf("%v returned error: %v", args, err)
			}
			if !strings.Contains(errOut.String(), "Usage: opsquest ") || out.Len() != 0 {
				t.Fatalf("%v stdout = %q, stderr = %q; want usage only", args, out.String(), errOut.String())
			}
			if player, err := store.Load(); err != nil || player.Onboarded {
				t.Fatalf("%v changed the profile: %#v, %v", args, player, err)
			}
		})
	}
}

func TestNoArgumentCommandsRejectArguments(t *testing.T) {
	for _, args := range [][]string{{"guide", "extra"}, {"version", "extra"}, {"guide", "--bogus"}} {
		store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
		app, _, _ := testApp(t, "", store)
		if err := app.Run(args); err == nil {
			t.Errorf("%v was accepted", args)
		}
	}
}

func TestFlagErrorsAreReportedOnce(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, _, errOut := testApp(t, "", store)
	err := app.Run([]string{"show", "--bogus"})
	if err == nil || !strings.Contains(err.Error(), "flag provided but not defined: -bogus") {
		t.Fatalf("show --bogus error = %v", err)
	}
	if strings.Contains(errOut.String(), "bogus") || strings.Count(errOut.String(), "Usage: opsquest show") != 1 {
		t.Fatalf("stderr = %q; want usage once and the error left to the caller", errOut.String())
	}
}

func TestFlagsMayFollowTheMissionArgument(t *testing.T) {
	tests := []struct {
		args        []string
		wantMission string
		wantOnce    bool
		wantErr     string
	}{
		{args: []string{"4", "--once"}, wantMission: "4", wantOnce: true},
		{args: []string{"--once", "4"}, wantMission: "4", wantOnce: true},
		{args: []string{"linux-workspace", "-web", "--once"}, wantMission: "linux-workspace", wantOnce: true},
		{args: []string{"--", "--once"}, wantMission: "--once"},
		{args: []string{"4", "--track", "docker"}, wantErr: "MISSION cannot be combined"},
		{args: []string{"4", "5"}, wantErr: "usage: opsquest play"},
		{args: []string{"4", "--bogus"}, wantErr: "flag provided but not defined"},
	}
	for _, test := range tests {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			app, _, _ := testApp(t, "", profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex"))
			options, help, err := app.parsePlayOptions(test.args)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("parsePlayOptions(%q) error = %v, want %q", test.args, err, test.wantErr)
				}
				return
			}
			if err != nil || help {
				t.Fatalf("parsePlayOptions(%q) = help %v, error %v", test.args, help, err)
			}
			if options.mission != test.wantMission || options.once != test.wantOnce {
				t.Fatalf("parsePlayOptions(%q) = %#v", test.args, options)
			}
		})
	}
}
