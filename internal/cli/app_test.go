package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
)

func testApp(t *testing.T, input string, store profile.Store) (*App, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	return NewWithDependencies(strings.NewReader(input), out, errOut, catalog, store), out, errOut
}

func TestPlayAwardsHintAdjustedXPAndPersistsProgress(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "hint\npwd\n", store)
	if err := app.Run([]string{"play", "01"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "✓ Mission complete!") || !strings.Contains(out.String(), "+30 XP") {
		t.Fatalf("output did not show adjusted completion:\n%s", out.String())
	}
	player, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if player.XP != 30 || !player.IsComplete("linux-orientation") || player.Commands["pwd"] != 1 {
		t.Fatalf("profile = %#v", player)
	}
}

func TestReplayDoesNotAwardXPAgain(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	first, _, _ := testApp(t, "pwd\n", store)
	if err := first.Run([]string{"play", "1"}); err != nil {
		t.Fatal(err)
	}
	second, out, _ := testApp(t, "pwd\n", store)
	if err := second.Run([]string{"play", "linux-orientation"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "XP was already claimed") {
		t.Fatalf("replay output = %s", out.String())
	}
	player, _ := store.Load()
	if player.XP != 40 {
		t.Fatalf("XP after replay = %d, want 40", player.XP)
	}
}

func TestHintPenaltySurvivesAPausedAttempt(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	paused, _, _ := testApp(t, "hint\nquit\n", store)
	if err := paused.Run([]string{"play", "1"}); err != nil {
		t.Fatal(err)
	}
	player, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if player.MissionHints("linux-orientation") != 1 {
		t.Fatalf("saved hint progress = %d, want 1", player.MissionHints("linux-orientation"))
	}

	resumed, out, _ := testApp(t, "pwd\n", store)
	if err := resumed.Run([]string{"play", "1"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "+30 XP") {
		t.Fatalf("resumed output did not retain hint penalty:\n%s", out.String())
	}
	player, _ = store.Load()
	if player.MissionHints("linux-orientation") != 0 {
		t.Fatalf("hint progress was not cleared after completion")
	}
}

func TestListAndProfileWorkWithoutExistingSave(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "newbie")
	app, out, _ := testApp(t, "", store)
	if err := app.Run([]string{"list"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "0/10 missions complete") {
		t.Fatalf("list output = %s", out.String())
	}

	app, out, _ = testApp(t, "", store)
	if err := app.Run([]string{"profile"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Operator: newbie") || !strings.Contains(out.String(), "Rank: Intern") {
		t.Fatalf("profile output = %s", out.String())
	}
}
