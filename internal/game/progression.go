package game

import (
	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
)

// HasCompletedCatalog reports whether every Linux mission in the current
// catalog is complete. It retains the existing call surface for the Linux
// Completionist achievement while optional tracks can grow independently.
func HasCompletedCatalog(player profile.Profile, catalog mission.Catalog) bool {
	return HasCompletedTrack(player, catalog, mission.TrackLinux)
}

// HasCompletedTrack reports whether every mission in track is complete.
// Completion records for other tracks and removed or unknown mission IDs are
// ignored.
func HasCompletedTrack(player profile.Profile, catalog mission.Catalog, track string) bool {
	items := catalog.InTrack(track)
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if !player.IsComplete(item.ID) {
			return false
		}
	}
	return true
}
