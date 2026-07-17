package game

import (
	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
)

// HasCompletedCatalog reports whether every mission in the current catalog is
// complete. Completion records for removed or unknown mission IDs are ignored.
func HasCompletedCatalog(player profile.Profile, catalog mission.Catalog) bool {
	items := catalog.All()
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
