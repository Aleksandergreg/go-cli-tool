package game

import (
	"testing"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
)

func TestHasCompletedCatalogIgnoresUnknownCompletionIDs(t *testing.T) {
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

	if HasCompletedCatalog(player, catalog) {
		t.Fatal("unknown completion replaced a missing catalog mission")
	}
	player.Complete(items[len(items)-1].ID, 0, 0, completedAt)
	if !HasCompletedCatalog(player, catalog) {
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
	if !HasCompletedCatalog(player, catalog) {
		t.Fatal("Docker mission blocked Linux catalog completion")
	}
	if HasCompletedTrack(player, catalog, mission.TrackDocker) {
		t.Fatal("incomplete Docker track reported complete")
	}
}

func TestHasCompletedCatalogRequiresAtLeastOneMission(t *testing.T) {
	player := profile.New("tester")
	player.Complete("retired-linux-mission", 0, 0, time.Now())
	if HasCompletedCatalog(player, mission.Catalog{}) {
		t.Fatal("empty catalog should not count as complete")
	}
}
