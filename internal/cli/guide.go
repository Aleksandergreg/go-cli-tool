package cli

import (
	"fmt"

	"github.com/aleksandergregersen/opsquest/internal/profile"
)

func (a *App) runGuide(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("guide does not accept arguments")
	}
	player, err := a.loadPlayer()
	if err != nil {
		return err
	}
	if !player.Onboarded {
		player.Onboarded = true
		if err := a.store.Save(player); err != nil {
			return err
		}
	}
	a.printGuide()
	return nil
}

func (a *App) printQuickStart() {
	fmt.Fprintln(a.out, a.style.Header("WELCOME TO OPSQUEST"))
	fmt.Fprintln(a.out, "Repair fictional ByteWorks incidents while learning Linux in a safe terminal lab.")
	fmt.Fprintln(a.out)
	fmt.Fprintln(a.out, a.style.Section("THE THREE RULES"))
	fmt.Fprintf(a.out, "  %s Produce the objective's result; the final outcome is what counts.\n", a.style.Accent("1."))
	fmt.Fprintf(a.out, "  %s Try any supported route. Suggested tools orient you without requiring one command.\n", a.style.Accent("2."))
	fmt.Fprintf(a.out, "  %s Hints are always available, but they reduce bonus XP.\n", a.style.Accent("3."))
	fmt.Fprintln(a.out)
	fmt.Fprintln(a.out, a.style.Section("PROGRESSION"))
	fmt.Fprintf(a.out, "  %s follows the next incomplete stage; %s shows all four Linux worlds.\n",
		a.style.Accent("opsquest play"), a.style.Accent("opsquest map"))
	fmt.Fprintln(a.out, "  Every 100 XP raises your level. Worlds guide the learning order but never lock replay or jumps.")
	fmt.Fprintln(a.out)
	fmt.Fprintln(a.out, a.style.Section("IN THE LAB"))
	fmt.Fprintf(a.out, "  Use %s for commands, %s for outcome progress, and %s for a nudge.\n",
		a.style.Accent("help"), a.style.Accent("status"), a.style.Accent("hint"))
	fmt.Fprintln(a.out, "  Tab completes commands and virtual paths; arrows edit input and recall history.")
	fmt.Fprintln(a.out, "  Player commands never reach your host shell or files.")
	fmt.Fprintf(a.out, "  Read the full guide any time with %s. Start with the suggested command under the objective.\n\n",
		a.style.Accent("opsquest guide"))
}

func (a *App) printGuide() {
	fmt.Fprintln(a.out, a.style.Header("WELCOME TO OPSQUEST"))
	fmt.Fprintln(a.out, "Learn Linux by repairing fictional ByteWorks incidents in short terminal missions.")
	fmt.Fprintln(a.out)

	fmt.Fprintln(a.out, a.style.Section("HOW A MISSION WORKS"))
	fmt.Fprintf(a.out, "  %s Explore the lab and produce the requested result.\n", a.style.Accent("1."))
	fmt.Fprintf(a.out, "  %s Use any supported command sequence—the final outcome is what counts.\n", a.style.Accent("2."))
	fmt.Fprintf(a.out, "  %s Earn XP, discover commands, and continue to the next incomplete stage.\n", a.style.Accent("3."))
	fmt.Fprintln(a.out, "  Hints are always available; each one reduces the mission's bonus XP, never your progress.")
	fmt.Fprintln(a.out)

	fmt.Fprintln(a.out, a.style.Section("YOUR LEARNING PATH"))
	fmt.Fprintln(a.out, "  Linux is arranged into worlds. Each begins with focused practice")
	fmt.Fprintln(a.out, "  and builds toward a larger challenge.")
	fmt.Fprintf(a.out, "  %s Orientation, files, search, and movement\n", a.style.World("World 1 · First Day —"))
	fmt.Fprintf(a.out, "  %s Permissions, environment, processes, archives, and pipelines\n", a.style.World("World 2 · The Logpocalypse —"))
	fmt.Fprintf(a.out, "  %s Logs, transformation, ownership, and incident repair\n", a.style.World("World 3 · Production Friday —"))
	fmt.Fprintf(a.out, "  %s Modal editing and reusable shell scripts\n", a.style.World("World 4 · The Automation Shift —"))
	fmt.Fprintf(a.out, "  Run %s to follow the recommended path, or %s to inspect and jump between worlds.\n",
		a.style.Accent("opsquest play"), a.style.Accent("opsquest map"))
	fmt.Fprintln(a.out, "  Completed stages remain replayable; explicit mission jumps are never locked.")
	fmt.Fprintf(a.out, "  Every 100 XP raises your level; ranks and world progress are shown by %s.\n", a.style.Accent("opsquest profile"))
	fmt.Fprintln(a.out)

	fmt.Fprintln(a.out, a.style.Section("INSIDE THE LAB"))
	fmt.Fprintf(a.out, "  %s lists teaching-shell commands; %s explains one command.\n", a.style.Accent("help"), a.style.Accent("help COMMAND"))
	fmt.Fprintf(a.out, "  %s shows the goal, %s shows outcome progress, and %s reveals the next hint.\n",
		a.style.Accent("objective"), a.style.Accent("status"), a.style.Accent("hint"))
	fmt.Fprintf(a.out, "  %s shows every mission control. Tab completes commands and paths; arrows edit and recall input.\n", a.style.Accent("?"))
	fmt.Fprintln(a.out)

	fmt.Fprintln(a.out, a.style.Section("SAFE BY DESIGN"))
	fmt.Fprintln(a.out, "  Linux labs use an in-memory filesystem and simulated processes.")
	fmt.Fprintln(a.out, "  Player commands never reach your host shell or files.")
	fmt.Fprintln(a.out, "  OpsQuest commands such as doctor and profile run outside a lab—")
	fmt.Fprintln(a.out, "  after you quit or from another terminal.")
	fmt.Fprintln(a.out)
	fmt.Fprintf(a.out, "%s Start with the suggested command under the objective.\n\n", a.style.Success("Ready?"))
}

func isPristineProfile(player profile.Profile) bool {
	return !player.Onboarded && player.XP == 0 && len(player.Completed) == 0 && len(player.Commands) == 0 &&
		len(player.Hints) == 0 && len(player.Unlocked) == 0
}
