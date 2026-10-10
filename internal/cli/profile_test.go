package cli

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
)

func TestProfileRenameShowAndDoctor(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, _ := testApp(t, "", store)
	if err := app.Run([]string{"profile", "--name", "casey"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Operator: casey") {
		t.Fatalf("profile output = %s", out.String())
	}

	app, out, _ = testApp(t, "", store)
	if err := app.Run([]string{"show", "19"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Production Friday") || !strings.Contains(out.String(), "Reward: 160 XP") ||
		!strings.Contains(out.String(), "Commands you may need to solve this level:") ||
		!strings.Contains(out.String(), "  tar, sed, vi, chmod, ps, kill") {
		t.Fatalf("show output = %s", out.String())
	}

	app, out, _ = testApp(t, "", store)
	if err := app.Run([]string{"doctor"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "39 missions (23 Linux, 16 Docker)") || !strings.Contains(out.String(), "Linux labs: in-memory; no host shell or filesystem access") {
		t.Fatalf("doctor output = %s", out.String())
	}
}

func TestCommandMasteryHidesLegacyMetaCommands(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	player := profile.New("alex")
	player.RecordCommands([]string{"help", "man", "clear", "history", "grep", "ls"})
	if err := store.Save(player); err != nil {
		t.Fatal(err)
	}

	app, out, errOut := testApp(t, "", store)
	if err := app.Run([]string{"commands"}); err != nil {
		t.Fatalf("commands error = %v; stderr = %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "COMMAND MASTERY (2)") {
		t.Fatalf("commands output did not count only teaching commands:\n%s", out.String())
	}
	for _, meta := range []string{"help", "man", "clear", "history"} {
		if strings.Contains(out.String(), meta) {
			t.Errorf("commands output listed meta command %q:\n%s", meta, out.String())
		}
	}

	profileApp, profileOut, _ := testApp(t, "", store)
	if err := profileApp.Run([]string{"profile"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(profileOut.String(), "Commands mastered: 2\n") {
		t.Fatalf("profile counted meta commands:\n%s", profileOut.String())
	}
	saved, err := store.Load()
	if err != nil || saved.Commands["help"] != 1 {
		t.Fatalf("legacy meta command data = %#v, %v; want it left on disk", saved.Commands, err)
	}
}

func TestProfileRenameRejectsTerminalControlsWithoutChangingTheSave(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	player := profile.New("alex")
	if err := store.Save(player); err != nil {
		t.Fatal(err)
	}
	app, _, _ := testApp(t, "", store)
	err := app.Run([]string{"profile", "--name", "casey\x1b[2J"})
	if err == nil || !strings.Contains(err.Error(), "control or non-printable") {
		t.Fatalf("control-bearing name error = %v", err)
	}
	loaded, loadErr := store.Load()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if loaded.Name != "alex" {
		t.Fatalf("rejected rename changed profile name to %q", loaded.Name)
	}
}

func TestPipelineAndBossAchievementsUnlock(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	command := "grep ERROR incidents.log | awk '{print $3}' | sort | uniq > /reports/error-services.txt\n"
	app, out, errOut := testApp(t, command, store)
	if err := app.Run([]string{"play", "12"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}
	player, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"first-fix", "pipe-dream", "boss-slayer"} {
		if !player.HasAchievement(id) {
			t.Errorf("achievement %s was not unlocked", id)
		}
	}
	if !strings.Contains(out.String(), "Achievement unlocked: Pipe Dream") {
		t.Fatalf("completion output = %s", out.String())
	}

	app, out, _ = testApp(t, "", store)
	if err := app.Run([]string{"achievements"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "3/6 unlocked") {
		t.Fatalf("achievements output = %s", out.String())
	}
}

func TestCompletionistRequiresEveryCurrentCatalogMission(t *testing.T) {
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	items := catalog.InTrack(mission.TrackLinux)
	completedAt := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		seedFrom   int
		wantUnlock bool
	}{
		{name: "unknown ID cannot replace missing mission", seedFrom: 2, wantUnlock: false},
		{name: "all current missions still unlock", seedFrom: 1, wantUnlock: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
			player := profile.New("alex")
			for _, item := range items[test.seedFrom:] {
				player.Complete(item.ID, 0, 0, completedAt)
			}
			player.Complete("retired-linux-mission", 0, 0, completedAt)
			if err := store.Save(player); err != nil {
				t.Fatal(err)
			}

			app, out, errOut := testApp(t, "pwd\n", store)
			if err := app.Run([]string{"play", "1"}); err != nil {
				t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
			}
			player, err = store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if got := player.HasAchievement("linux-completionist"); got != test.wantUnlock {
				t.Fatalf("completionist unlocked = %v, want %v; completed = %#v", got, test.wantUnlock, player.Completed)
			}
			if announced := strings.Contains(out.String(), "Achievement unlocked: Linux Completionist"); announced != test.wantUnlock {
				t.Fatalf("completionist announcement = %v, want %v; output:\n%s", announced, test.wantUnlock, out.String())
			}
		})
	}
}
