package mission

import (
	"reflect"
	"strings"
	"testing"
)

func TestCatalogWorldsAreOrderedWithinEachTrack(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}

	linux := catalog.Worlds("")
	wantNames := []string{"First Day", "The Logpocalypse", "Production Friday", "The Automation Shift"}
	wantStages := []int{5, 5, 6, 3}
	if len(linux) != len(wantNames) {
		t.Fatalf("len(Worlds(linux)) = %d, want %d", len(linux), len(wantNames))
	}
	for index, world := range linux {
		if world.Number != index+1 || world.Track != TrackLinux || world.Name != wantNames[index] || len(world.Missions) != wantStages[index] {
			t.Errorf("Worlds(linux)[%d] = number %d, track %q, name %q, stages %d", index, world.Number, world.Track, world.Name, len(world.Missions))
		}
	}
	if linux[1].Missions[0].ID != "linux-permissions" || linux[2].Missions[0].ID != "linux-tail-trouble" || linux[3].Missions[0].ID != "linux-vi-first-aid" {
		t.Fatalf("Linux world boundaries = %q, %q, then %q", linux[1].Missions[0].ID, linux[2].Missions[0].ID, linux[3].Missions[0].ID)
	}

	docker := catalog.Worlds(TrackDocker)
	if len(docker) != 1 || docker[0].Number != 1 || docker[0].Name != "It Works on My Machine" || len(docker[0].Missions) != 1 || docker[0].Missions[0].ID != "docker-container-census" {
		t.Fatalf("Worlds(docker) = %#v", docker)
	}
	if worlds := catalog.Worlds("missing"); worlds != nil {
		t.Fatalf("Worlds(missing) = %#v, want nil", worlds)
	}
}

func TestCatalogWorldLookupAndPlacement(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}

	world, found := catalog.World(TrackLinux, 2)
	if !found || world.Name != "The Logpocalypse" || len(world.Missions) != 5 {
		t.Fatalf("World(linux, 2) = %#v, %v", world, found)
	}
	defaultWorld, found := catalog.World("", 1)
	if !found || defaultWorld.Name != "First Day" {
		t.Fatalf("World(default, 1) = %#v, %v", defaultWorld, found)
	}
	for _, test := range []struct {
		track  string
		number int
	}{
		{track: TrackLinux, number: 0},
		{track: TrackLinux, number: 5},
		{track: "missing", number: 1},
	} {
		if _, found := catalog.World(test.track, test.number); found {
			t.Errorf("World(%q, %d) unexpectedly found", test.track, test.number)
		}
	}

	tests := []struct {
		missionID string
		want      Placement
	}{
		{
			missionID: "linux-orientation",
			want:      Placement{Track: TrackLinux, WorldNumber: 1, WorldTotal: 4, WorldName: "First Day", StageNumber: 1, StageTotal: 5},
		},
		{
			missionID: "linux-pipeline-report",
			want:      Placement{Track: TrackLinux, WorldNumber: 2, WorldTotal: 4, WorldName: "The Logpocalypse", StageNumber: 5, StageTotal: 5},
		},
		{
			missionID: "linux-vi-first-aid",
			want:      Placement{Track: TrackLinux, WorldNumber: 4, WorldTotal: 4, WorldName: "The Automation Shift", StageNumber: 1, StageTotal: 3},
		},
		{
			missionID: "docker-container-census",
			want:      Placement{Track: TrackDocker, WorldNumber: 1, WorldTotal: 1, WorldName: "It Works on My Machine", StageNumber: 1, StageTotal: 1},
		},
	}
	for _, test := range tests {
		got, found := catalog.Placement(test.missionID)
		if !found || got != test.want {
			t.Errorf("Placement(%q) = %#v, %v; want %#v", test.missionID, got, found, test.want)
		}
	}
	if _, found := catalog.Placement("missing"); found {
		t.Fatal("Placement(missing) unexpectedly found")
	}
}

func TestCatalogNextInWorldUsesCompletionAndReturnsClones(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}

	completed := map[string]bool{
		"linux-permissions": true,
		"linux-environment": true,
	}
	next, found := catalog.NextInWorld("", 2, func(id string) bool { return completed[id] })
	if !found || next.ID != "linux-runaway" {
		t.Fatalf("NextInWorld(linux, 2) = %#v, %v", next, found)
	}

	first, found := catalog.NextInWorld(TrackLinux, 1, nil)
	if !found || first.ID != "linux-orientation" {
		t.Fatalf("NextInWorld(linux, 1, nil) = %#v, %v", first, found)
	}
	first.Setup.Files[0].Content = "mutated"
	again, _ := catalog.NextInWorld(TrackLinux, 1, nil)
	if again.Setup.Files[0].Content == "mutated" {
		t.Fatal("NextInWorld returned catalog-owned mission state")
	}

	if _, found := catalog.NextInWorld(TrackLinux, 2, func(string) bool { return true }); found {
		t.Fatal("completed world returned another mission")
	}
	if _, found := catalog.NextInWorld(TrackLinux, 0, nil); found {
		t.Fatal("invalid world number returned a mission")
	}
}

func TestCatalogWorldResultsAreDeepCopies(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}

	worlds := catalog.Worlds(TrackLinux)
	worlds[0].Missions[0].SuggestedCommands[0] = "changed"
	worlds[0].Missions[0].Setup.Files[0].Content = "changed"
	worlds[0].Missions[0].Validation.All[0].Value = "changed"

	fresh, _ := catalog.World(TrackLinux, 1)
	first := fresh.Missions[0]
	if first.SuggestedCommands[0] == "changed" || first.Setup.Files[0].Content == "changed" || first.Validation.All[0].Value == "changed" {
		t.Fatal("world lookup returned catalog-owned mission state")
	}
}

func TestWorldContiguityIsValidatedPerTrack(t *testing.T) {
	missionItem := func(id, track, campaign string) Mission {
		return Mission{ID: id, Track: track, Campaign: campaign}
	}

	valid := []Mission{
		missionItem("linux-a1", TrackLinux, "A"),
		missionItem("docker-a", TrackDocker, "Docker"),
		missionItem("linux-a2", TrackLinux, "A"),
		missionItem("linux-b", TrackLinux, "B"),
		missionItem("docker-b", TrackDocker, "Docker"),
	}
	before := append([]Mission(nil), valid...)
	if err := validateWorldContiguity(valid); err != nil {
		t.Fatalf("interleaved tracks rejected: %v", err)
	}
	if !reflect.DeepEqual(valid, before) {
		t.Fatal("contiguity validation mutated mission order")
	}

	invalid := []Mission{
		missionItem("linux-a1", TrackLinux, "A"),
		missionItem("linux-b", TrackLinux, "B"),
		missionItem("linux-a2", TrackLinux, "A"),
	}
	err := validateWorldContiguity(invalid)
	if err == nil || !strings.Contains(err.Error(), `campaign "A" must be contiguous within track "linux"`) {
		t.Fatalf("validateWorldContiguity() error = %v", err)
	}

	// Different tracks may intentionally reuse a display name without merging
	// their independent curricula.
	sharedName := []Mission{
		missionItem("linux-shared", TrackLinux, "Shared"),
		missionItem("docker-shared", TrackDocker, "Shared"),
	}
	if err := validateWorldContiguity(sharedName); err != nil {
		t.Fatalf("same campaign name across tracks rejected: %v", err)
	}
}
