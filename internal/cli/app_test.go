package cli

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/buildinfo"
	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
	"github.com/aleksandergregersen/opsquest/internal/ui"
)

var sgrPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func testApp(t *testing.T, input string, store profile.Store) (*App, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	return New(Config{
		In:      strings.NewReader(input),
		Out:     out,
		ErrOut:  errOut,
		Catalog: catalog,
		Store:   store,
	}), out, errOut
}

func TestNewUsesSafeContextAndFactoryDefaults(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, _, _ := testApp(t, "", store)
	if app.ctx == nil {
		t.Fatal("default CLI context is nil")
	}
	if _, ok := app.factory.(game.SandboxFactory); !ok {
		t.Fatalf("default factory = %T, want game.SandboxFactory", app.factory)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	factory := &cliDockerFactory{}
	configured := New(Config{
		Context: ctx,
		In:      strings.NewReader(""),
		Out:     &bytes.Buffer{},
		ErrOut:  &bytes.Buffer{},
		Catalog: app.catalog,
		Store:   store,
		Factory: factory,
	})
	if configured.ctx != ctx || configured.factory != factory {
		t.Fatalf("configured dependencies were not retained: context match %v, factory = %T", configured.ctx == ctx, configured.factory)
	}
}

func TestVersionUsesBuildInfo(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "", store)
	if err := app.Run([]string{"version"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}
	if got, want := out.String(), "OpsQuest "+buildinfo.Version+"\n"; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestNonTerminalOutputRemainsPlain(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "", store)
	if err := app.Run([]string{"list"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}
	if strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("buffered output contains ANSI escapes:\n%q", out.String())
	}
}

func TestForcedColorPreservesCLIText(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	plainApp, plainOut, plainErr := testApp(t, "", store)
	if err := plainApp.Run([]string{"show", "3"}); err != nil {
		t.Fatalf("plain Run() error = %v; stderr = %s", err, plainErr.String())
	}

	colorApp, colorOut, colorErr := testApp(t, "", store)
	colorApp.style = ui.New(true)
	colorApp.errorStyle = ui.New(true)
	if err := colorApp.Run([]string{"show", "3"}); err != nil {
		t.Fatalf("color Run() error = %v; stderr = %s", err, colorErr.String())
	}
	if !strings.Contains(colorOut.String(), "\x1b[") {
		t.Fatalf("forced-color output contains no ANSI styling:\n%q", colorOut.String())
	}
	if got := sgrPattern.ReplaceAllString(colorOut.String(), ""); got != plainOut.String() {
		t.Fatalf("color changed CLI text\ncolored without SGR:\n%q\nplain:\n%q", got, plainOut.String())
	}
}

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
	player, _ := store.Load()
	if player.XP != 40 {
		t.Fatalf("XP after replay = %d, want 40", player.XP)
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
		"Continuing to Mission 03: A Place for Everything",
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
	if player.IsComplete("linux-workspace") || player.XP != 85 {
		t.Fatalf("player after continuous play = %#v", player)
	}
}

func TestSelectedMissionReturnsAfterCompletion(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "pwd\ncd /srv/web/config/live\n", store)
	if err := app.Run([]string{"play", "1"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}
	if strings.Contains(out.String(), "MISSION 02") {
		t.Fatalf("selected mission unexpectedly continued:\n%s", out.String())
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
		"opsquest play 3",
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
		"1/19 missions complete",
		"Switching to Mission 03: A Place for Everything",
		"MISSION 03: A Place for Everything",
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

func TestMissionPromptParsesQuotedNavigationArguments(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "list --campaign \"First Day\"\nquit\n", store)
	if err := app.Run([]string{"play", "3"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "Where Am I?") || !strings.Contains(out.String(), "0/3 missions complete") {
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
		"Switching to Mission 08: The Runaway Worker",
		"Switching to Mission 09: Backup Archaeology",
	} {
		if !strings.Contains(out.String(), expected) {
			t.Errorf("arrow-style navigation output missing %q:\n%s", expected, out.String())
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

func TestListAndProfileWorkWithoutExistingSave(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "newbie")
	app, out, _ := testApp(t, "", store)
	if err := app.Run([]string{"list"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "0/19 missions complete") {
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

func TestProfileDefersDockerReadinessToDoctor(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "", store)
	factory := &cliDockerFactory{available: true, detail: "test Docker engine ready"}
	app.factory = factory

	if err := app.Run([]string{"profile"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}
	if factory.availabilityChecks != 0 {
		t.Fatalf("profile availability checks = %d, want 0", factory.availabilityChecks)
	}
	if !strings.Contains(out.String(), "readiness: run opsquest doctor") {
		t.Fatalf("profile did not direct readiness checks to doctor:\n%s", out.String())
	}
}

func TestListDockerTrackAndRejectsUnknownTrack(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "", store)
	app.factory = &cliDockerFactory{available: true, detail: "test Docker engine ready"}
	if err := app.Run([]string{"list", "--track", "docker"}); err != nil {
		t.Fatalf("list Docker track error = %v; stderr = %s", err, errOut.String())
	}
	for _, expected := range []string{
		"DOCKER LABS",
		"Container Census",
		"docker-container-census",
		"0/1 missions complete",
		"Docker labs ready.",
	} {
		if !strings.Contains(out.String(), expected) {
			t.Errorf("Docker list output missing %q:\n%s", expected, out.String())
		}
	}
	if strings.Contains(out.String(), "Where Am I?") {
		t.Fatalf("Docker list leaked a Linux mission:\n%s", out.String())
	}
	if got := app.factory.(*cliDockerFactory).availabilityChecks; got != 1 {
		t.Fatalf("Docker list availability checks = %d, want 1", got)
	}

	app, _, _ = testApp(t, "", store)
	if err := app.Run([]string{"list", "--track", "mainframe"}); err == nil || !strings.Contains(err.Error(), "use linux, docker, or all") {
		t.Fatalf("unknown track error = %v", err)
	}
}

func TestShowDockerMissionReportsReadinessAndToolHints(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "", store)
	app.factory = &cliDockerFactory{available: true, detail: "test Docker engine ready"}
	if err := app.Run([]string{"show", "17"}); err != nil {
		t.Fatalf("show Docker mission error = %v; stderr = %s", err, errOut.String())
	}
	for _, expected := range []string{
		"MISSION 17: Container Census",
		"Track: Docker",
		"Outcome checks: 3",
		"Hints available: 3",
		"Commands you may need to solve this level:",
		"  docker",
		"Docker lab ready.",
	} {
		if !strings.Contains(out.String(), expected) {
			t.Errorf("Docker show output missing %q:\n%s", expected, out.String())
		}
	}
}

func TestUnavailableDockerMissionIsActionableAndDoesNotComplete(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, _, _ := testApp(t, "", store)
	app.factory = &cliDockerFactory{
		available: false,
		detail:    "Docker labs unavailable: install Docker and start the daemon",
	}
	err := app.Run([]string{"play", "17"})
	if err == nil || !strings.Contains(err.Error(), "install Docker and start the daemon") {
		t.Fatalf("unavailable Docker play error = %v", err)
	}
	player, loadErr := store.Load()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if player.IsComplete("docker-container-census") || player.XP != 0 || len(player.Commands) != 0 {
		t.Fatalf("unavailable Docker play changed profile = %#v", player)
	}
	if got := app.factory.(*cliDockerFactory).availabilityChecks; got != 0 {
		t.Fatalf("Docker play availability checks = %d, want Create to be the only readiness path", got)
	}
}

func TestDockerMissionToolHintsAndOutcomeCompletionPersist(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	input := strings.Join([]string{
		"hint",
		"hint",
		"hint",
		"docker ps -a",
		"docker start api",
	}, "\n") + "\n"
	app, out, errOut := testApp(t, input, store)
	factory := &cliDockerFactory{available: true, detail: "test Docker engine ready"}
	app.factory = factory
	if err := app.Run([]string{"play", "17"}); err != nil {
		t.Fatalf("play Docker mission error = %v; stderr = %s", err, errOut.String())
	}
	for _, expected := range []string{
		"Hint 1/3 (-10 XP): Images are reusable templates",
		"Hint 2/3 (-10 XP): Use docker ps -a",
		"Hint 3/3 (-10 XP): Start the existing container with docker start api",
		"Not complete yet — 2/3 outcome checks satisfied",
		"✓ Mission complete!",
		"+20 XP",
		"New command discovered: docker",
	} {
		if !strings.Contains(out.String(), expected) {
			t.Errorf("Docker play output missing %q:\n%s", expected, out.String())
		}
	}
	if factory.created != 1 {
		t.Fatalf("Docker environments created = %d, want 1", factory.created)
	}
	if factory.availabilityChecks != 0 {
		t.Fatalf("Docker play availability checks = %d, want Create to be the only readiness path", factory.availabilityChecks)
	}

	player, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	completion, complete := player.Completed["docker-container-census"]
	if !complete || player.XP != 20 || completion.XP != 20 || completion.HintsUsed != 3 {
		t.Fatalf("Docker completion profile = %#v", player)
	}
	if got := player.Commands["docker"]; got != 2 {
		t.Fatalf("docker command mastery count = %d, want 2", got)
	}
	if player.MissionHints("docker-container-census") != 0 {
		t.Fatal("completed Docker mission retained in-progress hints")
	}
}

func TestSubcommandHelpIsSuccessful(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, _, errOut := testApp(t, "", store)
	if err := app.Run([]string{"play", "--help"}); err != nil {
		t.Fatalf("play --help returned error: %v", err)
	}
	if !strings.Contains(errOut.String(), "Usage: opsquest play") {
		t.Fatalf("help output = %s", errOut.String())
	}
}

func TestProfileRenameShowAndDoctor(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, _ := testApp(t, "", store)
	if err := app.Run([]string{"profile", "--name", "casey"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Operator: casey") {
		t.Fatalf("profile output = %s", out.String())
	}

	app, out, _ = testApp(t, "", store)
	if err := app.Run([]string{"show", "16"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Production Friday") || !strings.Contains(out.String(), "Reward: 160 XP") ||
		!strings.Contains(out.String(), "Commands you may need to solve this level:") ||
		!strings.Contains(out.String(), "  tar, sed, vi, chmod, ps, kill") {
		t.Fatalf("show output = %s", out.String())
	}

	app, out, _ = testApp(t, "", store)
	if err := app.Run([]string{"doctor"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "20 missions (19 Linux, 1 Docker)") || !strings.Contains(out.String(), "Linux labs: in-memory; no host shell or filesystem access") {
		t.Fatalf("doctor output = %s", out.String())
	}
}

func TestPipelineAndBossAchievementsUnlock(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	command := "grep ERROR incidents.log | awk '{print $3}' | sort | uniq > /reports/error-services.txt\n"
	app, out, errOut := testApp(t, command, store)
	if err := app.Run([]string{"play", "10"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}
	player, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"first-fix", "pipe-dream", "boss-slayer"} {
		if !player.HasAchievement(id) {
			t.Errorf("achievement %s was not unlocked", id)
		}
	}
	if !strings.Contains(out.String(), "Achievement unlocked: Pipe Dream") {
		t.Fatalf("completion output = %s", out.String())
	}

	app, out, _ = testApp(t, "", store)
	if err := app.Run([]string{"achievements"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "3/6 unlocked") {
		t.Fatalf("achievements output = %s", out.String())
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
	if !strings.Contains(out.String(), "0/3 missions complete") {
		t.Fatalf("campaign-filtered total missing:\n%s", out.String())
	}
	if strings.Contains(out.String(), "0/20 missions complete") || strings.Contains(out.String(), "Production Friday") {
		t.Fatalf("campaign filter used catalog-wide scope:\n%s", out.String())
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
	if err := app.Run([]string{"play", "3"}); err != nil {
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
	if err := app.Run([]string{"play", "3"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}
	output := out.String()
	for _, expected := range []string{
		"Not complete yet — 1/2 outcome checks satisfied",
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
	if err := app.Run([]string{"play", "3"}); err != nil {
		t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "✓ Mission complete!") {
		t.Fatalf("alternative solution output:\n%s", out.String())
	}
}

func TestCompletionistRequiresEveryCurrentCatalogMission(t *testing.T) {
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	items := catalog.InTrack(mission.TrackLinux)
	completedAt := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		seedFrom   int
		wantUnlock bool
	}{
		{name: "unknown ID cannot replace missing mission", seedFrom: 2, wantUnlock: false},
		{name: "all current missions still unlock", seedFrom: 1, wantUnlock: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
			player := profile.New("alex")
			for _, item := range items[test.seedFrom:] {
				player.Complete(item.ID, 0, 0, completedAt)
			}
			player.Complete("retired-linux-mission", 0, 0, completedAt)
			if err := store.Save(player); err != nil {
				t.Fatal(err)
			}

			app, out, errOut := testApp(t, "pwd\n", store)
			if err := app.Run([]string{"play", "1"}); err != nil {
				t.Fatalf("Run() error = %v; stderr = %s", err, errOut.String())
			}
			player, err = store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if got := player.HasAchievement("linux-completionist"); got != test.wantUnlock {
				t.Fatalf("completionist unlocked = %v, want %v; completed = %#v", got, test.wantUnlock, player.Completed)
			}
			if announced := strings.Contains(out.String(), "Achievement unlocked: Linux Completionist"); announced != test.wantUnlock {
				t.Fatalf("completionist announcement = %v, want %v; output:\n%s", announced, test.wantUnlock, out.String())
			}
		})
	}
}

type cliDockerFactory struct {
	available          bool
	detail             string
	created            int
	availabilityChecks int
}

func (f *cliDockerFactory) Availability(_ context.Context, item mission.Mission) game.Availability {
	f.availabilityChecks++
	if item.EffectiveEnvironment() != mission.EnvironmentDocker {
		return game.Availability{Available: true, Detail: "in-memory sandbox ready"}
	}
	return game.Availability{Available: f.available, Detail: f.detail}
}

func (f *cliDockerFactory) Create(ctx context.Context, item mission.Mission) (game.Environment, error) {
	if item.EffectiveEnvironment() != mission.EnvironmentDocker {
		return (game.SandboxFactory{}).Create(ctx, item)
	}
	if !f.available {
		return nil, fmt.Errorf("%s", f.detail)
	}
	f.created++
	return &cliDockerEnvironment{
		running: map[string]bool{
			"api":     false,
			"metrics": true,
		},
		containerCount: 2,
	}, nil
}

type cliDockerEnvironment struct {
	running        map[string]bool
	containerCount int
	closed         bool
}

func (e *cliDockerEnvironment) PromptLabel() string { return "docker" }

func (e *cliDockerEnvironment) CompletionSource() game.CompletionSource { return nil }

func (e *cliDockerEnvironment) Execute(ctx context.Context, line string) (game.Execution, error) {
	if err := ctx.Err(); err != nil {
		return game.Execution{}, err
	}
	if e.closed {
		return game.Execution{}, fmt.Errorf("Docker test environment is closed")
	}
	fields := strings.Fields(line)
	if len(fields) == 3 && fields[0] == "docker" && fields[1] == "ps" && fields[2] == "-a" {
		return game.Execution{
			Output:            "CONTAINER ID   NAME      STATUS\nlab-01         api       Exited\nlab-02         metrics   Up\n",
			PracticedCommands: []string{"docker"},
			PipelineWidth:     1,
		}, nil
	}
	if len(fields) == 3 && fields[0] == "docker" && fields[1] == "start" && fields[2] == "api" {
		e.running["api"] = true
		return game.Execution{Output: "api\n", PracticedCommands: []string{"docker"}, PipelineWidth: 1}, nil
	}
	return game.Execution{}, fmt.Errorf("unsupported Docker test command %q", line)
}

func (e *cliDockerEnvironment) Observe(ctx context.Context, condition mission.Condition) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	switch condition.Type {
	case "docker_container_running":
		return e.running[condition.Container], nil
	case "docker_container_count_equals":
		if condition.Count == nil {
			return false, fmt.Errorf("missing container count")
		}
		return e.containerCount == *condition.Count, nil
	default:
		return false, fmt.Errorf("unsupported Docker test observation %q", condition.Type)
	}
}

func (e *cliDockerEnvironment) Close() error {
	e.closed = true
	return nil
}
