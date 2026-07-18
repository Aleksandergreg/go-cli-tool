package mission

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEmbeddedCatalog(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	items := catalog.All()
	if len(items) != 20 {
		t.Fatalf("len(All()) = %d, want 20", len(items))
	}
	for index, item := range items {
		if item.Number != index+1 {
			t.Errorf("mission at index %d has number %d", index, item.Number)
		}
	}

	byID, found := catalog.Find("linux-find-logs")
	if !found || byID.Number != 4 {
		t.Fatalf("Find(id) = %#v, %v", byID, found)
	}
	byNumber, found := catalog.Find("04")
	if !found || byNumber.ID != byID.ID {
		t.Fatalf("Find(04) = %#v, %v", byNumber, found)
	}
}

func TestLegacyMissionDefaultsAndCatalogTrackFiltering(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	legacy, found := catalog.Find("linux-orientation")
	if !found {
		t.Fatal("legacy mission missing")
	}
	if legacy.EffectiveTrack() != TrackLinux || legacy.EffectiveEnvironment() != EnvironmentSimulated {
		t.Fatalf("legacy defaults = track %q, environment %q", legacy.EffectiveTrack(), legacy.EffectiveEnvironment())
	}

	linux := catalog.InTrack("")
	docker := catalog.InTrack(TrackDocker)
	if len(linux) != 19 || len(docker) != 1 {
		t.Fatalf("track sizes = linux %d, docker %d", len(linux), len(docker))
	}
	if docker[0].ID != "docker-container-census" || docker[0].Number != 17 {
		t.Fatalf("docker track = %#v", docker)
	}

	next, found := catalog.NextInTrack(TrackDocker, func(string) bool { return false })
	if !found || next.ID != "docker-container-census" {
		t.Fatalf("NextInTrack(docker) = %#v, %v", next, found)
	}
	if _, found := catalog.NextInTrack(TrackDocker, func(string) bool { return true }); found {
		t.Fatal("completed Docker track returned another mission")
	}
	completedFirstCampaigns := make(map[string]bool)
	for _, item := range linux {
		if item.Number <= 16 {
			completedFirstCampaigns[item.ID] = true
		}
	}
	nextLinux, found := catalog.NextInTrack(TrackLinux, func(id string) bool { return completedFirstCampaigns[id] })
	if !found || nextLinux.ID != "linux-vi-first-aid" || nextLinux.Number != 18 {
		t.Fatalf("NextInTrack(linux) after Mission 16 = %#v, %v", nextLinux, found)
	}
}

func TestAutomationShiftCurriculum(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	wants := []struct {
		id         string
		number     int
		difficulty string
		hints      int
	}{
		{id: "linux-vi-first-aid", number: 18, difficulty: "beginner", hints: 3},
		{id: "linux-report-on-repeat", number: 19, difficulty: "intermediate", hints: 4},
		{id: "linux-scope-creep", number: 20, difficulty: "advanced", hints: 5},
	}
	for _, want := range wants {
		item, found := catalog.Find(want.id)
		if !found {
			t.Errorf("mission %q missing", want.id)
			continue
		}
		if item.Number != want.number || item.Campaign != "The Automation Shift" || item.Difficulty != want.difficulty || len(item.Hints) != want.hints {
			t.Errorf("mission %q curriculum metadata = number %d, campaign %q, difficulty %q, hints %d", item.ID, item.Number, item.Campaign, item.Difficulty, len(item.Hints))
		}
	}
}

func TestRebalancedLinuxCurriculumMetadata(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}

	wants := []struct {
		id         string
		campaign   string
		difficulty string
		hints      int
	}{
		{id: "linux-find-logs", campaign: "First Day", difficulty: "beginner", hints: 3},
		{id: "linux-release-shuffle", campaign: "First Day", difficulty: "beginner", hints: 3},
		{id: "linux-permissions", campaign: "The Logpocalypse", difficulty: "beginner", hints: 3},
		{id: "linux-pipeline-report", campaign: "The Logpocalypse", difficulty: "advanced", hints: 5},
		{id: "linux-production-friday", campaign: "Production Friday", difficulty: "advanced", hints: 5},
	}
	for _, want := range wants {
		item, found := catalog.Find(want.id)
		if !found {
			t.Errorf("mission %q missing", want.id)
			continue
		}
		if item.Campaign != want.campaign || item.Difficulty != want.difficulty || len(item.Hints) != want.hints {
			t.Errorf("mission %q curriculum metadata = campaign %q, difficulty %q, hints %d; want %q, %q, %d",
				item.ID, item.Campaign, item.Difficulty, len(item.Hints), want.campaign, want.difficulty, want.hints)
		}
	}

	items := catalog.InTrack(TrackLinux)
	for _, item := range items {
		switch {
		case item.Number >= 1 && item.Number <= 5 && item.Campaign != "First Day":
			t.Errorf("World 1 mission %d campaign = %q, want First Day", item.Number, item.Campaign)
		case item.Number >= 6 && item.Number <= 10 && item.Campaign != "The Logpocalypse":
			t.Errorf("World 2 mission %d campaign = %q, want The Logpocalypse", item.Number, item.Campaign)
		}
	}
}

