package game

import (
	"slices"
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

// metaCommands explain or tidy the lab rather than practice an operations
// skill, so they never earn command mastery.
var metaCommands = map[string]bool{"clear": true, "help": true, "history": true, "man": true}

// CountsAsPractice reports whether a successful command earns command mastery.
func CountsAsPractice(command string) bool { return !metaCommands[command] }

// PracticedCommands returns the player's mastered commands in sorted order.
// Meta commands recorded by earlier builds remain on disk but are ignored.
func PracticedCommands(player profile.Profile) []string {
	return slices.DeleteFunc(player.MasteredCommands(), func(command string) bool {
		return !CountsAsPractice(command)
	})
}

// ReconcileCommandAchievements applies criteria that can change after an
// ordinary successful command without traversing mission content.
func ReconcileCommandAchievements(player *profile.Profile, now time.Time) []profile.Achievement {
	if player == nil || len(PracticedCommands(*player)) < 10 {
		return nil
	}
	achievement, added := player.UnlockAchievement(profile.AchievementCommandCollector, now)
	if !added {
		return nil
	}
	return []profile.Achievement{achievement}
}

// AdjustedReward returns the XP still available after hints while preserving
// the mission's minimum quarter-reward floor.
func AdjustedReward(item mission.Mission, hintsUsed int) int {
	return max(item.Rewards.XP-hintsUsed*item.Rewards.HintPenalty, item.Rewards.XP/4)
}
