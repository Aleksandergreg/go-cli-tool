package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/dockerlab"
	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
	"github.com/aleksandergregersen/opsquest/internal/ui"
)

const version = "0.3.0"

type App struct {
	in         io.Reader
	out        io.Writer
	errOut     io.Writer
	catalog    mission.Catalog
	store      profile.Store
	context    context.Context
	factory    game.Factory
	style      ui.Style
	errorStyle ui.Style
}

func New(in io.Reader, out, errOut io.Writer) (*App, error) {
	return NewWithContext(context.Background(), in, out, errOut)
}

// NewWithContext constructs the CLI with a process-lifecycle context. The
// executable uses a signal-aware context so Docker operations can stop and
// attempt cleanup before the process exits.
func NewWithContext(ctx context.Context, in io.Reader, out, errOut io.Writer) (*App, error) {
	catalog, err := mission.LoadCatalog()
	if err != nil {
		return nil, err
	}
	store, err := profile.DefaultStore()
	if err != nil {
		return nil, err
	}
	app := NewWithDependencies(in, out, errOut, catalog, store)
	if ctx != nil {
		app.context = ctx
	}
	return app, nil
}

func NewWithDependencies(in io.Reader, out, errOut io.Writer, catalog mission.Catalog, store profile.Store) *App {
	return &App{
		in:         in,
		out:        out,
		errOut:     errOut,
		catalog:    catalog,
		store:      store,
		context:    context.Background(),
		factory:    dockerlab.NewFactory(game.SandboxFactory{}),
		style:      ui.Auto(out),
		errorStyle: ui.Auto(errOut),
	}
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
	for _, item := range a.catalog.All() {
		if !player.IsComplete(item.ID) {
			continue
		}
		if item.Difficulty == "advanced" {
			unlock("boss-slayer")
		}
	}
	if game.HasCompletedCatalog(player, a.catalog) {
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
		fmt.Fprintf(a.out, "%s %s\n", a.style.Header("OpsQuest"), a.style.Accent(version))
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
		item, found = a.catalog.NextInTrack(mission.TrackLinux, player.IsComplete)
		if !found {
			fmt.Fprintln(a.out, a.style.Success("Linux campaign complete! Replay a mission or explore Docker with 'opsquest list --track docker'."))
			return nil
		}
	}
	continuous := flags.NArg() == 0
	reader := game.NewCommandLineReader(a.in, a.out)
	for {
		availability := game.EnvironmentAvailability(a.context, a.factory, item)
		if !availability.Available {
			return errors.New(availability.Detail)
		}
		currentTrack := item.EffectiveTrack()
		session := game.Session{
			Mission: item,
			Player:  &player,
			Store:   a.store,
			In:      a.in,
			Out:     a.out,
			Err:     a.errOut,
			Reader:  reader,
			Catalog: a.catalog,
			ListMissions: func(args []string) error {
				if !hasFlag(args, "--track") {
					args = append([]string{"--track", currentTrack}, args...)
				}
				return a.listMissions(args, player)
			},
			Context:    a.context,
			Factory:    a.factory,
			Style:      a.style,
			ErrorStyle: a.errorStyle,
		}
		result, err := session.Run()
		if err != nil {
			return err
		}
		if result.SwitchMission != "" {
			item, _ = a.catalog.Find(result.SwitchMission)
			continue
		}
		if !continuous || !result.Completed {
			return nil
		}

		next, found := a.catalog.NextInTrack(item.EffectiveTrack(), player.IsComplete)
		if !found {
			fmt.Fprintf(a.out, "\n%s\n", a.style.Success(fmt.Sprintf("%s track complete!", trackDisplayName(item.EffectiveTrack()))))
			return nil
		}
		fmt.Fprintf(a.out, "\n%s\n", a.style.Accent(fmt.Sprintf("→ Continuing to Mission %02d: %s", next.Number, next.Title)))
		item = next
	}
}

func (a *App) runList(args []string) error {
	player, err := a.loadPlayer()
	if err != nil {
		return err
	}
	return a.listMissions(args, player)
}

