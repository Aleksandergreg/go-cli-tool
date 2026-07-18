package profile

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreRoundTripAndReset(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "nested", "profile.json"), "alex")
	player, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if player.Name != "alex" {
		t.Fatalf("new profile name = %q", player.Name)
	}
	player.RecordCommands([]string{"pwd", "pwd"})
	if achievement, added := player.UnlockAchievement("first-fix", time.Unix(1, 0)); !added || achievement.Title != "First Fix" {
		t.Fatalf("UnlockAchievement() = %#v, %v", achievement, added)
	}
	if _, added := player.UnlockAchievement("first-fix", time.Unix(2, 0)); added {
		t.Fatal("achievement unlocked twice")
	}
	if !player.Complete("mission-1", 40, 1, time.Unix(1, 0)) {
		t.Fatal("first completion was not recorded")
	}
	if player.Complete("mission-1", 40, 0, time.Unix(2, 0)) {
		t.Fatal("duplicate completion was recorded")
	}
	if err := store.Save(player); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.XP != 40 || loaded.Commands["pwd"] != 2 || loaded.HintsUsed() != 1 || !loaded.HasAchievement("first-fix") {
		t.Fatalf("loaded profile = %#v", loaded)
	}
	if next, needed, ok := loaded.NextRank(); !ok || next != "Operator" || needed != 60 {
		t.Fatalf("NextRank() = %q, %d, %v", next, needed, ok)
	}
	removed, err := store.Reset()
	if err != nil || !removed {
		t.Fatalf("Reset() = %v, %v", removed, err)
	}
	removed, err = store.Reset()
	if err != nil || removed {
		t.Fatalf("second Reset() = %v, %v", removed, err)
	}
}

func TestCompleteClearsReplayHintsWithoutChangingOriginalCompletion(t *testing.T) {
	player := New("alex")
	missionID := "linux-orientation"
	completedAt := time.Unix(1, 0)
	player.RecordHint(missionID)
	if !player.Complete(missionID, 30, 1, completedAt) {
		t.Fatal("first completion was not recorded")
	}
	original := player.Completed[missionID]

	player.RecordHint(missionID)
	if player.Complete(missionID, 40, 0, time.Unix(2, 0)) {
		t.Fatal("replay was recorded as a new completion")
	}
	if got := player.MissionHints(missionID); got != 0 {
		t.Fatalf("replay hint progress = %d, want 0", got)
	}
	if player.XP != 30 {
		t.Fatalf("XP after replay = %d, want 30", player.XP)
	}
	if got := player.Completed[missionID]; got != original {
		t.Fatalf("replay changed original completion: got %#v, want %#v", got, original)
	}
}

func TestNormalizeRemovesNegativeHintProgress(t *testing.T) {
	player := New("alex")
	player.Hints["negative"] = -2
	player.Hints["valid"] = 1

	player.Normalize()

	if _, exists := player.Hints["negative"]; exists {
		t.Fatalf("negative hint progress survived normalization: %#v", player.Hints)
	}
	if got := player.MissionHints("valid"); got != 1 {
		t.Fatalf("valid hint progress = %d, want 1", got)
	}
}

func TestNormalizeRemovesLegacyHintProgressForCompletedMissions(t *testing.T) {
	player := New("operator")
	player.Completed["completed"] = Completion{XP: 25, HintsUsed: 1, CompletedAt: time.Unix(1, 0)}
	player.Hints["completed"] = 3
	player.Hints["in-progress"] = 2

	player.Normalize()

	if got := player.MissionHints("completed"); got != 0 {
		t.Fatalf("completed mission hints = %d, want 0", got)
	}
	if got := player.MissionHints("in-progress"); got != 2 {
		t.Fatalf("in-progress mission hints = %d, want 2", got)
	}
}

func TestRankAndNextRankShareThresholds(t *testing.T) {
	tests := []struct {
		xp      int
		rank    string
		next    string
		needed  int
		hasNext bool
	}{
		{xp: 0, rank: "Intern", next: "Operator", needed: 100, hasNext: true},
		{xp: 100, rank: "Operator", next: "Junior Sysadmin", needed: 150, hasNext: true},
		{xp: 649, rank: "Sysadmin", next: "SRE", needed: 1, hasNext: true},
		{xp: 1100, rank: "Senior SRE", hasNext: false},
	}
	for _, test := range tests {
		player := New("alex")
		player.XP = test.xp
		if got := player.Rank(); got != test.rank {
			t.Errorf("Rank() at %d XP = %q, want %q", test.xp, got, test.rank)
		}
		next, needed, hasNext := player.NextRank()
		if next != test.next || needed != test.needed || hasNext != test.hasNext {
			t.Errorf("NextRank() at %d XP = %q, %d, %v; want %q, %d, %v", test.xp, next, needed, hasNext, test.next, test.needed, test.hasNext)
		}
	}
}
