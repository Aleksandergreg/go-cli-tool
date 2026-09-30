package cli

import (
	"fmt"

	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
)

func (a *App) runDoctor(args []string) error {
	flags := a.newFlagSet("doctor", func() { fmt.Fprintln(a.errOut, "Usage: opsquest doctor [--cleanup]") })
	cleanup := flags.Bool("cleanup", false, "remove Docker lab containers left behind by exited OpsQuest processes")
	if help, err := parseFlags(flags, args); help || err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("doctor does not accept positional arguments")
	}
	player, err := a.loadPlayer()
	if err != nil {
		return fmt.Errorf("profile check failed: %w", err)
	}
	fmt.Fprintln(a.out, a.style.Header("OpsQuest diagnostics"))
	check := a.style.Success("✓")
	fmt.Fprintf(a.out, "  %s embedded catalog: %d missions (%d Linux, %d Docker)\n", check, len(a.catalog.All()), len(a.catalog.InTrack(mission.TrackLinux)), len(a.catalog.InTrack(mission.TrackDocker)))
	fmt.Fprintf(a.out, "  %s profile: version %d, %d completed missions\n", check, player.Version, len(player.Completed))
	fmt.Fprintf(a.out, "  %s profile path: %s\n", check, a.store.Path())
	fmt.Fprintf(a.out, "  %s Linux labs: in-memory; no host shell or filesystem access\n", check)
	dockerReady := false
	if item, found := a.catalog.FirstInTrack(mission.TrackDocker); found {
		availability := game.EnvironmentAvailability(a.ctx, a.factory, item)
		dockerReady = availability.Available
		if availability.Available {
			fmt.Fprintf(a.out, "  %s docker labs: ready · %s\n", check, availability.Detail)
		} else {
			fmt.Fprintf(a.out, "  %s docker labs: unavailable · %s\n", a.style.Warning("!"), availability.Detail)
		}
	}
	return a.reportOrphanedLabs(*cleanup, dockerReady)
}

// reportOrphanedLabs checks for, or with cleanup removes, lab resources left
// behind by OpsQuest processes that exited without closing their attempt.
// A plain doctor run stays read-only and only checks a reachable engine.
func (a *App) reportOrphanedLabs(cleanup, dockerReady bool) error {
	janitor, supported := a.factory.(game.ResourceJanitor)
	check := a.style.Success("✓")
	warning := a.style.Warning("!")
	if !supported {
		if cleanup {
			fmt.Fprintf(a.out, "  %s docker cleanup: nothing to clean; Docker labs are not configured\n", check)
		}
		return nil
	}
	if cleanup {
		removed, err := janitor.RemoveOrphanedResources(a.ctx)
		if err != nil {
			if removed > 0 {
				fmt.Fprintf(a.out, "  %s docker cleanup: removed %d, then failed · %v\n", warning, removed, err)
			} else {
				fmt.Fprintf(a.out, "  %s docker cleanup: failed · %v\n", warning, err)
			}
			return fmt.Errorf("docker cleanup failed: %w", err)
		}
		fmt.Fprintf(a.out, "  %s docker cleanup: removed %d orphaned lab %s\n", check, removed, plural(removed, "resource", "resources"))
		return nil
	}
	if !dockerReady {
		return nil
	}
	count, err := janitor.OrphanedResources(a.ctx)
	switch {
	case err != nil:
		fmt.Fprintf(a.out, "  %s docker cleanup: could not check for orphaned lab resources · %v\n", warning, err)
	case count > 0:
		fmt.Fprintf(a.out, "  %s docker cleanup: %d orphaned lab %s from exited OpsQuest processes; run 'opsquest doctor --cleanup'\n", warning, count, plural(count, "resource", "resources"))
	default:
		fmt.Fprintf(a.out, "  %s docker cleanup: no orphaned lab resources\n", check)
	}
	return nil
}
