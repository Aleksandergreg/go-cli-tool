package cli

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
)

const version = "0.2.0"

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

func (a *App) loadPlayer() (profile.Profile, error) {
	player, err := a.store.Load()
	if err != nil {
		return profile.Profile{}, err
	}
	now := time.Now()
	changed := false
	unlock := func(id string) {
		if _, added := player.UnlockAchievement(id, now); added {
			changed = true
		}
	}
	if len(player.Completed) > 0 {
		unlock("first-fix")
	}
	if len(player.Commands) >= 10 {
		unlock("command-collector")
	}
	if player.HintFreeCompletions() >= 5 {
		unlock("self-reliant")
	}
	knownCompleted := 0
	for _, item := range a.catalog.All() {
		if !player.IsComplete(item.ID) {
			continue
		}
		knownCompleted++
		if item.Difficulty == "advanced" {
			unlock("boss-slayer")
		}
	}
	if knownCompleted == len(a.catalog.All()) && knownCompleted > 0 {
		unlock("linux-completionist")
	}
	if changed {
		if err := a.store.Save(player); err != nil {
			return profile.Profile{}, err
		}
	}
	return player, nil
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
	case "achievements":
		return a.runAchievements(args[1:])
	case "show", "mission":
		return a.runShow(args[1:])
	case "doctor":
		return a.runDoctor(args[1:])
	case "reset":
		return a.runReset(args[1:])
	case "version", "--version", "-v":
		if len(args) > 1 {
			return fmt.Errorf("version does not accept arguments")
		}
		fmt.Fprintf(a.out, "OpsQuest %s\n", version)
		return nil
	case "help", "--help", "-h":
		return a.runHelp(args[1:])
	default:
		return fmt.Errorf("unknown command %q; run 'opsquest help'", args[0])
	}
}

func (a *App) runPlay(args []string) error {
	flags := flag.NewFlagSet("play", flag.ContinueOnError)
	flags.SetOutput(a.errOut)
	flags.Usage = func() { a.printPlayUsage(a.errOut) }
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() > 1 {
		return fmt.Errorf("usage: opsquest play [MISSION]")
	}
	player, err := a.loadPlayer()
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
	continuous := flags.NArg() == 0
	reader := game.NewCommandLineReader(a.in, a.out)
	for {
		session := game.Session{
			Mission:      item,
			MissionCount: len(a.catalog.All()),
			Player:       &player,
			Store:        a.store,
			In:           a.in,
			Out:          a.out,
			Err:          a.errOut,
			Reader:       reader,
		}
		result, err := session.Run()
		if err != nil {
			return err
		}
		if !continuous || !result.Completed {
			return nil
		}

		next, found := a.catalog.Next(player.IsComplete)
		if !found {
			fmt.Fprintln(a.out, "\nCampaign complete! Every Linux mission is now complete.")
			return nil
		}
		fmt.Fprintf(a.out, "\n→ Continuing to Mission %02d: %s\n", next.Number, next.Title)
		item = next
	}
}

