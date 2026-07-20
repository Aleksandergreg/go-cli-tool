package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	currentVersion    = 2
	defaultPlayerName = "operator"
	// MaxPlayerNameRunes bounds profile names without imposing an ASCII-only
	// policy on players.
	MaxPlayerNameRunes = 40
)

type Profile struct {
	Version   int                   `json:"version"`
	Name      string                `json:"name"`
	Onboarded bool                  `json:"onboarded,omitempty"`
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

var rankThresholds = []struct {
	name string
	xp   int
}{
	{name: "Intern", xp: 0},
	{name: "Operator", xp: 100},
	{name: "Junior Sysadmin", xp: 250},
	{name: "Sysadmin", xp: 450},
	{name: "SRE", xp: 650},
	{name: "Senior SRE", xp: 1100},
}

func New(name string) Profile {
	return Profile{
		Version:   currentVersion,
		Name:      normalizeName(name),
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
	for missionID, count := range p.Hints {
		if count < 0 || p.IsComplete(missionID) {
			delete(p.Hints, missionID)
		}
	}
	if p.Unlocked == nil {
		p.Unlocked = make(map[string]time.Time)
	}
	p.Name = normalizeName(p.Name)
}

func (p Profile) clone() Profile {
	p.Completed = maps.Clone(p.Completed)
	p.Commands = maps.Clone(p.Commands)
	p.Hints = maps.Clone(p.Hints)
	p.Unlocked = maps.Clone(p.Unlocked)
	return p
}

// ValidateName validates a user-supplied profile display name before it is
// persisted. Names remain Unicode-friendly, but terminal controls, invisible
// formatting characters, invalid UTF-8, and values too long for the CLI layout
// are rejected.
func ValidateName(name string) error {
	if !utf8.ValidString(name) {
		return fmt.Errorf("profile name must be valid UTF-8")
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("profile name cannot be blank")
	}
	for _, char := range name {
		if !unicode.IsPrint(char) {
			return fmt.Errorf("profile name cannot contain control or non-printable characters")
		}
	}
	if utf8.RuneCountInString(trimmed) > MaxPlayerNameRunes {
		return fmt.Errorf("profile name cannot exceed %d characters", MaxPlayerNameRunes)
	}
	return nil
}

// normalizeName keeps profiles created by older builds and names sourced from
// the environment safe to display. New user-supplied values are rejected by
// ValidateName in Store.Save; normalization is intentionally forgiving only at
// compatibility and default-value boundaries.
func normalizeName(name string) string {
	name = strings.ToValidUTF8(name, "")
	var cleaned strings.Builder
	for _, char := range name {
		if unicode.IsPrint(char) {
			cleaned.WriteRune(char)
		}
	}
	runes := []rune(strings.TrimSpace(cleaned.String()))
	if len(runes) > MaxPlayerNameRunes {
		runes = runes[:MaxPlayerNameRunes]
	}
	name = strings.TrimSpace(string(runes))
	if name == "" {
		return defaultPlayerName
	}
	return name
}

func (p Profile) IsComplete(missionID string) bool {
	_, complete := p.Completed[missionID]
	return complete
}

func (p *Profile) RecordCommands(commands []string) {
	if p.Commands == nil {
		p.Commands = make(map[string]int)
	}
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
	if p.Hints == nil {
		p.Hints = make(map[string]int)
	}
	p.Hints[missionID]++
	return p.Hints[missionID]
}

func (p *Profile) Complete(missionID string, xp, hints int, now time.Time) bool {
	if p.IsComplete(missionID) {
		// Hints used while replaying a mission are transient attempt state. Keep
		// the original completion and XP intact, but do not carry those hints
		// into every future replay.
		delete(p.Hints, missionID)
		return false
	}
	if p.Completed == nil {
		p.Completed = make(map[string]Completion)
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

// ActiveHints reports hints persisted for incomplete mission attempts. These
// are shown separately from the historical hints stored with completions.
func (p Profile) ActiveHints() int {
	total := 0
	for _, count := range p.Hints {
		total += count
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
	if p.Unlocked == nil {
		p.Unlocked = make(map[string]time.Time)
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
	for index := len(rankThresholds) - 1; index >= 0; index-- {
		if p.XP >= rankThresholds[index].xp {
			return rankThresholds[index].name
		}
	}
	return rankThresholds[0].name
}

func (p Profile) NextRank() (string, int, bool) {
	for _, threshold := range rankThresholds[1:] {
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
	// Validate before Normalize so controls and oversized user input cannot be
	// silently converted into a different persisted display name. Load and New
	// have already normalized compatibility/default values at their boundaries.
	if err := ValidateName(player.Name); err != nil {
		return fmt.Errorf("invalid profile name: %w", err)
	}
	// Profile is passed by value, but its maps still alias the caller. Clone
	// them before compatibility normalization so saving never mutates the live
	// session (notably when completed-mission replay hints are omitted on disk).
	player = player.clone()
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
