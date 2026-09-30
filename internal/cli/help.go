package cli

import (
	"fmt"
	"io"
)

func (a *App) runHelp(args []string) error {
	if len(args) == 0 || len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		a.printUsage()
		return nil
	}
	if len(args) > 1 {
		return fmt.Errorf("usage: opsquest help [COMMAND]")
	}
	switch args[0] {
	case "play":
		a.printPlayUsage(a.out)
	case "guide", "tutorial":
		fmt.Fprintln(a.out, "Usage: opsquest guide")
	case "list", "campaign", "map", "worlds":
		a.printListUsage(a.out)
	case "profile":
		a.printProfileUsage(a.out)
	case "commands":
		fmt.Fprintln(a.out, "Usage: opsquest commands")
	case "achievements":
		fmt.Fprintln(a.out, "Usage: opsquest achievements")
	case "show", "mission":
		fmt.Fprintln(a.out, "Usage: opsquest show [MISSION]")
	case "doctor":
		fmt.Fprintln(a.out, "Usage: opsquest doctor [--cleanup]")
	case "reset":
		fmt.Fprintln(a.out, "Usage: opsquest reset [--yes]")
	case "version":
		fmt.Fprintln(a.out, "Usage: opsquest version")
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
	return nil
}

func (a *App) printPlayUsage(out io.Writer) {
	fmt.Fprintln(out, "Usage: opsquest play [--track linux|docker] [--world NUMBER] [--once] [--web] [MISSION]")
	fmt.Fprintln(out, "Without a selector, resume the first incomplete mission and continue the recommended path.")
	fmt.Fprintln(out, "A mission number or ID follows catalog order from that exact stage; --world stays inside one world.")
	fmt.Fprintln(out, "Use --once to return after one completed mission.")
	fmt.Fprintln(out, "Use --web to show guidance and live objective progress in a local browser companion.")
}

func (a *App) printListUsage(out io.Writer) {
	fmt.Fprintln(out, "Usage: opsquest map|list [--completed|--remaining] [--ids] [--campaign NAME] [--track linux|docker|all]")
}

func (a *App) printProfileUsage(out io.Writer) {
	fmt.Fprintln(out, "Usage: opsquest profile [--name NAME]")
}

func (a *App) printUsage() {
	fmt.Fprintln(a.out, a.style.Header("OpsQuest")+" — learn operations by fixing fictional production")
	fmt.Fprintln(a.out)
	fmt.Fprintln(a.out, `Usage:
  opsquest play [OPTIONS]  Follow the recommended path, one world, or one mission
  opsquest guide           Replay the getting-started guide
  opsquest map             View worlds, stages, and completion progress
  opsquest list            Alias for map; accepts progress filters
  opsquest profile         Show rank, XP, and world progress
  opsquest commands        Show commands practiced successfully
  opsquest achievements    Show learning achievements
  opsquest show [MISSION]  Preview a mission without starting it
  opsquest doctor          Check the catalog, profile, safety mode, and lab cleanup
  opsquest reset [--yes]   Reset local progress
  opsquest version         Print the version

New here: opsquest guide
Start playing: opsquest play`)
}