func (a *App) runList(args []string) error {
	flags := flag.NewFlagSet("list", flag.ContinueOnError)
	flags.SetOutput(a.errOut)
	completedOnly := flags.Bool("completed", false, "show completed missions only")
	remainingOnly := flags.Bool("remaining", false, "show incomplete missions only")
	campaign := flags.String("campaign", "", "filter by campaign name")
	flags.Usage = func() { a.printListUsage(a.errOut) }
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("list does not accept positional arguments")
	}
	if *completedOnly && *remainingOnly {
		return fmt.Errorf("--completed and --remaining cannot be combined")
	}
	player, err := a.loadPlayer()
	if err != nil {
		return err
	}
	fmt.Fprintln(a.out, "LINUX CAMPAIGN")
	lastCampaign := ""
	shown := 0
	for _, item := range a.catalog.All() {
		isComplete := player.IsComplete(item.ID)
		if *completedOnly && !isComplete || *remainingOnly && isComplete {
			continue
		}
		if *campaign != "" && !strings.EqualFold(item.Campaign, *campaign) {
			continue
		}
		if item.Campaign != lastCampaign {
			if lastCampaign != "" {
				fmt.Fprintln(a.out)
			}
			fmt.Fprintf(a.out, "%s\n", item.Campaign)
			lastCampaign = item.Campaign
		}
		status := "○"
		if isComplete {
			status = "✓"
		}
		fmt.Fprintf(a.out, "  %s %02d  %-38s %-12s %3d XP  %s\n", status, item.Number, item.Title, item.Difficulty, item.Rewards.XP, item.ID)
		shown++
	}
	if shown == 0 {
		fmt.Fprintln(a.out, "  No missions match these filters.")
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
	flags := flag.NewFlagSet("profile", flag.ContinueOnError)
	flags.SetOutput(a.errOut)
	name := flags.String("name", "", "update the operator display name")
	flags.Usage = func() { a.printProfileUsage(a.errOut) }
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("profile does not accept positional arguments")
	}
	nameProvided := false
	flags.Visit(func(item *flag.Flag) {
		if item.Name == "name" {
			nameProvided = true
		}
	})
	player, err := a.loadPlayer()
	if err != nil {
		return err
	}
	if nameProvided {
		player.Name = strings.TrimSpace(*name)
		if player.Name == "" {
			return fmt.Errorf("profile name cannot be blank")
		}
		if err := a.store.Save(player); err != nil {
			return err
		}
		fmt.Fprintln(a.out, "Profile name updated.")
	}
	total := len(a.catalog.All())
	completed := 0
	type campaignProgress struct {
		name      string
		completed int
		total     int
	}
	campaigns := make([]campaignProgress, 0)
	campaignIndex := make(map[string]int)
	for _, item := range a.catalog.All() {
		index, exists := campaignIndex[item.Campaign]
		if !exists {
			index = len(campaigns)
			campaignIndex[item.Campaign] = index
			campaigns = append(campaigns, campaignProgress{name: item.Campaign})
		}
		campaigns[index].total++
		if player.IsComplete(item.ID) {
			completed++
			campaigns[index].completed++
		}
	}
	fmt.Fprintf(a.out, "Operator: %s\n", player.Name)
	fmt.Fprintf(a.out, "Rank: %s\n", player.Rank())
	fmt.Fprintf(a.out, "Level: %d · %d XP\n", player.Level(), player.XP)
	if nextRank, needed, exists := player.NextRank(); exists {
		fmt.Fprintf(a.out, "Next rank: %s in %d XP\n", nextRank, needed)
	}
	fmt.Fprintln(a.out)
	fmt.Fprintf(a.out, "Linux  %s %3d%%\n", progressBar(completed, total, 20), percentage(completed, total))
	for _, campaign := range campaigns {
		fmt.Fprintf(a.out, "  %-19s %s %3d%%\n", campaign.name, progressBar(campaign.completed, campaign.total, 10), percentage(campaign.completed, campaign.total))
	}
	fmt.Fprintf(a.out, "Docker %s  locked\n", progressBar(0, 20, 20))
	fmt.Fprintf(a.out, "K8s    %s  locked\n\n", progressBar(0, 20, 20))
	fmt.Fprintf(a.out, "Commands mastered: %d\n", len(player.Commands))
	fmt.Fprintf(a.out, "Missions completed: %d\n", completed)
	fmt.Fprintf(a.out, "Hints used: %d\n", player.HintsUsed())
	fmt.Fprintf(a.out, "Achievements: %d/%d\n", player.AchievementCount(), len(profile.AchievementDefinitions()))
	return nil
}

func (a *App) runCommands(args []string) error {
	flags := flag.NewFlagSet("commands", flag.ContinueOnError)
	flags.SetOutput(a.errOut)
	flags.Usage = func() { fmt.Fprintln(a.errOut, "Usage: opsquest commands") }
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("commands does not accept positional arguments")
	}
	player, err := a.loadPlayer()
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

func (a *App) runAchievements(args []string) error {
	flags := flag.NewFlagSet("achievements", flag.ContinueOnError)
	flags.SetOutput(a.errOut)
	flags.Usage = func() { fmt.Fprintln(a.errOut, "Usage: opsquest achievements") }
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("achievements does not accept positional arguments")
	}
	player, err := a.loadPlayer()
	if err != nil {
		return err
	}
	fmt.Fprintln(a.out, "ACHIEVEMENTS")
	for _, achievement := range profile.AchievementDefinitions() {
		unlockedAt, unlocked := player.Unlocked[achievement.ID]
		if unlocked {
			fmt.Fprintf(a.out, "  ★ %-22s %s  [%s]\n", achievement.Title, achievement.Description, unlockedAt.Local().Format("2006-01-02"))
		} else {
			fmt.Fprintf(a.out, "  ☆ %-22s %s\n", achievement.Title, achievement.Description)
		}
	}
	fmt.Fprintf(a.out, "\n%d/%d unlocked\n", player.AchievementCount(), len(profile.AchievementDefinitions()))
	return nil
}

