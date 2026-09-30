package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/profile"
)

func TestMissionPromptCanShowMapAndJumpWorlds(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "map\nworld 4\nquit\n", store)
	if err := app.Run([]string{"play", "1"}); err != nil {
		t.Fatalf("in-mission world navigation error = %v; stderr = %s", err, errOut.String())
	}
	for _, expected := range []string{
		"WORLD 4/4 · The Automation Shift",
		"Switching to Mission 26: Run the Runbook",
		"MISSION 26: Run the Runbook",
		"Navigate here: world N",
	} {
		if !strings.Contains(out.String(), expected) {
			t.Errorf("in-mission world navigation missing %q:\n%s", expected, out.String())
		}
	}
	if strings.Contains(errOut.String(), "command not available") {
		t.Fatalf("world navigation reached the sandbox: %s", errOut.String())
	}
}

func TestMissionPromptCanScopeTheCurrentStageToItsWorld(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	seedCompletedMissions(t, store, linuxMissionIDsThroughArchive[:5]...)

	app, out, errOut := testApp(t, "world 1\nmv incident-104.txt /archive/2026/incident-104.txt\n", store)
	if err := app.Run([]string{"play"}); err != nil {
		t.Fatalf("same-stage world selection error = %v; stderr = %s", err, errOut.String())
	}
	for _, expected := range []string{"World 1 route selected", "World 1 complete: First Day!"} {
		if !strings.Contains(out.String(), expected) {
			t.Fatalf("same-stage world selection missing %q:\n%s", expected, out.String())
		}
	}
	if strings.Contains(out.String(), "MISSION 07") {
		t.Fatalf("same-stage world route crossed its boundary:\n%s", out.String())
	}
}

func TestMissionPromptReportsInvalidWorldWithoutDispatchingIt(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, _, errOut := testApp(t, "world 0\nworld 99\nquit\n", store)
	if err := app.Run([]string{"play", "1"}); err != nil {
		t.Fatalf("play error = %v", err)
	}
	for _, expected := range []string{"world number must be a positive integer", "world 99 does not exist"} {
		if !strings.Contains(errOut.String(), expected) {
			t.Errorf("world navigation stderr missing %q: %s", expected, errOut.String())
		}
	}
	if strings.Contains(errOut.String(), "command not available") {
		t.Fatalf("invalid world navigation reached the sandbox: %s", errOut.String())
	}
}

