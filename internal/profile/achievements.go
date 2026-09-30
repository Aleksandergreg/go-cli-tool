package profile

import (
	"slices"
	"time"
)

type Achievement struct {
	ID          string
	Title       string
	Description string
}

const (
	AchievementFirstFix           = "first-fix"
	AchievementPipeDream          = "pipe-dream"
	AchievementCommandCollector   = "command-collector"
	AchievementSelfReliant        = "self-reliant"
	AchievementBossSlayer         = "boss-slayer"
	AchievementLinuxCompletionist = "linux-completionist"
)

var achievements = []Achievement{
	{ID: AchievementFirstFix, Title: "First Fix", Description: "Complete your first mission."},
	{ID: AchievementPipeDream, Title: "Pipe Dream", Description: "Complete a successful pipeline with at least three commands."},
	{ID: AchievementCommandCollector, Title: "Command Collector", Description: "Successfully practice ten different commands."},
	{ID: AchievementSelfReliant, Title: "Self-Reliant", Description: "Complete five missions without using a hint."},
	{ID: AchievementBossSlayer, Title: "Boss Slayer", Description: "Complete an advanced mission."},
	{ID: AchievementLinuxCompletionist, Title: "Linux Completionist", Description: "Complete every Linux mission."},
}

func (p Profile) HintFreeCompletions() int {
	total := 0
	for _, completion := range p.Completed {
		if completion.HintsUsed == 0 {
			total++
		}
	}
	return total
}

func (p *Profile) UnlockAchievement(id string, now time.Time) (Achievement, bool) {
	definition, exists := FindAchievement(id)
	if !exists {
		return Achievement{}, false
	}
	ensureMap(&p.Unlocked)
	if _, unlocked := p.Unlocked[id]; unlocked {
		return definition, false
	}
	p.Unlocked[id] = now
	return definition, true
}

func (p Profile) HasAchievement(id string) bool {
	_, unlocked := p.Unlocked[id]
	return unlocked
}

func (p Profile) AchievementCount() int {
	total := 0
	for _, achievement := range achievements {
		if p.HasAchievement(achievement.ID) {
			total++
		}
	}
	return total
}

func AchievementDefinitions() []Achievement {
	return slices.Clone(achievements)
}

func FindAchievement(id string) (Achievement, bool) {
	for _, achievement := range achievements {
		if achievement.ID == id {
			return achievement, true
		}
	}
	return Achievement{}, false
}
