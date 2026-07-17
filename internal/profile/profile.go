package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const currentVersion = 2

type Profile struct {
	Version   int                   `json:"version"`
	Name      string                `json:"name"`
	XP        int                   `json:"xp"`
	Completed map[string]Completion `json:"completed"`
	Commands  map[string]int        `json:"commands"`
	Hints     map[string]int        `json:"hint_progress,omitempty"`
	Unlocked  map[string]time.Time  `json:"achievements,omitempty"`
}

type Completion struct {
	XP          int       `json:"xp"`
	HintsUsed   int       `json:"hints_used"`
	CompletedAt time.Time `json:"completed_at"`
}

type Achievement struct {
	ID          string
	Title       string
	Description string
}

var achievements = []Achievement{
	{ID: "first-fix", Title: "First Fix", Description: "Complete your first mission."},
	{ID: "pipe-dream", Title: "Pipe Dream", Description: "Complete a successful pipeline with at least three commands."},
	{ID: "command-collector", Title: "Command Collector", Description: "Successfully practice ten different commands."},
	{ID: "self-reliant", Title: "Self-Reliant", Description: "Complete five missions without using a hint."},
	{ID: "boss-slayer", Title: "Boss Slayer", Description: "Complete an advanced mission."},
	{ID: "linux-completionist", Title: "Linux Completionist", Description: "Complete every Linux mission."},
}

func New(name string) Profile {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "operator"
	}
	return Profile{
		Version:   currentVersion,
		Name:      name,
		Completed: make(map[string]Completion),
		Commands:  make(map[string]int),
		Hints:     make(map[string]int),
		Unlocked:  make(map[string]time.Time),
	}
}

func (p *Profile) Normalize() {
	if p.Version < currentVersion {
		p.Version = currentVersion
	}
	if p.Completed == nil {
		p.Completed = make(map[string]Completion)
	}
	if p.Commands == nil {
		p.Commands = make(map[string]int)
	}
	if p.Hints == nil {
		p.Hints = make(map[string]int)
	}
	if p.Unlocked == nil {
		p.Unlocked = make(map[string]time.Time)
	}
	if strings.TrimSpace(p.Name) == "" {
		p.Name = "operator"
	}
}

func (p Profile) IsComplete(missionID string) bool {
	_, complete := p.Completed[missionID]
	return complete
}

func (p *Profile) RecordCommands(commands []string) {
	for _, command := range commands {
		command = strings.TrimSpace(command)
		if command != "" {
			p.Commands[command]++
		}
	}
}

func (p Profile) MissionHints(missionID string) int {
	return p.Hints[missionID]
}

func (p *Profile) RecordHint(missionID string) int {
	p.Hints[missionID]++
	return p.Hints[missionID]
}

func (p *Profile) Complete(missionID string, xp, hints int, now time.Time) bool {
	if p.IsComplete(missionID) {
		return false
	}
	p.Completed[missionID] = Completion{XP: xp, HintsUsed: hints, CompletedAt: now}
	delete(p.Hints, missionID)
	p.XP += xp
	return true
}

func (p Profile) MasteredCommands() []string {
	commands := make([]string, 0, len(p.Commands))
	for command := range p.Commands {
		commands = append(commands, command)
	}
	sort.Strings(commands)
	return commands
}

func (p Profile) HintsUsed() int {
	total := 0
	for _, completion := range p.Completed {
		total += completion.HintsUsed
	}
	return total
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

func AchievementDefinitions() []Achievement {
	items := make([]Achievement, len(achievements))
	copy(items, achievements)
	return items
}

func FindAchievement(id string) (Achievement, bool) {
	for _, achievement := range achievements {
		if achievement.ID == id {
			return achievement, true
		}
	}
	return Achievement{}, false
}

func (p Profile) Level() int {
	return p.XP/100 + 1
}

func (p Profile) Rank() string {
	switch {
	case p.XP >= 1100:
		return "Senior SRE"
	case p.XP >= 650:
		return "SRE"
	case p.XP >= 450:
		return "Sysadmin"
	case p.XP >= 250:
		return "Junior Sysadmin"
	case p.XP >= 100:
		return "Operator"
	default:
		return "Intern"
	}
}

func (p Profile) NextRank() (string, int, bool) {
	thresholds := []struct {
		name string
		xp   int
	}{
		{name: "Operator", xp: 100},
		{name: "Junior Sysadmin", xp: 250},
		{name: "Sysadmin", xp: 450},
		{name: "SRE", xp: 650},
		{name: "Senior SRE", xp: 1100},
	}
	for _, threshold := range thresholds {
		if p.XP < threshold.xp {
			return threshold.name, threshold.xp - p.XP, true
		}
	}
	return "", 0, false
}

type Store struct {
	path string
	name func() string
}

func DefaultStore() (Store, error) {
	root := strings.TrimSpace(os.Getenv("OPSQUEST_HOME"))
	if root == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return Store{}, fmt.Errorf("locate config directory: %w", err)
		}
		root = filepath.Join(configDir, "opsquest")
	}
	return Store{
		path: filepath.Join(root, "profile.json"),
		name: func() string {
			if value := strings.TrimSpace(os.Getenv("OPSQUEST_PLAYER")); value != "" {
				return value
			}
			if value := strings.TrimSpace(os.Getenv("USER")); value != "" {
				return value
			}
			return "operator"
		},
	}, nil
}

func NewStore(path, playerName string) Store {
	return Store{path: path, name: func() string { return playerName }}
}

func (s Store) Path() string {
	return s.path
}

func (s Store) Load() (Profile, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return New(s.name()), nil
	}
	if err != nil {
		return Profile{}, fmt.Errorf("read profile: %w", err)
	}
	var player Profile
	if err := json.Unmarshal(data, &player); err != nil {
		return Profile{}, fmt.Errorf("decode profile: %w", err)
	}
	if player.Version > currentVersion {
		return Profile{}, fmt.Errorf("profile version %d is newer than this OpsQuest build", player.Version)
	}
	player.Normalize()
	return player, nil
}

func (s Store) Save(player Profile) error {
	player.Normalize()
	data, err := json.MarshalIndent(player, "", "  ")
	if err != nil {
		return fmt.Errorf("encode profile: %w", err)
	}
	data = append(data, '\n')
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create profile directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".profile-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary profile: %w", err)
	}
	temporaryName := temporary.Name()
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("secure temporary profile: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write profile: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync profile: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close profile: %w", err)
	}
	if err := os.Rename(temporaryName, s.path); err != nil {
		return fmt.Errorf("replace profile: %w", err)
	}
	removeTemporary = false
	return nil
}

func (s Store) Reset() (bool, error) {
	err := os.Remove(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("remove profile: %w", err)
	}
	return true, nil
}
