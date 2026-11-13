package cli

import (
	"strings"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
	"github.com/aleksandergregersen/opsquest/internal/ui"
)

func progressBar(value, total, width int) string {
	filled := progressFilled(value, total, width)
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func styledProgressBar(style ui.Style, value, total, width int) string {
	filled := progressFilled(value, total, width)
	return style.Progress(strings.Repeat("█", filled), strings.Repeat("░", width-filled))
}

func progressFilled(value, total, width int) int {
	filled := 0
	if total > 0 {
		filled = value * width / total
	}
	return min(max(filled, 0), width)
}

func percentage(value, total int) int {
	if total == 0 {
		return 0
	}
	return value * 100 / total
}

func plural(value int, singular, plural string) string {
	if value == 1 {
		return singular
	}
	return plural
}

func completedMissions(items []mission.Mission, player profile.Profile) int {
	completed := 0
	for _, item := range items {
		if player.IsComplete(item.ID) {
			completed++
		}
	}
	return completed
}

func trackDisplayName(track string) string {
	switch track {
	case mission.TrackDocker:
		return "Docker"
	case mission.TrackLinux:
		return "Linux"
	default:
		return track
	}
}

func trackHeading(track string) string {
	switch track {
	case mission.TrackDocker:
		return "DOCKER LABS"
	case "all":
		return "ALL MISSIONS"
	default:
		return "LINUX CAMPAIGN"
	}
}
