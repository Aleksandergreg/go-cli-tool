package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/profile"
)

func TestFreshPlayExplainsTheGameAndGuideIsReplayable(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "pwd\n", store)
	if err := app.Run([]string{"play", "1"}); err != nil {
		t.Fatalf("fresh play error = %v; stderr = %s", err, errOut.String())
	}
	for _, expected := range []string{
		"WELCOME TO OPSQUEST",
		"final outcome is what counts",
		"Hints are always available",
		"opsquest map",
		"never reach your host shell or files",
	} {
		if !strings.Contains(out.String(), expected) {
			t.Errorf("fresh guide missing %q:\n%s", expected, out.String())
		}
	}

	app, out, errOut = testApp(t, "quit\n", store)
	if err := app.Run([]string{"play", "2"}); err != nil {
		t.Fatalf("returning play error = %v; stderr = %s", err, errOut.String())
	}
	if strings.Contains(out.String(), "WELCOME TO OPSQUEST") {
		t.Fatalf("first-activity guide repeated after progress:\n%s", out.String())
	}

	app, out, errOut = testApp(t, "", store)
	if err := app.Run([]string{"guide"}); err != nil {
		t.Fatalf("guide command error = %v; stderr = %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "WELCOME TO OPSQUEST") {
		t.Fatalf("guide command did not replay onboarding:\n%s", out.String())
	}
}

func TestFirstRunGuideIsRememberedAfterImmediateQuit(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "quit\n", store)
	if err := app.Run([]string{"play", "1"}); err != nil {
		t.Fatalf("first play error = %v; stderr = %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "WELCOME TO OPSQUEST") {
		t.Fatalf("first play omitted quick start:\n%s", out.String())
	}
	player, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !player.Onboarded {
		t.Fatal("first-play onboarding marker was not persisted")
	}

	app, out, errOut = testApp(t, "quit\n", store)
	if err := app.Run([]string{"play", "1"}); err != nil {
		t.Fatalf("second play error = %v; stderr = %s", err, errOut.String())
	}
	if strings.Contains(out.String(), "WELCOME TO OPSQUEST") {
		t.Fatalf("quick start repeated after an immediate quit:\n%s", out.String())
	}
}
