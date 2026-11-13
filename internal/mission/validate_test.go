package mission

import (
	"strings"
	"testing"
)

func TestMissionValidationRejectsUnknownTrackAndEnvironment(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	base, _ := catalog.Find("linux-orientation")
	tests := []struct {
		name    string
		mutate  func(*Mission)
		wantErr string
	}{
		{
			name:    "track",
			mutate:  func(item *Mission) { item.Track = "cloud" },
			wantErr: `unknown track "cloud"`,
		},
		{
			name:    "environment",
			mutate:  func(item *Mission) { item.Environment = "host" },
			wantErr: `unknown environment "host"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item := base
			test.mutate(&item)
			if err := validateMission(item); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validateMission() error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestMissionValidationRejectsUnknownDifficultyAndInvalidHintCount(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	base, _ := catalog.Find("linux-orientation")
	tests := []struct {
		name    string
		mutate  func(*Mission)
		wantErr string
	}{
		{
			name:    "unknown difficulty",
			mutate:  func(item *Mission) { item.Difficulty = "expert-ish" },
			wantErr: "unknown difficulty",
		},
		{
			name:    "no hints",
			mutate:  func(item *Mission) { item.Hints = nil },
			wantErr: "between 1 and 5 hints",
		},
		{
			name:    "too many hints",
			mutate:  func(item *Mission) { item.Hints = make([]string, 6) },
			wantErr: "between 1 and 5 hints",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item := base
			test.mutate(&item)
			if err := validateMission(item); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validateMission() error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestMissionValidationRejectsInvalidSuggestedCommands(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	base, _ := catalog.Find("linux-orientation")
	tests := []struct {
		name     string
		commands []string
		wantErr  string
	}{
		{name: "missing", commands: nil, wantErr: "at least one suggested command"},
		{name: "blank", commands: []string{""}, wantErr: "lowercase command name"},
		{name: "syntax instead of command name", commands: []string{"grep ERROR"}, wantErr: "lowercase command name"},
		{name: "duplicate", commands: []string{"pwd", "pwd"}, wantErr: "duplicate suggested command"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item := base
			item.SuggestedCommands = test.commands
			if err := validateMission(item); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validateMission() error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestMissionDecoderRejectsUnknownFields(t *testing.T) {
	_, err := decodeMission([]byte(`{"id":"test","surprise":true}`))
	if err == nil {
		t.Fatal("unknown mission field was accepted")
	}
}

func TestMissionValidationRejectsUnsafeArchiveEntries(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, found := catalog.Find("linux-archive-rescue")
	if !found {
		t.Fatal("archive mission missing")
	}
	item.Setup.Archives[0].Entries[0].Path = "../../outside"
	if err := validateMission(item); err == nil {
		t.Fatal("unsafe archive entry was accepted")
	}
}

func TestMissionValidationRejectsConflictingSetupPaths(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, _ := catalog.Find("1")
	item.Setup.Files = append(item.Setup.Files, FileSpec{Path: "/home/operator", Content: "conflict"})
	if err := validateMission(item); err == nil {
		t.Fatal("directory/file path conflict was accepted")
	}
}