func TestContainerCensusUsesTypedDockerSetup(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, found := catalog.Find("docker-container-census")
	if !found {
		t.Fatal("Container Census mission missing")
	}
	if item.EffectiveTrack() != TrackDocker || item.EffectiveEnvironment() != EnvironmentDocker || item.Docker == nil {
		t.Fatalf("docker mission discriminators/setup = %#v", item)
	}
	if len(item.Docker.Images) != 1 || item.Docker.Images[0].Alias != "fixture" || !strings.Contains(item.Docker.Images[0].Reference, "@sha256:") {
		t.Fatalf("docker images = %#v", item.Docker.Images)
	}
	if len(item.Docker.Containers) != 2 || item.Docker.Containers[0].Name != "api" || item.Docker.Containers[0].State != "stopped" || item.Docker.Containers[1].State != "running" {
		t.Fatalf("docker containers = %#v", item.Docker.Containers)
	}
	if len(item.Validation.All) != 3 || item.Validation.All[2].Count == nil || *item.Validation.All[2].Count != 2 {
		t.Fatalf("docker validation = %#v", item.Validation.All)
	}
}

func TestMissionValidationRejectsInvalidDockerDefinitions(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Mission)
		wantErr string
	}{
		{
			name: "missing setup",
			mutate: func(item *Mission) {
				item.Docker = nil
			},
			wantErr: "requires docker setup",
		},
		{
			name: "mixed simulated setup",
			mutate: func(item *Mission) {
				item.Setup.Files = []FileSpec{{Path: "/host-shaped", Content: "no"}}
			},
			wantErr: "cannot define simulated setup",
		},
		{
			name: "duplicate image alias",
			mutate: func(item *Mission) {
				item.Docker.Images = append(item.Docker.Images, item.Docker.Images[0])
			},
			wantErr: "duplicate docker image alias",
		},
		{
			name: "unpinned image",
			mutate: func(item *Mission) {
				item.Docker.Images[0].Reference = "docker.io/library/busybox:latest"
			},
			wantErr: "pinned by sha256 digest",
		},
		{
			name: "duplicate container",
			mutate: func(item *Mission) {
				item.Docker.Containers = append(item.Docker.Containers, item.Docker.Containers[0])
			},
			wantErr: "duplicate docker container name",
		},
		{
			name: "unknown image alias",
			mutate: func(item *Mission) {
				item.Docker.Containers[0].Image = "missing"
			},
			wantErr: "references unknown image alias",
		},
		{
			name: "unknown state",
			mutate: func(item *Mission) {
				item.Docker.Containers[0].State = "paused"
			},
			wantErr: "unknown state",
		},
		{
			name: "unknown validation container",
			mutate: func(item *Mission) {
				item.Validation.All[0].Container = "missing"
			},
			wantErr: "unknown docker container",
		},
		{
			name: "missing count",
			mutate: func(item *Mission) {
				item.Validation.All[2].Count = nil
			},
			wantErr: "count must be a non-negative integer",
		},
		{
			name: "negative count",
			mutate: func(item *Mission) {
				count := -1
				item.Validation.All[2].Count = &count
			},
			wantErr: "count must be a non-negative integer",
		},
		{
			name: "running condition with unrelated value",
			mutate: func(item *Mission) {
				item.Validation.All[0].Value = "api"
			},
			wantErr: "does not support value",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			catalog, err := LoadCatalog()
			if err != nil {
				t.Fatal(err)
			}
			item, _ := catalog.Find("docker-container-census")
			test.mutate(&item)
			err = validateMission(item)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validateMission() error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestMissionValidationRejectsDockerConditionOnSimulatedMission(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, _ := catalog.Find("linux-orientation")
	item.Validation.All = []Condition{{Type: "docker_container_running", Container: "api"}}
	if err := validateMission(item); err == nil || !strings.Contains(err.Error(), "requires a docker environment") {
		t.Fatalf("validateMission() error = %v", err)
	}
}

func TestDockerSetupResourceLimits(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, _ := catalog.Find("docker-container-census")

	tooManyImages := *item.Docker
	tooManyImages.Images = make([]DockerImageSpec, maxDockerImagesPerMission+1)
	if err := ValidateDockerSetup(tooManyImages); err == nil || !strings.Contains(err.Error(), "image limit") {
		t.Fatalf("ValidateDockerSetup(images) error = %v", err)
	}

	tooManyContainers := *item.Docker
	tooManyContainers.Containers = make([]DockerContainerSpec, maxDockerContainersPerMission+1)
	if err := ValidateDockerSetup(tooManyContainers); err == nil || !strings.Contains(err.Error(), "container limit") {
		t.Fatalf("ValidateDockerSetup(containers) error = %v", err)
	}
}

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

func TestMissionValidationRejectsFieldsOutsideConditionContract(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, _ := catalog.Find("linux-orientation")
	item.Validation.All[0].Path = "/not-used"
	if err := validateMission(item); err == nil || !strings.Contains(err.Error(), `type "output_equals" does not support path`) {
		t.Fatalf("validateMission() error = %v", err)
	}
}

func TestConditionValidationRejectsExplicitZeroValueFieldsOutsideContract(t *testing.T) {
	tests := []struct {
		data    string
		wantErr string
	}{
		{data: `{"type":"output_equals","pid":0}`, wantErr: `does not support pid`},
		{data: `{"type":"file_exists","value":""}`, wantErr: `does not support value`},
		{data: `{"type":"path_missing","values":[]}`, wantErr: `does not support values`},
	}
	for _, test := range tests {
		var condition Condition
		if err := json.Unmarshal([]byte(test.data), &condition); err != nil {
			t.Fatalf("Unmarshal(%s) error = %v", test.data, err)
		}
		if err := validateCondition(condition, EnvironmentSimulated); err == nil || !strings.Contains(err.Error(), test.wantErr) {
			t.Errorf("validateCondition(%s) error = %v, want substring %q", test.data, err, test.wantErr)
		}
	}
}

func TestConditionDecoderPreservesUnknownFieldRejection(t *testing.T) {
	var condition Condition
	err := json.Unmarshal([]byte(`{"type":"output_equals","surprise":0}`), &condition)
	if err == nil || !strings.Contains(err.Error(), `unknown field "surprise"`) {
		t.Fatalf("Unmarshal() error = %v", err)
	}
}

func TestCatalogResultsCannotMutateCatalogState(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}

	archive, _ := catalog.Find("9")
	archive.SuggestedCommands[0] = "changed"
	archive.Hints[0] = "changed"
	archive.Setup.Archives[0].Entries[0].Content = "changed"
	archiveAgain, _ := catalog.Find("9")
	if archiveAgain.SuggestedCommands[0] == "changed" || archiveAgain.Hints[0] == "changed" || archiveAgain.Setup.Archives[0].Entries[0].Content == "changed" {
		t.Fatal("Find returned catalog-owned teaching or archive storage")
	}

	lines, _ := catalog.Find("12")
	lines.Validation.All[0].Values[0] = "changed"
	linesAgain, _ := catalog.Find("12")
	if linesAgain.Validation.All[0].Values[0] == "changed" {
		t.Fatal("Find returned catalog-owned validation values")
	}

	environment, _ := catalog.Find("7")
	environment.Setup.Environment["DEPLOY_ENV"] = "changed"
	environmentAgain, _ := catalog.Find("7")
	if environmentAgain.Setup.Environment["DEPLOY_ENV"] == "changed" {
		t.Fatal("Find returned catalog-owned environment storage")
	}

	docker, _ := catalog.Find("17")
	docker.Docker.Images[0].Alias = "changed"
	*docker.Validation.All[2].Count = 99
	dockerAgain, _ := catalog.Find("17")
	if dockerAgain.Docker.Images[0].Alias == "changed" || *dockerAgain.Validation.All[2].Count == 99 {
		t.Fatal("Find returned catalog-owned Docker or count storage")
	}

	items := catalog.All()
	items[0].Setup.Files[0].Content = "changed"
	firstAgain, _ := catalog.Find("1")
	if firstAgain.Setup.Files[0].Content == "changed" {
		t.Fatal("All returned catalog-owned setup storage")
	}
}

func TestCatalogTrackBoundariesAndAdjacency(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	first, found := catalog.FirstInTrack(TrackLinux)
	if !found || first.Number != 1 {
		t.Fatalf("FirstInTrack(linux) = %#v, %v", first, found)
	}
	last, found := catalog.LastInTrack(TrackLinux)
	if !found || last.Number != 20 {
		t.Fatalf("LastInTrack(linux) = %#v, %v", last, found)
	}
	next, found := catalog.AdjacentInTrack("linux-production-friday", 1)
	if !found || next.ID != "linux-vi-first-aid" {
		t.Fatalf("AdjacentInTrack(after 16) = %#v, %v", next, found)
	}
	previous, found := catalog.AdjacentInTrack("linux-vi-first-aid", -1)
	if !found || previous.ID != "linux-production-friday" {
		t.Fatalf("AdjacentInTrack(before 18) = %#v, %v", previous, found)
	}
	if _, found := catalog.AdjacentInTrack("docker-container-census", 1); found {
		t.Fatal("single-mission Docker track has a next mission")
	}
	if _, found := catalog.AdjacentInTrack("missing", 1); found {
		t.Fatal("unknown mission has an adjacent mission")
	}
}

func TestNextReturnsFirstIncompleteMission(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, found := catalog.Next(func(id string) bool {
		return id == "linux-orientation" || id == "linux-config-crawl"
	})
	if !found || item.Number != 3 {
		t.Fatalf("Next() = %#v, %v; want mission 3", item, found)
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
	item, found := catalog.Find("9")
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
