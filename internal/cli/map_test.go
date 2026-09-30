package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/profile"
)

func TestMapShowsTrackLocalWorldsAndStages(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "", store)
	if err := app.Run([]string{"map"}); err != nil {
		t.Fatalf("map error = %v; stderr = %s", err, errOut.String())
	}
	for _, expected := range []string{
		"WORLD 1/4 · First Day",
		"Stage 6/6 · #06",
		"WORLD 2/4 · The Logpocalypse",
		"WORLD 4/4 · The Automation Shift",
		"Jump: opsquest play --world N",
	} {
		if !strings.Contains(out.String(), expected) {
			t.Errorf("map output missing %q:\n%s", expected, out.String())
		}
	}
	if strings.Contains(out.String(), "Container Census") {
		t.Fatalf("default Linux map included the optional Docker track:\n%s", out.String())
	}
}

func TestListAndProfileWorkWithoutExistingSave(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "newbie")
	app, out, _ := testApp(t, "", store)
	if err := app.Run([]string{"list"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "0/23 missions complete") {
		t.Fatalf("list output = %s", out.String())
	}

	app, out, _ = testApp(t, "", store)
	if err := app.Run([]string{"profile"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Operator: newbie") || !strings.Contains(out.String(), "Rank: Intern") {
		t.Fatalf("profile output = %s", out.String())
	}
}

func TestListFiltersMissions(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	play, _, _ := testApp(t, "pwd\n", store)
	if err := play.Run([]string{"play", "1"}); err != nil {
		t.Fatal(err)
	}
	app, out, _ := testApp(t, "", store)
	if err := app.Run([]string{"list", "--completed"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Where Am I?") || strings.Contains(out.String(), "Configuration Crawl") {
		t.Fatalf("filtered list output = %s", out.String())
	}
}

func TestListCampaignFilterUsesCampaignTotal(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "", store)
	if err := app.Run([]string{"list", "--campaign", "First Day"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "0/6 missions complete") {
		t.Fatalf("campaign-filtered total missing:\n%s", out.String())
	}
	if strings.Contains(out.String(), "0/39 missions complete") || strings.Contains(out.String(), "Production Friday") {
		t.Fatalf("campaign filter used catalog-wide scope:\n%s", out.String())
	}
}