func (a *App) runShow(args []string) error {
	flags := flag.NewFlagSet("show", flag.ContinueOnError)
	flags.SetOutput(a.errOut)
	flags.Usage = func() { fmt.Fprintln(a.errOut, "Usage: opsquest show [MISSION]") }
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() > 1 {
		return fmt.Errorf("usage: opsquest show [MISSION]")
	}
	player, err := a.loadPlayer()
	if err != nil {
		return err
	}
	var item mission.Mission
	var found bool
	if flags.NArg() == 1 {
		item, found = a.catalog.Find(flags.Arg(0))
		if !found {
			return fmt.Errorf("mission %q not found; run 'opsquest list'", flags.Arg(0))
		}
	} else {
		item, found = a.catalog.Next(player.IsComplete)
		if !found {
			items := a.catalog.All()
			if len(items) > 0 {
				item, found = items[len(items)-1], true
			}
		}
	}
	if !found {
		return fmt.Errorf("no missions are available")
	}
	status := "not completed"
	if player.IsComplete(item.ID) {
		status = "completed"
	}
	fmt.Fprintf(a.out, "MISSION %02d: %s\n", item.Number, item.Title)
	fmt.Fprintf(a.out, "Campaign: %s · Difficulty: %s · Reward: %d XP\n", item.Campaign, item.Difficulty, item.Rewards.XP)
	fmt.Fprintf(a.out, "Status: %s · Outcome checks: %d · Hints available: %d\n\n", status, len(item.Validation.All), len(item.Hints))
	fmt.Fprintf(a.out, "%s\n\n%s\n", item.Story, item.Objective)
	fmt.Fprintf(a.out, "\nPlay with: opsquest play %d\n", item.Number)
	return nil
}

func (a *App) runDoctor(args []string) error {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(a.errOut)
	flags.Usage = func() { fmt.Fprintln(a.errOut, "Usage: opsquest doctor") }
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("doctor does not accept positional arguments")
	}
	player, err := a.loadPlayer()
	if err != nil {
		return fmt.Errorf("profile check failed: %w", err)
	}
	fmt.Fprintln(a.out, "OpsQuest diagnostics")
	fmt.Fprintf(a.out, "  ✓ embedded catalog: %d missions\n", len(a.catalog.All()))
	fmt.Fprintf(a.out, "  ✓ profile: version %d, %d completed missions\n", player.Version, len(player.Completed))
	fmt.Fprintf(a.out, "  ✓ profile path: %s\n", a.store.Path())
	fmt.Fprintln(a.out, "  ✓ sandbox: in-memory; host command execution disabled")
	return nil
}

func (a *App) runReset(args []string) error {
	flags := flag.NewFlagSet("reset", flag.ContinueOnError)
	flags.SetOutput(a.errOut)
	yes := flags.Bool("yes", false, "reset without confirmation")
	flags.Usage = func() { fmt.Fprintln(a.errOut, "Usage: opsquest reset [--yes]") }
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
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
	case "list", "campaign":
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
		fmt.Fprintln(a.out, "Usage: opsquest doctor")
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
	fmt.Fprintln(out, "Usage: opsquest play [MISSION]")
	fmt.Fprintln(out, "Without MISSION, continue through incomplete missions until you quit.")
	fmt.Fprintln(out, "With a mission number or ID, play only that selected mission.")
}

func (a *App) printListUsage(out io.Writer) {
	fmt.Fprintln(out, "Usage: opsquest list [--completed|--remaining] [--campaign NAME]")
}

func (a *App) printProfileUsage(out io.Writer) {
	fmt.Fprintln(out, "Usage: opsquest profile [--name NAME]")
}

func (a *App) printUsage() {
	fmt.Fprintln(a.out, `OpsQuest — learn operations by fixing fictional production

Usage:
  opsquest play [MISSION]  Play the next mission or choose one by number/ID
  opsquest list            List the Linux campaign and completion status
  opsquest profile         Show rank, XP, and campaign progress
  opsquest commands        Show commands practiced successfully
  opsquest achievements    Show learning achievements
  opsquest show [MISSION]  Preview a mission without starting it
  opsquest doctor          Check the catalog, profile, and safety mode
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
