package mission

import (
	"testing"
)

func TestEmbeddedCatalog(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	items := catalog.All()
	if len(items) != 39 {
		t.Fatalf("len(All()) = %d, want 39", len(items))
	}
	for index, item := range items {
		if item.Number != index+1 {
			t.Errorf("mission at index %d has number %d", index, item.Number)
		}
	}

	byID, found := catalog.Find("linux-find-logs")
	if !found || byID.Number != 5 {
		t.Fatalf("Find(id) = %#v, %v", byID, found)
	}
	byNumber, found := catalog.Find("05")
	if !found || byNumber.ID != byID.ID {
		t.Fatalf("Find(05) = %#v, %v", byNumber, found)
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
	if len(linux) != 23 || len(docker) != 16 {
		t.Fatalf("track sizes = linux %d, docker %d", len(linux), len(docker))
	}
	if docker[0].ID != "docker-container-census" || docker[0].Number != 20 {
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
		if item.Number <= 19 {
			completedFirstCampaigns[item.ID] = true
		}
	}
	nextLinux, found := catalog.NextInTrack(TrackLinux, func(id string) bool { return completedFirstCampaigns[id] })
	if !found || nextLinux.ID != "linux-runbook-runner" || nextLinux.Number != 26 {
		t.Fatalf("NextInTrack(linux) after Mission 19 = %#v, %v", nextLinux, found)
	}
}

func TestCatalogResultsCannotMutateCatalogState(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}

	archive, _ := catalog.Find("linux-archive-rescue")
	archive.SuggestedCommands[0] = "changed"
	archive.Hints[0] = "changed"
	archive.Setup.Archives[0].Entries[0].Content = "changed"
	archiveAgain, _ := catalog.Find("linux-archive-rescue")
	if archiveAgain.SuggestedCommands[0] == "changed" || archiveAgain.Hints[0] == "changed" || archiveAgain.Setup.Archives[0].Entries[0].Content == "changed" {
		t.Fatal("Find returned catalog-owned teaching or archive storage")
	}

	lines, _ := catalog.Find("linux-alert-counts")
	lines.Validation.All[0].Values[0] = "changed"
	linesAgain, _ := catalog.Find("linux-alert-counts")
	if linesAgain.Validation.All[0].Values[0] == "changed" {
		t.Fatal("Find returned catalog-owned validation values")
	}

	environment, _ := catalog.Find("linux-environment")
	environment.Setup.Environment["DEPLOY_ENV"] = "changed"
	environmentAgain, _ := catalog.Find("linux-environment")
	if environmentAgain.Setup.Environment["DEPLOY_ENV"] == "changed" {
		t.Fatal("Find returned catalog-owned environment storage")
	}

	docker, _ := catalog.Find("docker-container-census")
	docker.Docker.Images[0].Alias = "changed"
	*docker.Validation.All[2].Count = 99
	dockerAgain, _ := catalog.Find("docker-container-census")
	if dockerAgain.Docker.Images[0].Alias == "changed" || *dockerAgain.Validation.All[2].Count == 99 {
		t.Fatal("Find returned catalog-owned Docker or count storage")
	}

	diagnostic, _ := catalog.Find("docker-last-broadcast")
	*diagnostic.Docker.Containers[0].ExitCode = 1
	diagnosticAgain, _ := catalog.Find("docker-last-broadcast")
	if *diagnosticAgain.Docker.Containers[0].ExitCode != 78 {
		t.Fatal("Find returned catalog-owned Docker exit-code storage")
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
	if !found || last.Number != 29 {
		t.Fatalf("LastInTrack(linux) = %#v, %v", last, found)
	}
	lastDocker, found := catalog.LastInTrack(TrackDocker)
	if !found || lastDocker.ID != "docker-segmentation" || lastDocker.Number != 39 {
		t.Fatalf("LastInTrack(docker) = %#v, %v", lastDocker, found)
	}
	crossWorld, found := catalog.AdjacentInTrack("docker-shift-handoff", 1)
	if !found || crossWorld.ID != "docker-janitor-duty" {
		t.Fatalf("AdjacentInTrack(after Docker 25) = %#v, %v", crossWorld, found)
	}
	next, found := catalog.AdjacentInTrack("linux-production-friday", 1)
	if !found || next.ID != "linux-runbook-runner" {
		t.Fatalf("AdjacentInTrack(after 19) = %#v, %v", next, found)
	}
	previous, found := catalog.AdjacentInTrack("linux-runbook-runner", -1)
	if !found || previous.ID != "linux-production-friday" {
		t.Fatalf("AdjacentInTrack(before 26) = %#v, %v", previous, found)
	}
	nextDocker, found := catalog.AdjacentInTrack("docker-container-census", 1)
	if !found || nextDocker.ID != "docker-last-broadcast" {
		t.Fatalf("AdjacentInTrack(after Docker mission 20) = %#v, %v", nextDocker, found)
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
