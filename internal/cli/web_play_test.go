package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
)

func TestWebPlayUsesCompanionForMissionPresentationAndLiveProgress(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "hint\npwd\n", store)
	companion := &testWebCompanion{}
	app.startCompanion = func(context.Context) (Companion, error) { return companion, nil }

	if err := app.Run([]string{"play", "--web", "--once", "1"}); err != nil {
		t.Fatalf("web play error = %v; stderr = %s", err, errOut.String())
	}
	if companion.closeCount != 1 {
		t.Fatalf("companion close count = %d, want 1", companion.closeCount)
	}
	if len(companion.events) != 4 || companion.events[0].Type != game.AttemptStarted || companion.events[1].Type != game.AttemptHint || companion.events[3].Type != game.AttemptCompleted {
		t.Fatalf("companion event types = %#v", eventTypesForCLI(companion.events))
	}
	output := out.String()
	for _, visible := range []string{
		"WEB MISSION COMPANION",
		companion.URL(),
		"Keep this terminal for commands",
		"Mission 01 ready in the web companion",
		"Hint 1/2 revealed in the web companion",
		"/home/operator",
		"✓ Mission complete!",
	} {
		if !strings.Contains(output, visible) {
			t.Errorf("web play output missing %q:\n%s", visible, output)
		}
	}
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, _ := catalog.Find("1")
	for _, hidden := range []string{item.Story, item.Objective, item.Hints[0], item.Explanation, "FIRST SHIFT BRIEFING", "Commands you may need to solve this level:"} {
		if strings.Contains(output, hidden) {
			t.Errorf("web play terminal leaked companion content %q:\n%s", hidden, output)
		}
	}
	player, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if player.XP != 30 || !player.IsComplete(item.ID) || !player.Onboarded {
		t.Fatalf("web play profile = %#v", player)
	}
}

func TestWebPlayFailsBeforeOnboardingWhenCompanionCannotStart(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, _, _ := testApp(t, "", store)
	app.startCompanion = func(context.Context) (Companion, error) {
		return nil, fmt.Errorf("loopback unavailable")
	}

	err := app.Run([]string{"play", "--web", "1"})
	if err == nil || !strings.Contains(err.Error(), "start web companion: loopback unavailable") {
		t.Fatalf("web companion start error = %v", err)
	}
	player, loadErr := store.Load()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if player.Onboarded || len(player.Completed) != 0 || player.XP != 0 {
		t.Fatalf("failed companion start changed profile = %#v", player)
	}
}

func TestWebPlayReportsCompanionCloseFailureAfterPersistingCompletion(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, _, _ := testApp(t, "pwd\n", store)
	companion := &testWebCompanion{closeErr: fmt.Errorf("shutdown stalled")}
	app.startCompanion = func(context.Context) (Companion, error) { return companion, nil }

	err := app.Run([]string{"play", "--web", "--once", "1"})
	if err == nil || !strings.Contains(err.Error(), "close web companion: shutdown stalled") {
		t.Fatalf("web companion close error = %v", err)
	}
	player, loadErr := store.Load()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if !player.IsComplete("linux-orientation") || player.XP != 40 {
		t.Fatalf("completion was not persisted before companion close error: %#v", player)
	}
}
