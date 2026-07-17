package cli

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
)

const version = "0.1.0"

type App struct {
	in      io.Reader
	out     io.Writer
	errOut  io.Writer
	catalog mission.Catalog
	store   profile.Store
}

func New(in io.Reader, out, errOut io.Writer) (*App, error) {
	catalog, err := mission.LoadCatalog()
	if err != nil {
		return nil, err
	}
	store, err := profile.DefaultStore()
	if err != nil {
		return nil, err
	}
	return NewWithDependencies(in, out, errOut, catalog, store), nil
}

func NewWithDependencies(in io.Reader, out, errOut io.Writer, catalog mission.Catalog, store profile.Store) *App {
	return &App{in: in, out: out, errOut: errOut, catalog: catalog, store: store}
}

func (a *App) Run(args []string) error {
	if len(args) == 0 {
		a.printUsage()
		return nil
	}
	switch args[0] {
	case "play":
		return a.runPlay(args[1:])
	case "list", "campaign":
		return a.runList(args[1:])
	case "profile":
		return a.runProfile(args[1:])
	case "commands":
		return a.runCommands(args[1:])
	case "reset":
		return a.runReset(args[1:])
	case "version", "--version", "-v":
		if len(args) > 1 {
			return fmt.Errorf("version does not accept arguments")
		}
		fmt.Fprintf(a.out, "OpsQuest %s\n", version)
		return nil
	case "help", "--help", "-h":
		a.printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q; run 'opsquest help'", args[0])
	}
}

func (a *App) runPlay(args []string) error {
	flags := flag.NewFlagSet("play", flag.ContinueOnError)
	flags.SetOutput(a.errOut)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return fmt.Errorf("usage: opsquest play [MISSION]")
	}
	player, err := a.store.Load()
	if err != nil {
		return err
	}
	var item mission.Mission
	if flags.NArg() == 1 {
		var found bool
		item, found = a.catalog.Find(flags.Arg(0))
		if !found {
			return fmt.Errorf("mission %q not found; run 'opsquest list'", flags.Arg(0))
		}
	} else {
		var found bool
		item, found = a.catalog.Next(player.IsComplete)
		if !found {
			fmt.Fprintln(a.out, "Campaign complete! Replay a mission with 'opsquest play MISSION'.")
			return nil
		}
	}
	session := game.Session{
		Mission: item,
		Player:  &player,
		Store:   a.store,
		In:      a.in,
		Out:     a.out,
		Err:     a.errOut,
	}
	_, err = session.Run()
	return err
}

func (a *App) runList(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("list does not accept arguments")
	}
	player, err := a.store.Load()
	if err != nil {
		return err
	}
	fmt.Fprintln(a.out, "LINUX CAMPAIGN")
	lastCampaign := ""
	for _, item := range a.catalog.All() {
		if item.Campaign != lastCampaign {
			if lastCampaign != "" {
				fmt.Fprintln(a.out)
			}
			fmt.Fprintf(a.out, "%s\n", item.Campaign)
			lastCampaign = item.Campaign
		}
		status := "○"
		if player.IsComplete(item.ID) {
			status = "✓"
		}
		fmt.Fprintf(a.out, "  %s %02d  %-38s %-12s %3d XP  %s\n", status, item.Number, item.Title, item.Difficulty, item.Rewards.XP, item.ID)
	}
	completed := 0
	for _, item := range a.catalog.All() {
		if player.IsComplete(item.ID) {
			completed++
		}
	}
	fmt.Fprintf(a.out, "\n%d/%d missions complete · %d XP\n", completed, len(a.catalog.All()), player.XP)
	return nil
}

func (a *App) runProfile(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("profile does not accept arguments")
	}
	player, err := a.store.Load()
	if err != nil {
		return err
	}
	total := len(a.catalog.All())
	completed := 0
	for _, item := range a.catalog.All() {
		if player.IsComplete(item.ID) {
			completed++
		}
	}
	fmt.Fprintf(a.out, "Operator: %s\n", player.Name)
	fmt.Fprintf(a.out, "Rank: %s\n", player.Rank())
	fmt.Fprintf(a.out, "Level: %d · %d XP\n\n", player.Level(), player.XP)
	fmt.Fprintf(a.out, "Linux  %s %3d%%\n", progressBar(completed, total, 20), percentage(completed, total))
	fmt.Fprintf(a.out, "Docker %s  locked\n", progressBar(0, 20, 20))
	fmt.Fprintf(a.out, "K8s    %s  locked\n\n", progressBar(0, 20, 20))
	fmt.Fprintf(a.out, "Commands mastered: %d\n", len(player.Commands))
	fmt.Fprintf(a.out, "Missions completed: %d\n", completed)
	fmt.Fprintf(a.out, "Hints used: %d\n", player.HintsUsed())
	return nil
}

func (a *App) runCommands(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("commands does not accept arguments")
	}
	player, err := a.store.Load()
	if err != nil {
		return err
	}
	commands := player.MasteredCommands()
	if len(commands) == 0 {
		fmt.Fprintln(a.out, "No commands mastered yet. Start with 'opsquest play'.")
		return nil
	}
	fmt.Fprintf(a.out, "COMMAND MASTERY (%d)\n\n", len(commands))
	for _, command := range commands {
		fmt.Fprintf(a.out, "  %-12s used successfully %d %s\n", command, player.Commands[command], plural(player.Commands[command], "time", "times"))
	}
	return nil
}

func (a *App) runReset(args []string) error {
	flags := flag.NewFlagSet("reset", flag.ContinueOnError)
	flags.SetOutput(a.errOut)
	yes := flags.Bool("yes", false, "reset without confirmation")
	if err := flags.Parse(args); err != nil {
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
		fmt.Fprintln(a.out, "Progress reset. Welcome back, Intern.")
	} else {
		fmt.Fprintln(a.out, "No saved progress found.")
	}
	return nil
}

func (a *App) printUsage() {
	fmt.Fprintln(a.out, `OpsQuest — learn operations by fixing fictional production

Usage:
  opsquest play [MISSION]  Play the next mission or choose one by number/ID
  opsquest list            List the Linux campaign and completion status
  opsquest profile         Show rank, XP, and campaign progress
  opsquest commands        Show commands practiced successfully
  opsquest reset [--yes]   Reset local progress
  opsquest version         Print the version

Start with: opsquest play`)
}

func progressBar(value, total, width int) string {
	filled := 0
	if total > 0 {
		filled = value * width / total
	}
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func percentage(value, total int) int {
	if total == 0 {
		return 0
	}
	return value * 100 / total
}

func plural(value int, singular, plural string) string {
	if value == 1 {
		return singular
	}
	return plural
}
