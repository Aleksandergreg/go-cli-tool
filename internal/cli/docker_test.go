package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/profile"
)

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
	if err := app.Run([]string{"list", "--track", "docker", "--ids"}); err != nil {
		t.Fatalf("list Docker track error = %v; stderr = %s", err, errOut.String())
	}
	for _, expected := range []string{
		"DOCKER LABS",
		"Container Census",
		"Shift Handoff",
		"docker-container-census",
		"WORLD 2/3 · Container Triage",
		"Postmortem Triage",
		"WORLD 3/3 · Network Plumbing",
		"Segmentation",
		"0/16 missions complete",
		"Continue: opsquest play --track docker",
		"Jump: opsquest play --track docker --world N",
		"IDs: opsquest map --track docker --ids",
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
	if err := app.Run([]string{"show", "20"}); err != nil {
		t.Fatalf("show Docker mission error = %v; stderr = %s", err, errOut.String())
	}
	for _, expected := range []string{
		"MISSION 20: Container Census",
		"Track: Docker",
		"Outcome checks: 3",
		"Hints: 0 used · 3 remaining · 3 total",
		"Commands you may need to solve this level:",
		"  docker",
		"Docker lab ready.",
	} {
		if !strings.Contains(out.String(), expected) {
			t.Errorf("Docker show output missing %q:\n%s", expected, out.String())
		}
	}
}

func TestFreshDockerPlayDoesNotShowLinuxOnboarding(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, out, errOut := testApp(t, "quit\n", store)
	app.factory = &cliDockerFactory{available: true, detail: "test Docker engine ready"}
	if err := app.Run([]string{"play", "--once", "20"}); err != nil {
		t.Fatalf("Docker play error = %v; stderr = %s", err, errOut.String())
	}
	if strings.Contains(out.String(), "WELCOME TO OPSQUEST") || strings.Contains(out.String(), "Start with the suggested command under the objective") {
		t.Fatalf("Docker play received Linux-specific onboarding:\n%s", out.String())
	}
	player, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if player.Onboarded {
		t.Fatal("skipping Linux onboarding for Docker marked it as viewed")
	}
}

func TestUnavailableDockerMissionIsActionableAndDoesNotComplete(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "alex")
	app, _, _ := testApp(t, "", store)
	app.factory = &cliDockerFactory{
		available: false,
		detail:    "Docker labs unavailable: install Docker and start the daemon",
	}
	err := app.Run([]string{"play", "20"})
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
	if err := app.Run([]string{"play", "--once", "20"}); err != nil {
		t.Fatalf("play Docker mission error = %v; stderr = %s", err, errOut.String())
	}
	for _, expected := range []string{
		"Hint 1/3 (-10 XP): Images are reusable templates",
		"Hint 2/3 (-10 XP): Use docker ps -a",
		"Hint 3/3 (-10 XP): Start the existing container with docker start api",
		"Progress — 2/3 outcome checks satisfied",
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