func (a *App) listMissions(args []string, player profile.Profile) error {
	flags := flag.NewFlagSet("list", flag.ContinueOnError)
	flags.SetOutput(a.errOut)
	completedOnly := flags.Bool("completed", false, "show completed missions only")
	remainingOnly := flags.Bool("remaining", false, "show incomplete missions only")
	campaign := flags.String("campaign", "", "filter by campaign name")
	track := flags.String("track", mission.TrackLinux, "filter by track: linux, docker, or all")
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
	trackProvided := false
	flags.Visit(func(item *flag.Flag) {
		if item.Name == "track" {
			trackProvided = true
		}
	})
	selectedTrack := strings.ToLower(strings.TrimSpace(*track))
	if *campaign != "" && !trackProvided {
		selectedTrack = "all"
	}
	if selectedTrack != mission.TrackLinux && selectedTrack != mission.TrackDocker && selectedTrack != "all" {
		return fmt.Errorf("unknown track %q; use linux, docker, or all", *track)
	}
	fmt.Fprintln(a.out, a.style.Header(trackHeading(selectedTrack)))
	lastCampaign := ""
	shown := 0
	for _, item := range a.catalog.All() {
		if selectedTrack != "all" && item.EffectiveTrack() != selectedTrack {
			continue
		}
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
			fmt.Fprintf(a.out, "%s\n", a.style.Accent(item.Campaign))
			lastCampaign = item.Campaign
		}
		status := "○"
		if isComplete {
			status = a.style.Success("✓")
		} else {
			status = a.style.Muted(status)
		}
		title := fmt.Sprintf("%-38s", item.Title)
		difficulty := a.style.Difficulty(fmt.Sprintf("%-12s", item.Difficulty))
		reward := a.style.Reward(fmt.Sprintf("%3d XP", item.Rewards.XP))
		fmt.Fprintf(a.out, "  %s %02d  %s %s %s  %s\n", status, item.Number, title, difficulty, reward, a.style.Muted(item.ID))
		shown++
	}
	if shown == 0 {
		fmt.Fprintln(a.out, "  No missions match these filters.")
	}
	completed := 0
	total := 0
	for _, item := range a.catalog.All() {
		if selectedTrack != "all" && item.EffectiveTrack() != selectedTrack {
			continue
		}
		total++
		if player.IsComplete(item.ID) {
			completed++
		}
	}
	fmt.Fprintf(a.out, "\n%s\n", a.style.Accent(fmt.Sprintf("%d/%d missions complete · %d XP", completed, total, player.XP)))
	if selectedTrack == mission.TrackDocker {
		if item, found := firstMissionInTrack(a.catalog, mission.TrackDocker); found {
			availability := game.EnvironmentAvailability(a.context, a.factory, item)
			if availability.Available {
				fmt.Fprintln(a.out, a.style.Success("Docker labs ready."))
			} else {
				fmt.Fprintln(a.out, a.style.Warning(availability.Detail))
			}
		}
	}
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
		fmt.Fprintln(a.out, a.style.Success("Profile name updated."))
	}
	completed := 0
	type campaignProgress struct {
		name      string
		track     string
		completed int
		total     int
	}
	campaigns := make([]campaignProgress, 0)
	campaignIndex := make(map[string]int)
	for _, item := range a.catalog.All() {
		key := item.EffectiveTrack() + "\x00" + item.Campaign
		index, exists := campaignIndex[key]
		if !exists {
			index = len(campaigns)
			campaignIndex[key] = index
			campaigns = append(campaigns, campaignProgress{name: item.Campaign, track: item.EffectiveTrack()})
		}
		campaigns[index].total++
		if player.IsComplete(item.ID) {
			completed++
			campaigns[index].completed++
		}
	}
	trackProgress := func(track string) (int, int) {
		done, total := 0, 0
		for _, campaign := range campaigns {
			if campaign.track == track {
				done += campaign.completed
				total += campaign.total
			}
		}
		return done, total
	}
	linuxCompleted, linuxTotal := trackProgress(mission.TrackLinux)
	dockerCompleted, dockerTotal := trackProgress(mission.TrackDocker)
	fmt.Fprintf(a.out, "%s %s\n", a.style.Accent("Operator:"), player.Name)
	fmt.Fprintf(a.out, "%s %s\n", a.style.Accent("Rank:"), player.Rank())
	fmt.Fprintf(a.out, "%s %d · %s\n", a.style.Accent("Level:"), player.Level(), a.style.Reward(fmt.Sprintf("%d XP", player.XP)))
	if nextRank, needed, exists := player.NextRank(); exists {
		fmt.Fprintf(a.out, "%s %s in %s\n", a.style.Muted("Next rank:"), nextRank, a.style.Reward(fmt.Sprintf("%d XP", needed)))
	}
	fmt.Fprintln(a.out)
	fmt.Fprintf(a.out, "Linux  %s %3d%%\n", styledProgressBar(a.style, linuxCompleted, linuxTotal, 20), percentage(linuxCompleted, linuxTotal))
	for _, campaign := range campaigns {
		if campaign.track != mission.TrackLinux {
			continue
		}
		fmt.Fprintf(a.out, "  %-19s %s %3d%%\n", campaign.name, styledProgressBar(a.style, campaign.completed, campaign.total, 10), percentage(campaign.completed, campaign.total))
	}
	dockerState := "unavailable"
	if item, found := firstMissionInTrack(a.catalog, mission.TrackDocker); found {
		if game.EnvironmentAvailability(a.context, a.factory, item).Available {
			dockerState = "ready"
		}
	}
	fmt.Fprintf(a.out, "Docker %s %3d%%  %s\n", styledProgressBar(a.style, dockerCompleted, dockerTotal, 20), percentage(dockerCompleted, dockerTotal), dockerState)
	for _, campaign := range campaigns {
		if campaign.track != mission.TrackDocker {
			continue
		}
		fmt.Fprintf(a.out, "  %-19s %s %3d%%\n", campaign.name, styledProgressBar(a.style, campaign.completed, campaign.total, 10), percentage(campaign.completed, campaign.total))
	}
	fmt.Fprintf(a.out, "%s\n\n", a.style.Muted(fmt.Sprintf("K8s    %s  locked", progressBar(0, 20, 20))))
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
	fmt.Fprintf(a.out, "%s\n\n", a.style.Header(fmt.Sprintf("COMMAND MASTERY (%d)", len(commands))))
	for _, command := range commands {
		name := fmt.Sprintf("%-12s", command)
		fmt.Fprintf(a.out, "  %s used successfully %d %s\n", a.style.Accent(name), player.Commands[command], plural(player.Commands[command], "time", "times"))
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
	fmt.Fprintln(a.out, a.style.Header("ACHIEVEMENTS"))
	for _, achievement := range profile.AchievementDefinitions() {
		unlockedAt, unlocked := player.Unlocked[achievement.ID]
		if unlocked {
			title := fmt.Sprintf("%-22s", achievement.Title)
			fmt.Fprintf(a.out, "  %s %s %s  %s\n", a.style.Achievement("★"), a.style.Achievement(title), achievement.Description, a.style.Muted("["+unlockedAt.Local().Format("2006-01-02")+"]"))
		} else {
			fmt.Fprintf(a.out, "  %s\n", a.style.Muted(fmt.Sprintf("☆ %-22s %s", achievement.Title, achievement.Description)))
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
		item, found = a.catalog.NextInTrack(mission.TrackLinux, player.IsComplete)
		if !found {
			items := a.catalog.InTrack(mission.TrackLinux)
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
	fmt.Fprintln(a.out, a.style.Header(fmt.Sprintf("MISSION %02d: %s", item.Number, item.Title)))
	fmt.Fprintf(a.out, "Track: %s · Campaign: %s · Difficulty: %s · Reward: %s\n", trackDisplayName(item.EffectiveTrack()), a.style.Accent(item.Campaign), a.style.Difficulty(item.Difficulty), a.style.Reward(fmt.Sprintf("%d XP", item.Rewards.XP)))
	styledStatus := a.style.Muted(status)
	if status == "completed" {
		styledStatus = a.style.Success(status)
	}
	fmt.Fprintf(a.out, "Status: %s · Outcome checks: %d · Hints available: %d\n\n", styledStatus, len(item.Validation.All), len(item.Hints))
	fmt.Fprintf(a.out, "%s\n\n%s\n", item.Story, item.Objective)
	if item.EffectiveEnvironment() == mission.EnvironmentDocker {
		availability := game.EnvironmentAvailability(a.context, a.factory, item)
		fmt.Fprintln(a.out)
		if availability.Available {
			fmt.Fprintln(a.out, a.style.Success("Docker lab ready."))
		} else {
			fmt.Fprintln(a.out, a.style.Warning(availability.Detail))
		}
	}
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
	fmt.Fprintln(a.out, a.style.Header("OpsQuest diagnostics"))
	check := a.style.Success("✓")
	fmt.Fprintf(a.out, "  %s embedded catalog: %d missions (%d Linux, %d Docker)\n", check, len(a.catalog.All()), len(a.catalog.InTrack(mission.TrackLinux)), len(a.catalog.InTrack(mission.TrackDocker)))
	fmt.Fprintf(a.out, "  %s profile: version %d, %d completed missions\n", check, player.Version, len(player.Completed))
	fmt.Fprintf(a.out, "  %s profile path: %s\n", check, a.store.Path())
	fmt.Fprintf(a.out, "  %s Linux labs: in-memory; no host shell or filesystem access\n", check)
	if item, found := firstMissionInTrack(a.catalog, mission.TrackDocker); found {
		availability := game.EnvironmentAvailability(a.context, a.factory, item)
		if availability.Available {
			fmt.Fprintf(a.out, "  %s docker labs: ready · %s\n", check, availability.Detail)
		} else {
			fmt.Fprintf(a.out, "  %s docker labs: unavailable · %s\n", a.style.Warning("!"), availability.Detail)
		}
	}
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
		fmt.Fprintln(a.out, a.style.Success("Progress reset. Welcome back, Intern."))
	} else {
		fmt.Fprintln(a.out, a.style.Muted("No saved progress found."))
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
	fmt.Fprintln(out, "Usage: opsquest list [--completed|--remaining] [--campaign NAME] [--track linux|docker|all]")
}

func (a *App) printProfileUsage(out io.Writer) {
	fmt.Fprintln(out, "Usage: opsquest profile [--name NAME]")
}

func (a *App) printUsage() {
	fmt.Fprintln(a.out, a.style.Header("OpsQuest")+" — learn operations by fixing fictional production")
	fmt.Fprintln(a.out)
	fmt.Fprintln(a.out, `Usage:
  opsquest play [MISSION]  Continue the campaign or play one selected mission
  opsquest list            List missions by track and completion status
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
	filled := progressFilled(value, total, width)
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func styledProgressBar(style ui.Style, value, total, width int) string {
	filled := progressFilled(value, total, width)
	return style.Progress(strings.Repeat("█", filled), strings.Repeat("░", width-filled))
}

func progressFilled(value, total, width int) int {
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
	return filled
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

func hasFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == name || strings.HasPrefix(arg, name+"=") {
			return true
		}
	}
	return false
}

func firstMissionInTrack(catalog mission.Catalog, track string) (mission.Mission, bool) {
	items := catalog.InTrack(track)
	if len(items) == 0 {
		return mission.Mission{}, false
	}
	return items[0], true
}

func trackDisplayName(track string) string {
	switch track {
	case mission.TrackDocker:
		return "Docker"
	case mission.TrackLinux:
		return "Linux"
	default:
		return track
	}
}

func trackHeading(track string) string {
	switch track {
	case mission.TrackDocker:
		return "DOCKER LABS"
	case "all":
		return "ALL MISSIONS"
	default:
		return "LINUX CAMPAIGN"
	}
}