func TestMissionPromptExplainsCompletedWorldReplay(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	seedCompletedMissions(t, store, linuxMissionIDsThroughArchive[:6]...)
	app, out, errOut := testApp(t, "world 1\npwd\nquit\n", store)
	if err := app.Run([]string{"play", "6"}); err != nil {
		t.Fatalf("play error = %v; stderr = %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "World 1 is complete; replaying Stage 1") || !strings.Contains(out.String(), "MISSION 01: Where Am I?") {
		t.Fatalf("completed-world replay was not explained:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Continuing to Mission 02: Configuration Crawl") || !strings.Contains(out.String(), "MISSION 02") {
		t.Fatalf("completed-world replay did not retain its world route:\n%s", out.String())
	}
}

func TestMissionPromptCanListAndSwitchMissions(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	seed, _, _ := testApp(t, "pwd\n", store)
	if err := seed.Run([]string{"play", "1"}); err != nil {
		t.Fatal(err)
	}

	input := strings.Join([]string{
		"opsquest list --completed",
		"opsquest play linux-workspace",
		"mkdir -p reports/daily",
		"list --completed",
		"status",
		"quit",
	}, "\n") + "\n"
	app, out, errOut := testApp(t, input, store)
	if err := app.Run([]string{"play", "9"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}

	output := out.String()
	for _, expected := range []string{
		"LINUX CAMPAIGN",
		"1/23 missions complete",
		"Switching to Mission 04: A Place for Everything",
		"MISSION 04: A Place for Everything",
		"✓ Directory exists: /workspace/reports/daily",
		"○ File exists: /workspace/reports/daily/summary.txt",
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("in-mission navigation output missing %q:\n%s", expected, output)
		}
	}
	if strings.Contains(errOut.String(), "command not available") {
		t.Fatalf("navigation reached the sandbox dispatcher: %s", errOut.String())
	}
}

func TestMissionPromptUsesWorldLocalStageNumbers(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "play 4\nquit\n", store)
	if err := app.Run([]string{"play", "7"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}

	output := out.String()
	for _, expected := range []string{
		"Navigate: map · world N · play STAGE/ID",
		"Switching to Mission 10: The Runaway Worker",
		"MISSION 10: The Runaway Worker",
		"World 2/4: The Logpocalypse · Stage 4/6",
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("world-local stage navigation missing %q:\n%s", expected, output)
		}
	}
	if strings.Contains(output, "MISSION 04: A Place for Everything") {
		t.Fatalf("World 2 stage navigation fell back to the global Mission 04:\n%s", output)
	}
	if errOut.Len() != 0 {
		t.Fatalf("world-local stage navigation stderr = %s", errOut.String())
	}
}

func TestMissionPromptStageJumpContinuesIntoNextWorld(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	seedCompletedMissions(t, store, linuxMissionIDsThroughArchive[:6]...)
	input := "play 6\nmv /incoming/incident-104.txt /archive/2026/incident-104.txt\nquit\n"
	app, out, errOut := testApp(t, input, store)
	if err := app.Run([]string{"play", "3"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}

	output := out.String()
	for _, expected := range []string{
		"Switching to Mission 06: The Release Shuffle",
		"Replay complete",
		"Continuing to Mission 07: Permission to Deploy",
		"World 2/4: The Logpocalypse",
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("stage jump did not continue with %q:\n%s", expected, output)
		}
	}
}

func TestMissionPromptHandlesCurrentAndOutOfRangeWorldStages(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "play 1\nplay 7\nquit\n", store)
	if err := app.Run([]string{"play", "7"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}

	if !strings.Contains(out.String(), "Already playing Mission 07: Permission to Deploy.") {
		t.Fatalf("same-stage navigation was not explained:\n%s", out.String())
	}
	if strings.Contains(out.String(), "Switching to Mission") {
		t.Fatalf("same or invalid stage unexpectedly switched missions:\n%s", out.String())
	}
	wantError := `stage "7" does not exist in World 2; choose a stage from 1 to 6 or use map`
	if !strings.Contains(errOut.String(), wantError) {
		t.Fatalf("out-of-range stage error missing %q: %s", wantError, errOut.String())
	}
}

func TestMissionPromptParsesQuotedNavigationArguments(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	player := profile.New("alex")
	player.RecordCommands([]string{"pwd"})
	if err := store.Save(player); err != nil {
		t.Fatal(err)
	}
	app, out, errOut := testApp(t, "list --campaign \"First Day\"\nquit\n", store)
	if err := app.Run([]string{"play", "3"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "Where Am I?") || !strings.Contains(out.String(), "0/6 missions complete") {
		t.Fatalf("quoted campaign list output:\n%s", out.String())
	}
	if strings.Contains(out.String(), "Production Friday") || errOut.Len() != 0 {
		t.Fatalf("quoted campaign navigation leaked missions or failed; stdout:\n%s\nstderr:\n%s", out.String(), errOut.String())
	}
}

func TestMissionPromptReportsMalformedNavigation(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "list --campaign \"First Day\nquit\n", store)
	if err := app.Run([]string{"play", "3"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}
	if !strings.Contains(errOut.String(), "invalid mission navigation: unterminated double quote") {
		t.Fatalf("malformed navigation stderr = %q", errOut.String())
	}
	if !strings.Contains(out.String(), "Mission paused") {
		t.Fatalf("session did not continue after malformed navigation:\n%s", out.String())
	}
}

func TestMissionPromptSupportsPreviousAndNext(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "previous\nnext\nquit\n", store)
	if err := app.Run([]string{"play", "9"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}
	for _, expected := range []string{
		"Switching to Mission 08: Wrong Planet",
		"Switching to Mission 09: Opening Lines",
	} {
		if !strings.Contains(out.String(), expected) {
			t.Errorf("arrow-style navigation output missing %q:\n%s", expected, out.String())
		}
	}
}
