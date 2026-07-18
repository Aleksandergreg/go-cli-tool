package game

import (
	"testing"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
)

func TestHasCompletedTrackIgnoresUnknownCompletionIDs(t *testing.T) {
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	items := catalog.InTrack(mission.TrackLinux)
	player := profile.New("tester")
	completedAt := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	for _, item := range items[:len(items)-1] {
		player.Complete(item.ID, 0, 0, completedAt)
	}
	player.Complete("retired-linux-mission", 0, 0, completedAt)

	if HasCompletedTrack(player, catalog, mission.TrackLinux) {
		t.Fatal("unknown completion replaced a missing catalog mission")
	}
	player.Complete(items[len(items)-1].ID, 0, 0, completedAt)
	if !HasCompletedTrack(player, catalog, mission.TrackLinux) {
		t.Fatal("all catalog missions should be complete")
	}
}

func TestLinuxCompletionDoesNotRequireDockerTrack(t *testing.T) {
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	player := profile.New("tester")
	completedAt := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	for _, item := range catalog.InTrack(mission.TrackLinux) {
		player.Complete(item.ID, 0, 0, completedAt)
	}
	if player.IsComplete("docker-container-census") {
		t.Fatal("test unexpectedly completed Docker mission")
	}
	if !HasCompletedTrack(player, catalog, mission.TrackLinux) {
		t.Fatal("Docker mission blocked Linux catalog completion")
	}
	if HasCompletedTrack(player, catalog, mission.TrackDocker) {
		t.Fatal("incomplete Docker track reported complete")
	}
}

func TestHasCompletedTrackRequiresAtLeastOneMission(t *testing.T) {
	player := profile.New("tester")
	player.Complete("retired-linux-mission", 0, 0, time.Now())
	if HasCompletedTrack(player, mission.Catalog{}, mission.TrackLinux) {
		t.Fatal("empty catalog should not count as complete")
	}
}

func TestReconcileAchievementsUsesDurableProfileAndCatalogState(t *testing.T) {
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	player := profile.New("tester")
	completedAt := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	for index, item := range catalog.InTrack(mission.TrackLinux) {
		if index < 10 {
			player.Commands[item.ID] = 1
		}
		player.Complete(item.ID, 0, 0, completedAt)
	}

	unlocked := ReconcileAchievements(&player, catalog, completedAt)
	for _, id := range []string{
		profile.AchievementFirstFix,
		profile.AchievementCommandCollector,
		profile.AchievementSelfReliant,
		profile.AchievementBossSlayer,
		profile.AchievementLinuxCompletionist,
	} {
		if !player.HasAchievement(id) {
			t.Errorf("achievement %q was not reconciled", id)
		}
	}
	if len(unlocked) != 5 {
		t.Fatalf("newly unlocked achievements = %d, want 5", len(unlocked))
	}
	if again := ReconcileAchievements(&player, catalog, completedAt); len(again) != 0 {
		t.Fatalf("second reconciliation unlocked %#v", again)
	}
	if player.HasAchievement(profile.AchievementPipeDream) {
		t.Fatal("event-only pipeline achievement was inferred from durable state")
	}
}
