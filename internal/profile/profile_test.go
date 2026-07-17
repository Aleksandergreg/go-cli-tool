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
	if loaded.XP != 40 || loaded.Commands["pwd"] != 2 || loaded.HintsUsed() != 1 {
		t.Fatalf("loaded profile = %#v", loaded)
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
