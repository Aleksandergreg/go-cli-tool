package game

import (
	"time"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
)

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

// ReconcileAchievements applies every achievement derived from durable
// profile and catalog state. Event-only achievements, such as Pipe Dream,
// remain attached to the command event that proves them.
func ReconcileAchievements(player *profile.Profile, catalog mission.Catalog, now time.Time) []profile.Achievement {
	if player == nil {
		return nil
	}
	unlocked := make([]profile.Achievement, 0)
	unlock := func(id string) {
		if achievement, added := player.UnlockAchievement(id, now); added {
			unlocked = append(unlocked, achievement)
		}
	}
	if len(player.Completed) > 0 {
		unlock(profile.AchievementFirstFix)
	}
	unlocked = append(unlocked, ReconcileCommandAchievements(player, now)...)
	if player.HintFreeCompletions() >= 5 {
		unlock(profile.AchievementSelfReliant)
	}
	for _, item := range catalog.All() {
		if player.IsComplete(item.ID) && item.Difficulty == mission.DifficultyAdvanced {
			unlock(profile.AchievementBossSlayer)
			break
		}
	}
	if HasCompletedTrack(*player, catalog, mission.TrackLinux) {
		unlock(profile.AchievementLinuxCompletionist)
	}
	return unlocked
}

// ReconcileCommandAchievements applies criteria that can change after an
// ordinary successful command without traversing mission content.
func ReconcileCommandAchievements(player *profile.Profile, now time.Time) []profile.Achievement {
	if player == nil || len(player.Commands) < 10 {
		return nil
	}
	achievement, added := player.UnlockAchievement(profile.AchievementCommandCollector, now)
	if !added {
		return nil
	}
	return []profile.Achievement{achievement}
}
