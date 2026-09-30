package game

import (
	"slices"

	"github.com/aleksandergregersen/opsquest/internal/profile"
)

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

func (s Session) reportAttempt(eventType AttemptEventType, state string, outcomes []outcomeResult, hintsUsed int, replaying bool, xp int, firstCompletion bool, practiced, discovered []string, unlocked []profile.Achievement) {
	if s.Reporter == nil {
		return
	}
	placement, _ := s.Catalog.Placement(s.Mission.ID)
	revealedCount := min(hintsUsed, len(s.Mission.Hints))
	revealedHints := slices.Clone(s.Mission.Hints[:revealedCount])
	publicOutcomes := make([]AttemptOutcome, len(outcomes))
	for index, outcome := range outcomes {
		publicOutcomes[index] = AttemptOutcome{Description: outcome.Description, Satisfied: outcome.Satisfied}
	}
	achievements := make([]string, len(unlocked))
	for index, achievement := range unlocked {
		achievements[index] = achievement.Title
	}
	explanation := ""
	if state == AttemptStateCompleted {
		explanation = s.Mission.Explanation
	}
	s.Reporter.ReportAttempt(AttemptEvent{
		Type: eventType,
		Snapshot: AttemptSnapshot{
			MissionID:            s.Mission.ID,
			Number:               s.Mission.Number,
			Title:                s.Mission.Title,
			Track:                s.Mission.EffectiveTrack(),
			WorldNumber:          placement.WorldNumber,
			WorldTotal:           placement.WorldTotal,
			WorldName:            placement.WorldName,
			StageNumber:          placement.StageNumber,
			StageTotal:           placement.StageTotal,
			Difficulty:           s.Mission.Difficulty,
			Story:                s.Mission.Story,
			Objective:            s.Mission.Objective,
			SuggestedCommands:    slices.Clone(s.Mission.SuggestedCommands),
			RevealedHints:        revealedHints,
			HintCount:            len(s.Mission.Hints),
			HintsUsed:            hintsUsed,
			Outcomes:             publicOutcomes,
			SatisfiedOutcomes:    satisfiedOutcomeCount(outcomes),
			RewardAvailable:      AdjustedReward(s.Mission, hintsUsed),
			BaseReward:           s.Mission.Rewards.XP,
			Replaying:            replaying,
			State:                state,
			Explanation:          explanation,
			XPAwarded:            xp,
			FirstCompletion:      firstCompletion,
			PracticedCommands:    slices.Clone(practiced),
			DiscoveredCommands:   slices.Clone(discovered),
			UnlockedAchievements: achievements,
		},
	})
}
