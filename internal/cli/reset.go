package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

func (a *App) runReset(args []string) error {
	flags := a.newFlagSet("reset", func() { fmt.Fprintln(a.errOut, "Usage: opsquest reset [--yes]") })
	yes := flags.Bool("yes", false, "reset without confirmation")
	if help, err := parseFlags(flags, args); help || err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("reset does not accept positional arguments")
	}
	if !*yes {
		fmt.Fprint(a.out, "Reset all OpsQuest progress? [y/N] ")
		reader := bufio.NewReader(a.in)
		answer, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return fmt.Errorf("read confirmation: %w", err)
		}
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(a.out, "Reset cancelled.")
			return nil
		}
	}
	removed, err := a.store.Reset()
	if err != nil {
		return err
	}
	if removed {
		fmt.Fprintln(a.out, a.style.Success("Progress reset. Welcome back, Intern."))
	} else {
		fmt.Fprintln(a.out, a.style.Muted("No saved progress found."))
	}
	return nil
}
