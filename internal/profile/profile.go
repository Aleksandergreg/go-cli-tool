package profile

import (
	"fmt"
	"maps"
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
	// Counters are never negative in profiles OpsQuest writes; clamp hand-edited
	// values so levels, ranks, and mastery cannot display below zero.
	p.XP = max(p.XP, 0)
	ensureMap(&p.Completed)
	for missionID, completion := range p.Completed {
		completion.XP = max(completion.XP, 0)
		completion.HintsUsed = max(completion.HintsUsed, 0)
		p.Completed[missionID] = completion
	}
	ensureMap(&p.Commands)
	for command, count := range p.Commands {
		if count <= 0 {
			delete(p.Commands, command)
		}
	}
	ensureMap(&p.Hints)
	for missionID, count := range p.Hints {
		if count < 0 || p.IsComplete(missionID) {
			delete(p.Hints, missionID)
		}
	}
	ensureMap(&p.Unlocked)
	p.Name = normalizeName(p.Name)
}

func ensureMap[K comparable, V any](target *map[K]V) {
	if *target == nil {
		*target = make(map[K]V)
	}
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
	ensureMap(&p.Commands)
	for _, command := range commands {
		command = strings.TrimSpace(command)
		if command != "" {
			p.Commands[command]++
		}
	}
}

func (p Profile) MissionHints(missionID string) int { return p.Hints[missionID] }

func (p *Profile) RecordHint(missionID string) int {
	ensureMap(&p.Hints)
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
	ensureMap(&p.Completed)
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
