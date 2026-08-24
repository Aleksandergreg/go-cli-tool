package game

import "slices"

// AttemptEventType identifies one presentation-safe mission lifecycle update.
// Companion consumers may render these events, but they never participate in
// command execution or outcome validation.
type AttemptEventType string

const (
	AttemptStarted   AttemptEventType = "attempt_started"
	AttemptProgress  AttemptEventType = "attempt_progress"
	AttemptHint      AttemptEventType = "hint_revealed"
	AttemptRestarted AttemptEventType = "attempt_restarted"
	AttemptPaused    AttemptEventType = "attempt_paused"
	AttemptCompleted AttemptEventType = "attempt_completed"
)

const (
	AttemptStateActive    = "active"
	AttemptStatePaused    = "paused"
	AttemptStateCompleted = "completed"
)

// AttemptReporter receives sanitized snapshots for a presentation companion.
// ReportAttempt must return promptly and must not call back into Session. A
// reporter is not an authority boundary: Session remains the only component
// that executes commands, validates outcomes, and records completion.
type AttemptReporter interface {
	ReportAttempt(AttemptEvent)
}

// AttemptEvent carries a complete current snapshot so a browser can reconnect
// without replaying terminal input or querying the active environment.
type AttemptEvent struct {
	Type     AttemptEventType `json:"type"`
	Snapshot AttemptSnapshot  `json:"snapshot"`
}

// AttemptSnapshot is the public projection of one mission attempt. It
// deliberately excludes setup data, raw validation structures, command text,
// terminal output, and environment internals.
type AttemptSnapshot struct {
	MissionID            string           `json:"mission_id"`
	Number               int              `json:"number"`
	Title                string           `json:"title"`
	Track                string           `json:"track"`
	WorldNumber          int              `json:"world_number,omitempty"`
	WorldTotal           int              `json:"world_total,omitempty"`
	WorldName            string           `json:"world_name,omitempty"`
	StageNumber          int              `json:"stage_number,omitempty"`
	StageTotal           int              `json:"stage_total,omitempty"`
	Difficulty           string           `json:"difficulty"`
	Story                string           `json:"story"`
	Objective            string           `json:"objective"`
	SuggestedCommands    []string         `json:"suggested_commands"`
	RevealedHints        []string         `json:"revealed_hints"`
	HintCount            int              `json:"hint_count"`
	HintsUsed            int              `json:"hints_used"`
	Outcomes             []AttemptOutcome `json:"outcomes"`
	SatisfiedOutcomes    int              `json:"satisfied_outcomes"`
	RewardAvailable      int              `json:"reward_available"`
	BaseReward           int              `json:"base_reward"`
	Replaying            bool             `json:"replaying"`
	State                string           `json:"state"`
	Explanation          string           `json:"explanation,omitempty"`
	XPAwarded            int              `json:"xp_awarded,omitempty"`
	FirstCompletion      bool             `json:"first_completion,omitempty"`
	PracticedCommands    []string         `json:"practiced_commands,omitempty"`
	DiscoveredCommands   []string         `json:"discovered_commands,omitempty"`
	UnlockedAchievements []string         `json:"unlocked_achievements,omitempty"`
}

// AttemptOutcome exposes the same human-readable observable check available to
// the terminal status control without exposing its mission-schema structure.
type AttemptOutcome struct {
	Description string `json:"description"`
	Satisfied   bool   `json:"satisfied"`
}

func cloneAttemptSnapshot(snapshot AttemptSnapshot) AttemptSnapshot {
	snapshot.SuggestedCommands = slices.Clone(snapshot.SuggestedCommands)
	snapshot.RevealedHints = slices.Clone(snapshot.RevealedHints)
	snapshot.Outcomes = slices.Clone(snapshot.Outcomes)
	snapshot.PracticedCommands = slices.Clone(snapshot.PracticedCommands)
	snapshot.DiscoveredCommands = slices.Clone(snapshot.DiscoveredCommands)
	snapshot.UnlockedAchievements = slices.Clone(snapshot.UnlockedAchievements)
	return snapshot
}

// CloneAttemptEvent returns a defensive copy suitable for retaining beyond a
// ReportAttempt call.
func CloneAttemptEvent(event AttemptEvent) AttemptEvent {
	event.Snapshot = cloneAttemptSnapshot(event.Snapshot)
	return event
}
