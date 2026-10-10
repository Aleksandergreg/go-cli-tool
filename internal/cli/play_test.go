package cli

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
)

func TestPlayAwardsHintAdjustedXPAndPersistsProgress(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "hint\npwd\n", store)
	if err := app.Run([]string{"play", "01"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "✓ Mission complete!") || !strings.Contains(out.String(), "+30 XP") {
		t.Fatalf("output did not show adjusted completion:\n%s", out.String())
	}
	player, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if player.XP != 30 || !player.IsComplete("linux-orientation") || player.Commands["pwd"] != 1 {
		t.Fatalf("profile = %#v", player)
	}
}

func TestReplayDoesNotAwardXPAgain(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	first, _, _ := testApp(t, "pwd\n", store)
	if err := first.Run([]string{"play", "1"}); err != nil {
		t.Fatal(err)
	}
	second, out, _ := testApp(t, "pwd\n", store)
	if err := second.Run([]string{"play", "linux-orientation"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "XP was already claimed") {
		t.Fatalf("replay output = %s", out.String())
	}
	if !strings.Contains(out.String(), "Reward: already claimed · 40 XP base") {
		t.Fatalf("replay mission advertised claimable XP:\n%s", out.String())
	}
	player, _ := store.Load()
	if player.XP != 40 {
		t.Fatalf("XP after replay = %d, want 40", player.XP)
	}
	preview, previewOut, _ := testApp(t, "", store)
	if err := preview.Run([]string{"show", "1"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(previewOut.String(), "Reward: claimed · 40 XP earned") || !strings.Contains(previewOut.String(), "Hints: 0 used on first completion") {
		t.Fatalf("completed preview advertised a fresh reward or hint budget:\n%s", previewOut.String())
	}
}

func TestBarePlayContinuesThroughIncompleteMissions(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	input := strings.Join([]string{
		"pwd",
		"cd /srv/web/config/live",
		"quit",
	}, "\n") + "\n"
	app, out, errOut := testApp(t, input, store)
	if err := app.Run([]string{"play"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}

	output := out.String()
	for _, expected := range []string{
		"✓ Mission complete!",
		"Continuing to Mission 02: Configuration Crawl",
		"Continuing to Mission 03: Read the Handoff",
		"Mission paused",
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("continuous play output missing %q:\n%s", expected, output)
		}
	}
	player, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !player.IsComplete("linux-orientation") || !player.IsComplete("linux-config-crawl") {
		t.Fatalf("completed missions = %#v", player.Completed)
	}
	if player.IsComplete("linux-read-handoff") || player.XP != 85 {
		t.Fatalf("player after continuous play = %#v", player)
	}
}

func TestSelectedMissionContinuesAfterCompletion(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "pwd\nquit\n", store)
	if err := app.Run([]string{"play", "1"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}
	for _, expected := range []string{"Continuing to Mission 02: Configuration Crawl", "MISSION 02", "Mission paused"} {
		if !strings.Contains(out.String(), expected) {
			t.Fatalf("selected mission did not continue with %q:\n%s", expected, out.String())
		}
	}
	if strings.Contains(out.String(), "NEXT RECOMMENDED") {
		t.Fatalf("continuous mission unexpectedly returned a one-shot recommendation:\n%s", out.String())
	}
}

func TestSelectedMissionReplayContinuesWithoutRepeatingWorldCompletion(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	seedCompletedMissions(t, store, linuxMissionIDsThroughArchive...)

	app, out, errOut := testApp(t, "pwd\nquit\n", store)
	if err := app.Run([]string{"play", "1"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}
	for _, expected := range []string{"Replay complete", "Continuing to Mission 02: Configuration Crawl", "MISSION 02"} {
		if !strings.Contains(out.String(), expected) {
			t.Fatalf("selected replay did not continue with %q:\n%s", expected, out.String())
		}
	}
	if strings.Contains(out.String(), "World 1 complete") {
		t.Fatalf("replay repeated an already-earned world completion:\n%s", out.String())
	}
	if strings.Contains(out.String(), "Continuing to Mission 12") {
		t.Fatalf("selected replay snapped back to global progress:\n%s", out.String())
	}
}

func TestInMissionIDJumpFollowsTheSelectedSequence(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	seedCompletedMissions(t, store, linuxMissionIDsThroughArchive...)

	app, out, errOut := testApp(t, "play linux-orientation\npwd\nquit\n", store)
	if err := app.Run([]string{"play", "10"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}
	output := out.String()
	switchIndex := strings.Index(output, "Switching to Mission 01")
	if switchIndex < 0 {
		t.Fatalf("mission jump was not shown:\n%s", output)
	}
	afterJump := output[switchIndex:]
	for _, expected := range []string{"MISSION 01: Where Am I?", "Replay complete", "Continuing to Mission 02: Configuration Crawl", "MISSION 02"} {
		if !strings.Contains(afterJump, expected) {
			t.Fatalf("selected sequence missing %q:\n%s", expected, afterJump)
		}
	}
	if strings.Contains(afterJump, "Continuing to Mission 12") {
		t.Fatalf("in-mission jump snapped back to global progress:\n%s", afterJump)
	}
}

func TestPlayOnceReturnsAfterSelectedMission(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "pwd\n", store)
	if err := app.Run([]string{"play", "--once", "1"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}
	if strings.Contains(out.String(), "MISSION 02") {
		t.Fatalf("--once unexpectedly continued:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "NEXT RECOMMENDED") || !strings.Contains(out.String(), "Mission 02: Configuration Crawl") {
		t.Fatalf("--once did not explain the next step:\n%s", out.String())
	}
}

func TestPlayCanJumpToAndCompleteOneWorld(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "quit\n", store)
	if err := app.Run([]string{"play", "--world", "2"}); err != nil {
		t.Fatalf("world jump error = %v; stderr = %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "MISSION 07: Permission to Deploy") || !strings.Contains(out.String(), "World 2/4") {
		t.Fatalf("world jump started the wrong stage:\n%s", out.String())
	}

	commands := strings.Join([]string{
		"pwd",
		"cd /srv/web/config/live",
		"cat handoff.txt",
		"mkdir -p reports/daily",
		"touch reports/daily/summary.txt",
		`find . -name "*.log" -exec grep -l "ERROR" {} \;`,
		"mv incident-104.txt /archive/2026/incident-104.txt",
	}, "\n") + "\n"
	app, out, errOut = testApp(t, commands, profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex"))
	if err := app.Run([]string{"play", "--world", "1"}); err != nil {
		t.Fatalf("world play error = %v; stderr = %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "World 1 complete: First Day!") || strings.Contains(out.String(), "MISSION 07") {
		t.Fatalf("world play did not stop at its boundary:\n%s", out.String())
	}

	replayStore := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	seedCompletedMissions(t, replayStore, linuxMissionIDsThroughArchive[:6]...)
	app, out, errOut = testApp(t, commands, replayStore)
	if err := app.Run([]string{"play", "--world", "1"}); err != nil {
		t.Fatalf("completed world replay error = %v; stderr = %s", err, errOut.String())
	}
	for _, expected := range []string{
		"World 1 is complete; replaying Stage 1",
		"MISSION 02: Configuration Crawl",
		"MISSION 03: Read the Handoff",
		"MISSION 04: A Place for Everything",
		"MISSION 05: The Missing Log File",
		"MISSION 06: The Release Shuffle",
		"World 1 complete: First Day!",
	} {
		if !strings.Contains(out.String(), expected) {
			t.Fatalf("completed world replay missing %q:\n%s", expected, out.String())
		}
	}
	if strings.Contains(out.String(), "MISSION 07") {
		t.Fatalf("completed world replay crossed its boundary:\n%s", out.String())
	}

	app, _, _ = testApp(t, "", store)
	if err := app.Run([]string{"play", "--world", "99"}); err == nil || !strings.Contains(err.Error(), "world 99 does not exist") {
		t.Fatalf("invalid world error = %v", err)
	}
	for _, test := range []struct {
		args []string
		want string
	}{
		{args: []string{"play", "--world", "0"}, want: "world number must be positive"},
		{args: []string{"play", "--track", "mainframe"}, want: "use linux or docker"},
		{args: []string{"play", "--world", "1", "3"}, want: "MISSION cannot be combined"},
	} {
		app, _, _ = testApp(t, "", store)
		if err := app.Run(test.args); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("Run(%v) error = %v, want %q", test.args, err, test.want)
		}
	}
}

func TestHintPenaltySurvivesAPausedAttempt(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	paused, _, _ := testApp(t, "hint\nquit\n", store)
	if err := paused.Run([]string{"play", "1"}); err != nil {
		t.Fatal(err)
	}
	player, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if player.MissionHints("linux-orientation") != 1 {
		t.Fatalf("saved hint progress = %d, want 1", player.MissionHints("linux-orientation"))
	}
	preview, previewOut, previewErr := testApp(t, "", store)
	if err := preview.Run([]string{"show", "1"}); err != nil {
		t.Fatalf("show hinted mission error = %v; stderr = %s", err, previewErr.String())
	}
	if !strings.Contains(previewOut.String(), "Reward: 30 XP") || !strings.Contains(previewOut.String(), "Hints: 1 used · 1 remaining · 2 total") {
		t.Fatalf("show did not reflect persisted hint state:\n%s", previewOut.String())
	}
	profileApp, profileOut, profileErr := testApp(t, "", store)
	if err := profileApp.Run([]string{"profile"}); err != nil {
		t.Fatalf("profile with active hint error = %v; stderr = %s", err, profileErr.String())
	}
	if !strings.Contains(profileOut.String(), "Hints used: 1 (completed: 0 · active: 1)") {
		t.Fatalf("profile hid active hint penalties:\n%s", profileOut.String())
	}

	resumed, out, _ := testApp(t, "pwd\n", store)
	if err := resumed.Run([]string{"play", "1"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "+30 XP") {
		t.Fatalf("resumed output did not retain hint penalty:\n%s", out.String())
	}
	player, _ = store.Load()
	if player.MissionHints("linux-orientation") != 0 {
		t.Fatalf("hint progress was not cleared after completion")
	}
}

func TestRevealedHintsCanBeRereadWithoutExtraCost(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, _ := catalog.Find("linux-read-handoff")
	if len(item.Hints) < 2 {
		t.Fatalf("mission hints = %d, want at least 2 for this test", len(item.Hints))
	}

	paused, pausedOut, _ := testApp(t, "hint\nquit\n", store)
	if err := paused.Run([]string{"play", "--once", item.ID}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(pausedOut.String(), "REVEALED HINTS") {
		t.Fatalf("fresh attempt listed revealed hints before any were used:\n%s", pausedOut.String())
	}

	exhaust := strings.Repeat("hint\n", len(item.Hints)) + "hint\nquit\n"
	resumed, out, errOut := testApp(t, exhaust, store)
	if err := resumed.Run([]string{"play", "--once", item.ID}); err != nil {
		t.Fatalf("resumed play error = %v; stderr = %s", err, errOut.String())
	}
	banner, afterBanner, found := strings.Cut(out.String(), "Controls:")
	if !found {
		t.Fatalf("resumed output lacks mission controls:\n%s", out.String())
	}
	if !strings.Contains(banner, "REVEALED HINTS (no extra cost)") || !strings.Contains(banner, "Hint 1/"+strconv.Itoa(len(item.Hints))+": "+item.Hints[0]) {
		t.Fatalf("resumed banner did not repeat the paid hint:\n%s", banner)
	}
	if strings.Contains(banner, item.Hints[1]) {
		t.Fatalf("resumed banner revealed an unpaid hint:\n%s", banner)
	}
	_, exhausted, found := strings.Cut(afterBanner, "No more hints.")
	if !found {
		t.Fatalf("exhausted hint request was not reported:\n%s", afterBanner)
	}
	for _, hint := range item.Hints {
		if !strings.Contains(exhausted, hint) {
			t.Errorf("exhausted hint request did not repeat %q:\n%s", hint, exhausted)
		}
	}
	player, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := player.MissionHints(item.ID); got != len(item.Hints) {
		t.Fatalf("saved hints = %d, want %d; rereading must not record extra hints", got, len(item.Hints))
	}
}

func TestHintReportsTheActualFloorAdjustedCost(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "hint\nhint\nhint\nhint\nhint\nquit\n", store)
	if err := app.Run([]string{"play", "12"}); err != nil {
		t.Fatalf("hinted play error = %v; stderr = %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "Hint 5/5 (-14 XP):") {
		t.Fatalf("last hint did not report the reward floor's actual cost:\n%s", out.String())
	}
}

func TestReplayHintsAdvanceWithoutChangingProgress(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	player := profile.New("alex")
	completedAt := time.Unix(1, 0)
	player.Complete("linux-find-logs", 75, 0, completedAt)
	if err := store.Save(player); err != nil {
		t.Fatal(err)
	}

	app, out, errOut := testApp(t, "hint\nhint\nhint\nhint\nquit\n", store)
	if err := app.Run([]string{"play", "5"}); err != nil {
		t.Fatalf("replay hints error = %v; stderr = %s", err, errOut.String())
	}
	for _, expected := range []string{
		"Hint 1/3 (no XP cost):",
		"Hint 2/3 (no XP cost):",
		"Hint 3/3 (no XP cost):",
		"No more hints.",
	} {
		if count := strings.Count(out.String(), expected); count != 1 {
			t.Fatalf("replay output contains %q %d times, want once:\n%s", expected, count, out.String())
		}
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	completion := loaded.Completed["linux-find-logs"]
	if loaded.XP != 75 || completion.XP != 75 || !completion.CompletedAt.Equal(completedAt) {
		t.Fatalf("replay hints changed completion progress: %#v", loaded)
	}
	if got := loaded.MissionHints("linux-find-logs"); got != 0 {
		t.Fatalf("replay hint progress persisted as %d, want 0", got)
	}
}

func TestMissionStatusAndRestart(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	input := strings.Join([]string{
		"mkdir -p reports/daily",
		"status",
		"restart",
		"status",
		"mkdir -p reports/daily",
		"touch reports/daily/summary.txt",
	}, "\n") + "\n"
	app, out, errOut := testApp(t, input, store)
	if err := app.Run([]string{"play", "4"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "Outcome checks satisfied: 1/2") || !strings.Contains(out.String(), "Outcome checks satisfied: 0/2") {
		t.Fatalf("status output = %s", out.String())
	}
	if !strings.Contains(out.String(), "Mission environment restarted") || !strings.Contains(out.String(), "✓ Mission complete!") {
		t.Fatalf("restart output = %s", out.String())
	}
}

func TestMissionStatusExplainsWrongPathWithoutRequiringCommandOrder(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	input := strings.Join([]string{
		"mkdir -p /workspace/reports/daily",
		"touch summary.txt",
		"status",
		"quit",
	}, "\n") + "\n"
	app, out, errOut := testApp(t, input, store)
	if err := app.Run([]string{"play", "4"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}
	output := out.String()
	for _, expected := range []string{
		"Progress — 1/2 outcome checks satisfied",
		"✓ Directory exists: /workspace/reports/daily",
		"○ File exists: /workspace/reports/daily/summary.txt",
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("status output missing %q:\n%s", expected, output)
		}
	}
	player, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if player.IsComplete("linux-workspace") {
		t.Fatal("wrong-path file unexpectedly completed the mission")
	}
}

func TestWorkspaceAcceptsChangingDirectoryBeforeCreatingFile(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	input := "mkdir -p reports/daily\ncd reports/daily\ntouch summary.txt\n"
	app, out, errOut := testApp(t, input, store)
	if err := app.Run([]string{"play", "4"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "✓ Mission complete!") {
		t.Fatalf("alternative solution output:\n%s", out.String())
	}
}
