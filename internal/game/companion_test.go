package game

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
)

type recordingAttemptReporter struct {
	events []AttemptEvent
	report func(AttemptEvent)
}

func (r *recordingAttemptReporter) ReportAttempt(event AttemptEvent) {
	event = CloneAttemptEvent(event)
	r.events = append(r.events, event)
	if r.report != nil {
		r.report(event)
	}
}

func (r *recordingAttemptReporter) event(eventType AttemptEventType) (AttemptEvent, bool) {
	for _, event := range r.events {
		if event.Type == eventType {
			return event, true
		}
	}
	return AttemptEvent{}, false
}

func TestCompanionSessionPublishesGuidanceProgressHintsAndCompletion(t *testing.T) {
	catalog, item := seamCatalogMission(t, "1")
	player := profile.New("tester")
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "tester")
	reporter := &recordingAttemptReporter{}
	out := &bytes.Buffer{}
	session := Session{
		Mission:   item,
		Player:    &player,
		Saver:     store,
		Reporter:  reporter,
		Companion: true,
		Out:       out,
		ErrOut:    &bytes.Buffer{},
		Reader:    &seamReader{lines: []string{"hint", "pwd"}},
		Catalog:   catalog,
	}

	result, err := session.Run()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Completed || result.XPAwarded != 30 {
		t.Fatalf("result = %#v", result)
	}
	wantTypes := []AttemptEventType{AttemptStarted, AttemptHint, AttemptProgress, AttemptCompleted}
	if len(reporter.events) != len(wantTypes) {
		t.Fatalf("event types = %#v, want %#v", eventTypes(reporter.events), wantTypes)
	}
	for index, want := range wantTypes {
		if reporter.events[index].Type != want {
			t.Fatalf("event %d type = %q, want %q", index, reporter.events[index].Type, want)
		}
	}

	started := reporter.events[0].Snapshot
	if started.State != AttemptStateActive || started.Title != item.Title || started.Objective != item.Objective || started.SatisfiedOutcomes != 0 {
		t.Fatalf("started snapshot = %#v", started)
	}
	if started.WorldNumber != 1 || started.StageNumber != 1 || started.HintCount != 2 || len(started.RevealedHints) != 0 {
		t.Fatalf("started placement and hints = %#v", started)
	}
	hinted := reporter.events[1].Snapshot
	if hinted.HintsUsed != 1 || len(hinted.RevealedHints) != 1 || hinted.RevealedHints[0] != item.Hints[0] || hinted.RewardAvailable != 30 {
		t.Fatalf("hint snapshot = %#v", hinted)
	}
	completed := reporter.events[3].Snapshot
	if completed.State != AttemptStateCompleted || completed.SatisfiedOutcomes != 1 || completed.XPAwarded != 30 || !completed.FirstCompletion {
		t.Fatalf("completed snapshot = %#v", completed)
	}
	if completed.Explanation != item.Explanation || len(completed.DiscoveredCommands) != 1 || completed.DiscoveredCommands[0] != "pwd" {
		t.Fatalf("completed explanation and commands = %#v", completed)
	}

	terminal := out.String()
	for _, hidden := range []string{item.Story, item.Objective, item.Hints[0], item.Explanation, "Commands you may need to solve this level:"} {
		if strings.Contains(terminal, hidden) {
			t.Errorf("companion terminal output leaked presentation text %q:\n%s", hidden, terminal)
		}
	}
	for _, visible := range []string{"Mission 01 ready in the web companion", "Hint 1/2 revealed in the web companion", "/home/operator", "✓ Mission complete!", "+30 XP"} {
		if !strings.Contains(terminal, visible) {
			t.Errorf("companion terminal output missing %q:\n%s", visible, terminal)
		}
	}

	encoded, err := json.Marshal(completed)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{`"setup"`, `"validation"`, "WELCOME.txt"} {
		if strings.Contains(string(encoded), private) {
			t.Errorf("public companion snapshot contains private mission data %q: %s", private, encoded)
		}
	}
}

func TestCompanionPublishesCompletionOnlyAfterCleanupAndPersistence(t *testing.T) {
	catalog, item := seamCatalogMission(t, "1")
	player := profile.New("tester")
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "tester")
	environment := &seamEnvironment{
		prompt: "/generic",
		execute: func(_ context.Context, _ string) (Execution, error) {
			return Execution{Output: "/home/operator\n"}, nil
		},
	}
	reporter := &recordingAttemptReporter{}
	reporter.report = func(event AttemptEvent) {
		if event.Type != AttemptCompleted {
			return
		}
		if environment.closeCount != 1 {
			t.Errorf("completion published after %d closes, want 1", environment.closeCount)
		}
		persisted, err := store.Load()
		if err != nil {
			t.Errorf("load profile during completion event: %v", err)
			return
		}
		if !persisted.IsComplete(item.ID) {
			t.Error("completion published before profile persistence")
		}
	}
	session := Session{
		Mission:   item,
		Player:    &player,
		Saver:     store,
		Reporter:  reporter,
		Companion: true,
		Out:       &bytes.Buffer{},
		ErrOut:    &bytes.Buffer{},
		Reader:    &seamReader{lines: []string{"solve"}},
		Catalog:   catalog,
		Factory: FactoryFunc(func(context.Context, mission.Mission) (Environment, error) {
			return environment, nil
		}),
	}
	if _, err := session.Run(); err != nil {
		t.Fatal(err)
	}
	if _, found := reporter.event(AttemptCompleted); !found {
		t.Fatal("completion event was not published")
	}
}

func TestCompanionReportsRestartAndPause(t *testing.T) {
	catalog, item := seamCatalogMission(t, "4")
	player := profile.New("tester")
	reporter := &recordingAttemptReporter{}
	session := Session{
		Mission:   item,
		Player:    &player,
		Saver:     profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "tester"),
		Reporter:  reporter,
		Companion: true,
		Out:       &bytes.Buffer{},
		ErrOut:    &bytes.Buffer{},
		Reader:    &seamReader{lines: []string{"touch report.txt", "restart", "quit"}},
		Catalog:   catalog,
	}
	result, err := session.Run()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Quit {
		t.Fatalf("result = %#v", result)
	}
	restarted, found := reporter.event(AttemptRestarted)
	if !found || restarted.Snapshot.SatisfiedOutcomes != 0 || restarted.Snapshot.State != AttemptStateActive {
		t.Fatalf("restart event = %#v, found = %v", restarted, found)
	}
	paused, found := reporter.event(AttemptPaused)
	if !found || paused.Snapshot.State != AttemptStatePaused {
		t.Fatalf("pause event = %#v, found = %v", paused, found)
	}
}

func eventTypes(events []AttemptEvent) []AttemptEventType {
	types := make([]AttemptEventType, len(events))
	for index, event := range events {
		types[index] = event.Type
	}
	return types
}
