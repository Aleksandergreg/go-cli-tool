package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
)

func TestDoctorReportsAndCleansOrphanedDockerLabs(t *testing.T) {
	run := func(t *testing.T, factory game.Factory, args ...string) (string, error) {
		t.Helper()
		store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
		catalog, err := mission.LoadCatalog()
		if err != nil {
			t.Fatal(err)
		}
		out := &bytes.Buffer{}
		app := New(Config{Out: out, ErrOut: &bytes.Buffer{}, Catalog: catalog, Store: store, Factory: factory})
		err = app.Run(append([]string{"doctor"}, args...))
		return out.String(), err
	}

	t.Run("plain doctor is read-only", func(t *testing.T) {
		factory := &cliJanitorFactory{cliDockerFactory: cliDockerFactory{available: true, detail: "Docker is ready for this mission."}, orphans: 2}
		output, err := run(t, factory)
		if err != nil || !strings.Contains(output, "docker cleanup: 2 orphaned lab resources from exited OpsQuest processes; run 'opsquest doctor --cleanup'") {
			t.Fatalf("doctor output = %s, error = %v", output, err)
		}
		if factory.checks != 1 || factory.removeCalls != 0 {
			t.Fatalf("checks = %d, removals = %d", factory.checks, factory.removeCalls)
		}
	})

	t.Run("clean engine", func(t *testing.T) {
		factory := &cliJanitorFactory{cliDockerFactory: cliDockerFactory{available: true, detail: "ready"}}
		output, err := run(t, factory)
		if err != nil || !strings.Contains(output, "docker cleanup: no orphaned lab resources") {
			t.Fatalf("doctor output = %s, error = %v", output, err)
		}
	})

	t.Run("check failure is a warning", func(t *testing.T) {
		factory := &cliJanitorFactory{cliDockerFactory: cliDockerFactory{available: true, detail: "ready"}, checkErr: fmt.Errorf("list failed")}
		output, err := run(t, factory)
		if err != nil || !strings.Contains(output, "could not check for orphaned lab resources · list failed") {
			t.Fatalf("doctor output = %s, error = %v", output, err)
		}
	})

	t.Run("unavailable engine skips the check", func(t *testing.T) {
		factory := &cliJanitorFactory{cliDockerFactory: cliDockerFactory{detail: "Docker labs unavailable"}, orphans: 2}
		output, err := run(t, factory)
		if err != nil || strings.Contains(output, "docker cleanup") || factory.checks != 0 {
			t.Fatalf("doctor output = %s, error = %v, checks = %d", output, err, factory.checks)
		}
	})

	t.Run("cleanup removes orphans", func(t *testing.T) {
		factory := &cliJanitorFactory{cliDockerFactory: cliDockerFactory{available: true, detail: "ready"}, orphans: 1}
		output, err := run(t, factory, "--cleanup")
		if err != nil || !strings.Contains(output, "docker cleanup: removed 1 orphaned lab resource\n") || factory.removeCalls != 1 {
			t.Fatalf("doctor output = %s, error = %v, removals = %d", output, err, factory.removeCalls)
		}
	})

	t.Run("cleanup failure fails the command", func(t *testing.T) {
		factory := &cliJanitorFactory{cliDockerFactory: cliDockerFactory{available: true, detail: "ready"}, orphans: 2, removeErr: fmt.Errorf("daemon went away")}
		output, err := run(t, factory, "--cleanup")
		if err == nil || !strings.Contains(err.Error(), "docker cleanup failed") || !strings.Contains(output, "removed 1, then failed · daemon went away") {
			t.Fatalf("doctor output = %s, error = %v", output, err)
		}
	})

	t.Run("cleanup failure before removal", func(t *testing.T) {
		factory := &zeroRemovalJanitor{&cliJanitorFactory{cliDockerFactory: cliDockerFactory{detail: "unavailable"}, removeErr: fmt.Errorf("docker executable not found in PATH")}}
		output, err := run(t, factory, "--cleanup")
		if err == nil || !strings.Contains(output, "docker cleanup: failed · docker executable not found in PATH") || strings.Contains(output, "removed 0") {
			t.Fatalf("doctor output = %s, error = %v", output, err)
		}
	})

	t.Run("cleanup without Docker labs", func(t *testing.T) {
		output, err := run(t, game.SandboxFactory{}, "--cleanup")
		if err != nil || !strings.Contains(output, "nothing to clean; Docker labs are not configured") {
			t.Fatalf("doctor output = %s, error = %v", output, err)
		}
	})

	t.Run("rejects positional arguments", func(t *testing.T) {
		if _, err := run(t, game.SandboxFactory{}, "cleanup"); err == nil {
			t.Fatal("doctor accepted a positional argument")
		}
	})
}

func TestDoctorReportsACorruptProfileAndKeepsChecking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	factory := &cliJanitorFactory{cliDockerFactory: cliDockerFactory{available: true, detail: "Docker is ready for this mission."}}
	app := New(Config{Out: out, ErrOut: &bytes.Buffer{}, Catalog: catalog, Store: profile.NewStore(path, "alex"), Factory: factory})

	err = app.Run([]string{"doctor"})
	if err == nil || !strings.Contains(err.Error(), "profile check failed") {
		t.Fatalf("doctor error = %v, want a failed profile check", err)
	}
	output := out.String()
	for _, want := range []string{
		"embedded catalog:",
		"✗ profile: decode profile " + path,
		"opsquest reset",
		"profile path: " + path,
		"Linux labs: in-memory",
		"docker labs: ready",
		"docker cleanup: no orphaned lab resources",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("doctor output lacks %q:\n%s", want, output)
		}
	}
	if content, readErr := os.ReadFile(path); readErr != nil || string(content) != "{not json" {
		t.Fatalf("doctor changed the corrupt profile: %q, %v", content, readErr)
	}
}
