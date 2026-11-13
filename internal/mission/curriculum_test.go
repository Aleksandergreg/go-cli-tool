package mission

import (
	"strings"
	"testing"
)

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
		{id: "linux-runbook-runner", number: 26, difficulty: "beginner", hints: 3},
		{id: "linux-vi-first-aid", number: 27, difficulty: "beginner", hints: 3},
		{id: "linux-report-on-repeat", number: 28, difficulty: "intermediate", hints: 4},
		{id: "linux-scope-creep", number: 29, difficulty: "advanced", hints: 5},
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
		case item.Number >= 1 && item.Number <= 6 && item.Campaign != "First Day":
			t.Errorf("World 1 mission %d campaign = %q, want First Day", item.Number, item.Campaign)
		case item.Number >= 7 && item.Number <= 12 && item.Campaign != "The Logpocalypse":
			t.Errorf("World 2 mission %d campaign = %q, want The Logpocalypse", item.Number, item.Campaign)
		}
	}
}

func TestBeginnerLinuxExpansionMetadata(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	wants := []struct {
		id       string
		number   int
		campaign string
	}{
		{id: "linux-read-handoff", number: 3, campaign: "First Day"},
		{id: "linux-log-preview", number: 9, campaign: "The Logpocalypse"},
		{id: "linux-error-headcount", number: 14, campaign: "Production Friday"},
		{id: "linux-runbook-runner", number: 26, campaign: "The Automation Shift"},
	}
	for _, want := range wants {
		item, found := catalog.Find(want.id)
		if !found {
			t.Errorf("mission %q missing", want.id)
			continue
		}
		if item.Number != want.number || item.Campaign != want.campaign || item.Difficulty != DifficultyBeginner || len(item.Hints) != 3 {
			t.Errorf("mission %q metadata = number %d, campaign %q, difficulty %q, hints %d", item.ID, item.Number, item.Campaign, item.Difficulty, len(item.Hints))
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

func TestExpandedDockerCurriculumMetadata(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	wants := []string{
		"docker-container-census",
		"docker-last-broadcast",
		"docker-exit-code-detective",
		"docker-quiet-worker",
		"docker-recovery-pair",
		"docker-shift-handoff",
	}
	for index, id := range wants {
		item, found := catalog.Find(id)
		if !found {
			t.Errorf("Docker mission %q missing", id)
			continue
		}
		if item.Number != 20+index || item.Campaign != "It Works on My Machine" || item.Difficulty != DifficultyBeginner {
			t.Errorf("Docker mission %q metadata = number %d, campaign %q, difficulty %q", id, item.Number, item.Campaign, item.Difficulty)
		}
	}
	diagnostic, _ := catalog.Find("docker-last-broadcast")
	if diagnostic.Docker == nil || len(diagnostic.Docker.Containers) != 1 || diagnostic.Docker.Containers[0].Log == "" || diagnostic.Docker.Containers[0].ExitCode == nil || *diagnostic.Docker.Containers[0].ExitCode != 78 {
		t.Fatalf("diagnostic Docker setup = %#v", diagnostic.Docker)
	}
}

func TestContainerTriageCurriculum(t *testing.T) {
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
		{id: "docker-janitor-duty", number: 30, difficulty: DifficultyBeginner, hints: 3},
		{id: "docker-tail-end", number: 31, difficulty: DifficultyBeginner, hints: 3},
		{id: "docker-running-isnt-healthy", number: 32, difficulty: DifficultyIntermediate, hints: 3},
		{id: "docker-crash-loop", number: 33, difficulty: DifficultyIntermediate, hints: 3},
		{id: "docker-postmortem-triage", number: 34, difficulty: DifficultyAdvanced, hints: 5},
	}
	for _, want := range wants {
		item, found := catalog.Find(want.id)
		if !found {
			t.Errorf("mission %q missing", want.id)
			continue
		}
		if item.Number != want.number || item.Campaign != "Container Triage" || item.EffectiveTrack() != TrackDocker || item.Difficulty != want.difficulty || len(item.Hints) != want.hints {
			t.Errorf("mission %q curriculum metadata = number %d, campaign %q, track %q, difficulty %q, hints %d", item.ID, item.Number, item.Campaign, item.EffectiveTrack(), item.Difficulty, len(item.Hints))
		}
	}
	worlds := catalog.Worlds(TrackDocker)
	if len(worlds) < 2 || worlds[1].Name != "Container Triage" || len(worlds[1].Missions) != 5 {
		t.Fatalf("Docker worlds = %d, want Container Triage as World 2", len(worlds))
	}
	crashLoop, _ := catalog.Find("docker-crash-loop")
	payments := crashLoop.Docker.Containers[0]
	if payments.Restart != DockerRestartOnFailure || payments.State != DockerStateRunning || payments.ExitCode == nil || *payments.ExitCode == 0 {
		t.Fatalf("crash-loop fixture = %#v", payments)
	}
}

func TestNetworkPlumbingCurriculum(t *testing.T) {
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
		{id: "docker-network-census", number: 35, difficulty: DifficultyBeginner, hints: 3},
		{id: "docker-cant-reach-the-db", number: 36, difficulty: DifficultyBeginner, hints: 3},
		{id: "docker-need-to-know", number: 37, difficulty: DifficultyIntermediate, hints: 3},
		{id: "docker-private-channel", number: 38, difficulty: DifficultyIntermediate, hints: 3},
		{id: "docker-segmentation", number: 39, difficulty: DifficultyAdvanced, hints: 5},
	}
	for _, want := range wants {
		item, found := catalog.Find(want.id)
		if !found {
			t.Errorf("mission %q missing", want.id)
			continue
		}
		if item.Number != want.number || item.Campaign != "Network Plumbing" || item.EffectiveTrack() != TrackDocker || item.Difficulty != want.difficulty || len(item.Hints) != want.hints {
			t.Errorf("mission %q curriculum metadata = number %d, campaign %q, track %q, difficulty %q, hints %d", item.ID, item.Number, item.Campaign, item.EffectiveTrack(), item.Difficulty, len(item.Hints))
		}
		if len(item.Docker.Networks) == 0 {
			t.Errorf("network mission %q declares no networks", item.ID)
		}
	}
	worlds := catalog.Worlds(TrackDocker)
	if len(worlds) != 3 || worlds[2].Name != "Network Plumbing" || len(worlds[2].Missions) != 5 {
		t.Fatalf("Docker worlds = %d, want Network Plumbing as World 3", len(worlds))
	}
}
