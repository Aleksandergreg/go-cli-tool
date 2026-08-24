package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/buildinfo"
	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
	"github.com/aleksandergregersen/opsquest/internal/ui"
	"github.com/aleksandergregersen/opsquest/internal/webapp"
)

type App struct {
	in             io.Reader
	out            io.Writer
	errOut         io.Writer
	catalog        mission.Catalog
	store          profile.Store
	ctx            context.Context
	factory        game.Factory
	startCompanion CompanionStarter
	style          ui.Style
	errorStyle     ui.Style
}

type playRouteKind uint8

const (
	playRouteRecommended playRouteKind = iota
	playRouteSequential
	playRouteWorld
)

type playRoute struct {
	kind        playRouteKind
	worldNumber int
}

// Companion is the lifecycle and presentation boundary used by web-assisted
// play. It receives sanitized game snapshots and never accepts player command
// text or completion decisions.
type Companion interface {
	game.AttemptReporter
	URL() string
	Close(context.Context) error
}

// CompanionStarter creates one companion for the lifetime of a play command.
type CompanionStarter func(context.Context) (Companion, error)

// Config contains the process-level dependencies required by the CLI. The
// executable supplies persistent storage and the combined environment factory;
// focused tests may omit Context, Factory, and StartCompanion to use safe local
// defaults.
type Config struct {
	Context        context.Context
	In             io.Reader
	Out            io.Writer
	ErrOut         io.Writer
	Catalog        mission.Catalog
	Store          profile.Store
	Factory        game.Factory
	StartCompanion CompanionStarter
}

func New(config Config) *App {
	if config.In == nil {
		config.In = strings.NewReader("")
	}
	if config.Out == nil {
		config.Out = io.Discard
	}
	if config.ErrOut == nil {
		config.ErrOut = io.Discard
	}
	if config.Context == nil {
		config.Context = context.Background()
	}
	if config.Factory == nil {
		config.Factory = game.SandboxFactory{}
	}
	if config.StartCompanion == nil {
		config.StartCompanion = func(ctx context.Context) (Companion, error) {
			return webapp.Start(ctx)
		}
	}
	return &App{
		in:             config.In,
		out:            config.Out,
		errOut:         config.ErrOut,
		catalog:        config.Catalog,
		store:          config.Store,
		ctx:            config.Context,
		factory:        config.Factory,
		startCompanion: config.StartCompanion,
		style:          ui.Auto(config.Out),
		errorStyle:     ui.Auto(config.ErrOut),
	}
}

func (a *App) loadPlayer() (profile.Profile, error) {
	player, err := a.store.Load()
	if err != nil {
		return profile.Profile{}, err
	}
	if unlocked := game.ReconcileAchievements(&player, a.catalog, time.Now()); len(unlocked) > 0 {
		if err := a.store.Save(player); err != nil {
			return profile.Profile{}, err
		}
	}
	return player, nil
}

func (a *App) newFlagSet(name string, usage func()) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(a.errOut)
	flags.Usage = usage
	return flags
}

func parseFlags(flags *flag.FlagSet, args []string) (bool, error) {
	err := flags.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		return true, nil
	}
	return false, err
}

func flagProvided(flags *flag.FlagSet, name string) bool {
	provided := false
	flags.Visit(func(item *flag.Flag) { provided = provided || item.Name == name })
	return provided
}

func (a *App) Run(args []string) error {
	if len(args) == 0 {
		a.printUsage()
		return nil
	}
	switch args[0] {
	case "play":
		return a.runPlay(args[1:])
	case "guide", "tutorial":
		return a.runGuide(args[1:])
	case "list", "campaign", "map", "worlds":
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
		fmt.Fprintf(a.out, "%s %s\n", a.style.Header("OpsQuest"), a.style.Accent(buildinfo.Version))
		return nil
	case "help", "--help", "-h":
		return a.runHelp(args[1:])
	default:
		return fmt.Errorf("unknown command %q; run 'opsquest help'", args[0])
	}
}

func (a *App) runPlay(args []string) (returnErr error) {
	flags := a.newFlagSet("play", func() { a.printPlayUsage(a.errOut) })
	track := flags.String("track", mission.TrackLinux, "play the linux or docker track")
	worldNumber := flags.Int("world", 0, "start the next incomplete stage in a world")
	once := flags.Bool("once", false, "return after one completed mission")
	web := flags.Bool("web", false, "show mission guidance in a local browser companion")
	if help, err := parseFlags(flags, args); help || err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return fmt.Errorf("usage: opsquest play [--track linux|docker] [--world NUMBER] [--once] [--web] [MISSION]")
	}
	trackProvided, worldProvided := flagProvided(flags, "track"), flagProvided(flags, "world")
	selectedTrack := strings.ToLower(strings.TrimSpace(*track))
	if selectedTrack != mission.TrackLinux && selectedTrack != mission.TrackDocker {
		return fmt.Errorf("unknown track %q; use linux or docker", *track)
	}
	if worldProvided && *worldNumber < 1 {
		return fmt.Errorf("world number must be positive")
	}
	if flags.NArg() == 1 && (worldProvided || trackProvided) {
		return fmt.Errorf("MISSION cannot be combined with --track or --world")
	}
	player, err := a.loadPlayer()
	if err != nil {
		return err
	}
	var item mission.Mission
	route := playRoute{kind: playRouteRecommended}
	if flags.NArg() == 1 {
		var found bool
		item, found = a.catalog.Find(flags.Arg(0))
		if !found {
			return fmt.Errorf("mission %q not found; run 'opsquest list'", flags.Arg(0))
		}
		route.kind = playRouteSequential
	} else if *worldNumber > 0 {
		route = playRoute{kind: playRouteWorld, worldNumber: *worldNumber}
		var found bool
		item, found = a.catalog.NextInWorld(selectedTrack, *worldNumber, player.IsComplete)
		if !found {
			if world, exists := a.catalog.World(selectedTrack, *worldNumber); exists && len(world.Missions) > 0 {
				item, found = world.Missions[0], true
				fmt.Fprintf(a.out, "%s\n", a.style.Accent(fmt.Sprintf("World %d is complete; replaying Stage 1.", *worldNumber)))
			}
		}
		if !found {
			return fmt.Errorf("world %d does not exist in the %s track; run 'opsquest map --track %s'", *worldNumber, selectedTrack, selectedTrack)
		}
	} else {
		var found bool
		item, found = a.catalog.NextInTrack(selectedTrack, player.IsComplete)
		if !found {
			fmt.Fprintf(a.out, "%s\n", a.style.Success(fmt.Sprintf("%s track complete! Replay a mission or inspect other worlds with 'opsquest map'.", trackDisplayName(selectedTrack))))
			return nil
		}
	}
	var activeCompanion Companion
	if *web {
		activeCompanion, err = a.startCompanion(a.ctx)
		if err != nil {
			return fmt.Errorf("start web companion: %w", err)
		}
		defer func() {
			closeContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := activeCompanion.Close(closeContext); err != nil {
				returnErr = errors.Join(returnErr, fmt.Errorf("close web companion: %w", err))
			}
		}()
		fmt.Fprintln(a.out, a.style.Header("WEB MISSION COMPANION"))
		fmt.Fprintf(a.out, "Open this one-time local URL in your browser:\n%s\n", a.style.Accent(activeCompanion.URL()))
		fmt.Fprintln(a.out, a.style.Muted("Keep this terminal for commands. The browser can display progress but cannot execute anything."))
	}
	if item.EffectiveTrack() == mission.TrackLinux && isPristineProfile(player) {
		if !*web {
			a.printQuickStart()
		}
		player.Onboarded = true
		if err := a.store.Save(player); err != nil {
			return err
		}
	}
	continuous := !*once
	reader := game.NewCommandLineReader(a.in, a.out)
	for {
		currentTrack := item.EffectiveTrack()
		wasComplete := player.IsComplete(item.ID)
		session := game.Session{
			Mission:   item,
			Player:    &player,
			Saver:     a.store,
			Reporter:  activeCompanion,
			Companion: activeCompanion != nil,
			In:        a.in,
			Out:       a.out,
			ErrOut:    a.errOut,
			Reader:    reader,
			Catalog:   a.catalog,
			ListMissions: func(args []string) error {
				if !hasFlag(args, "--track") {
					args = append([]string{"--track", currentTrack}, args...)
				}
				return a.listMissions(args, player, true)
			},
			Context:    a.ctx,
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
			if result.WorldRoute > 0 {
				route = playRoute{kind: playRouteWorld, worldNumber: result.WorldRoute}
			} else {
				route = playRoute{kind: playRouteSequential}
			}
			continue
		}
		if result.WorldRoute > 0 {
			route = playRoute{kind: playRouteWorld, worldNumber: result.WorldRoute}
		}
		if !result.Completed {
			return nil
		}
		if !continuous {
			a.printNextRecommendation(item, player)
			return nil
		}

		next, found := a.nextOnRoute(route, item, player)
		if !found {
			a.printRouteFinished(route, item, player)
			return nil
		}
		if current, ok := a.catalog.Placement(item.ID); ok {
			if upcoming, ok := a.catalog.Placement(next.ID); ok && upcoming.WorldNumber != current.WorldNumber && !wasComplete && a.worldComplete(current, player) {
				fmt.Fprintf(a.out, "\n%s\n", a.style.Success(fmt.Sprintf("✓ World %d complete: %s", current.WorldNumber, current.WorldName)))
				transition := "Entering"
				if world, found := a.catalog.World(upcoming.Track, upcoming.WorldNumber); found {
					for _, stage := range world.Missions {
						if player.IsComplete(stage.ID) {
							transition = "Continuing in"
							break
						}
					}
				}
				fmt.Fprintln(a.out, a.style.World(fmt.Sprintf("%s World %d: %s", transition, upcoming.WorldNumber, upcoming.WorldName)))
			}
		}
		fmt.Fprintf(a.out, "\n%s\n", a.style.Accent(fmt.Sprintf("→ Continuing to Mission %02d: %s", next.Number, next.Title)))
		item = next
	}
}

func (a *App) nextOnRoute(route playRoute, current mission.Mission, player profile.Profile) (mission.Mission, bool) {
	switch route.kind {
	case playRouteSequential:
		return a.catalog.AdjacentInTrack(current.ID, 1)
	case playRouteWorld:
		next, found := a.catalog.AdjacentInTrack(current.ID, 1)
		if !found {
			return mission.Mission{}, false
		}
		placement, found := a.catalog.Placement(next.ID)
		if !found || placement.Track != current.EffectiveTrack() || placement.WorldNumber != route.worldNumber {
			return mission.Mission{}, false
		}
		return next, true
	default:
		return a.catalog.NextInTrack(current.EffectiveTrack(), player.IsComplete)
	}
}

func (a *App) printRouteFinished(route playRoute, current mission.Mission, player profile.Profile) {
	if route.kind == playRouteWorld {
		if placement, found := a.catalog.Placement(current.ID); found && a.worldComplete(placement, player) {
			fmt.Fprintf(a.out, "\n%s\n", a.style.Success(fmt.Sprintf("World %d complete: %s!", placement.WorldNumber, placement.WorldName)))
			return
		}
		fmt.Fprintf(a.out, "\n%s\n", a.style.Accent(fmt.Sprintf("Reached the end of World %d; unfinished stages remain. Resume with 'opsquest play --world %d'.", route.worldNumber, route.worldNumber)))
		return
	}

	if _, remaining := a.catalog.NextInTrack(current.EffectiveTrack(), player.IsComplete); remaining {
		fmt.Fprintf(a.out, "\n%s\n", a.style.Accent("Reached the end of the selected route. Resume unfinished missions with 'opsquest play'."))
		return
	}
	fmt.Fprintf(a.out, "\n%s\n", a.style.Success(fmt.Sprintf("%s track complete!", trackDisplayName(current.EffectiveTrack()))))
}

func (a *App) worldComplete(placement mission.Placement, player profile.Profile) bool {
	world, found := a.catalog.World(placement.Track, placement.WorldNumber)
	return found && completedMissions(world.Missions, player) == len(world.Missions)
}

func (a *App) printNextRecommendation(completed mission.Mission, player profile.Profile) {
	next, found := a.catalog.NextInTrack(completed.EffectiveTrack(), player.IsComplete)
	if !found {
		fmt.Fprintf(a.out, "\n%s\n", a.style.Success(fmt.Sprintf("%s track complete!", trackDisplayName(completed.EffectiveTrack()))))
		return
	}
	placement, _ := a.catalog.Placement(next.ID)
	fmt.Fprintf(a.out, "\n%s\n", a.style.Section("NEXT RECOMMENDED"))
	fmt.Fprintf(a.out, "Mission %02d: %s · World %d, Stage %d/%d\n", next.Number, next.Title, placement.WorldNumber, placement.StageNumber, placement.StageTotal)
	fmt.Fprintf(a.out, "Continue with %s, or jump anywhere with %s.\n", a.style.Accent("opsquest play"), a.style.Accent("opsquest map"))
}

func (a *App) runList(args []string) error {
	player, err := a.loadPlayer()
	if err != nil {
		return err
	}
	return a.listMissions(args, player, false)
}

func (a *App) listMissions(args []string, player profile.Profile, inMission bool) error {
	flags := a.newFlagSet("list", func() { a.printListUsage(a.errOut) })
	completedOnly := flags.Bool("completed", false, "show completed missions only")
	remainingOnly := flags.Bool("remaining", false, "show incomplete missions only")
	showIDs := flags.Bool("ids", false, "show stable mission IDs")
	campaign := flags.String("campaign", "", "filter by campaign name")
	track := flags.String("track", mission.TrackLinux, "filter by track: linux, docker, or all")
	if help, err := parseFlags(flags, args); help || err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("list does not accept positional arguments")
	}
	if *completedOnly && *remainingOnly {
		return fmt.Errorf("--completed and --remaining cannot be combined")
	}
	trackProvided := flagProvided(flags, "track")
	selectedTrack := strings.ToLower(strings.TrimSpace(*track))
	if *campaign != "" && !trackProvided {
		selectedTrack = "all"
	}
	if selectedTrack != mission.TrackLinux && selectedTrack != mission.TrackDocker && selectedTrack != "all" {
		return fmt.Errorf("unknown track %q; use linux, docker, or all", *track)
	}
	matchesScope := func(item mission.Mission) bool {
		if selectedTrack != "all" && item.EffectiveTrack() != selectedTrack {
			return false
		}
		return *campaign == "" || strings.EqualFold(item.Campaign, *campaign)
	}
	heading := trackHeading(selectedTrack)
	if strings.TrimSpace(*campaign) != "" {
		heading = "WORLD · " + *campaign
	}
	fmt.Fprintln(a.out, a.style.Header(heading))
	shown := 0
	displayedTracks := make(map[string]bool)
	tracks := []string{selectedTrack}
	if selectedTrack == "all" {
		tracks = []string{mission.TrackLinux, mission.TrackDocker}
	}
	printedWorld := false
	for _, currentTrack := range tracks {
		worlds := a.catalog.Worlds(currentTrack)
		for _, world := range worlds {
			visible := make([]mission.Mission, 0, len(world.Missions))
			for _, item := range world.Missions {
				if !matchesScope(item) {
					continue
				}
				complete := player.IsComplete(item.ID)
				if *completedOnly && !complete || *remainingOnly && complete {
					continue
				}
				visible = append(visible, item)
			}
			if len(visible) == 0 {
				continue
			}
			displayedTracks[currentTrack] = true
			if printedWorld {
				fmt.Fprintln(a.out)
			}
			worldDone := completedMissions(world.Missions, player)
			prefix := fmt.Sprintf("WORLD %d/%d", world.Number, len(worlds))
			if selectedTrack == "all" {
				prefix = strings.ToUpper(trackDisplayName(currentTrack)) + " · " + prefix
			}
			fmt.Fprintf(a.out, "%s  %s\n", a.style.World(prefix+" · "+world.Name), a.style.Muted(fmt.Sprintf("%d/%d complete", worldDone, len(world.Missions))))
			for _, item := range visible {
				isComplete := player.IsComplete(item.ID)
				status := "○"
				if isComplete {
					status = a.style.Success("✓")
				} else {
					status = a.style.Muted(status)
				}
				placement, _ := a.catalog.Placement(item.ID)
				difficulty := a.style.Difficulty(item.Difficulty)
				reward := a.style.Reward(fmt.Sprintf("%d XP", item.Rewards.XP))
				fmt.Fprintf(a.out, "  %s Stage %d/%d · #%02d · %s · %s · %s", status, placement.StageNumber, placement.StageTotal, item.Number, item.Title, difficulty, reward)
				if *showIDs {
					fmt.Fprintf(a.out, " · %s", a.style.Muted(item.ID))
				}
				fmt.Fprintln(a.out)
				shown++
			}
			printedWorld = true
		}
	}
	if shown == 0 {
		fmt.Fprintln(a.out, "  No missions match these filters.")
	}
	scoped := slices.DeleteFunc(a.catalog.All(), func(item mission.Mission) bool { return !matchesScope(item) })
	completed, total := completedMissions(scoped, player), len(scoped)
	fmt.Fprintf(a.out, "\n%s\n", a.style.Accent(fmt.Sprintf("%d/%d missions complete · Player total: %d XP", completed, total, player.XP)))
	if shown > 0 {
		if inMission {
			fmt.Fprintf(a.out, "Navigate here: %s · %s · reveal IDs with %s\n",
				a.style.Accent("world N"), a.style.Accent("play STAGE/ID"), a.style.Accent("list --ids"))
		} else {
			commandTrack := selectedTrack
			if commandTrack == "all" && len(displayedTracks) == 1 {
				for track := range displayedTracks {
					commandTrack = track
				}
			}
			follow := "opsquest play"
			jump := "opsquest play --world N"
			ids := "opsquest map --ids"
			switch commandTrack {
			case mission.TrackDocker:
				follow = "opsquest play --track docker"
				jump = "opsquest play --track docker --world N"
				ids = "opsquest map --track docker --ids"
			case "all":
				follow = "opsquest play --track TRACK"
				jump = "opsquest play --track TRACK --world N"
				ids = "opsquest map --track all --ids"
			}
			fmt.Fprintf(a.out, "Continue: %s · Jump: %s · IDs: %s\n",
				a.style.Accent(follow), a.style.Accent(jump), a.style.Accent(ids))
		}
	}
	if selectedTrack == mission.TrackDocker {
		if item, found := a.catalog.FirstInTrack(mission.TrackDocker); found {
			availability := game.EnvironmentAvailability(a.ctx, a.factory, item)
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
	flags := a.newFlagSet("profile", func() { a.printProfileUsage(a.errOut) })
	name := flags.String("name", "", "update the operator display name")
	if help, err := parseFlags(flags, args); help || err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("profile does not accept positional arguments")
	}
	nameProvided := flagProvided(flags, "name")
	player, err := a.loadPlayer()
	if err != nil {
		return err
	}
	if nameProvided {
		if err := profile.ValidateName(*name); err != nil {
			return err
		}
		player.Name = strings.TrimSpace(*name)
		if err := a.store.Save(player); err != nil {
			return err
		}
		fmt.Fprintln(a.out, a.style.Success("Profile name updated."))
	}
	completed := completedMissions(a.catalog.All(), player)
	type worldProgress struct {
		name      string
		track     string
		number    int
		completed int
		total     int
	}
	progress := make([]worldProgress, 0)
	for _, track := range []string{mission.TrackLinux, mission.TrackDocker} {
		for _, world := range a.catalog.Worlds(track) {
			item := worldProgress{name: world.Name, track: track, number: world.Number, completed: completedMissions(world.Missions, player), total: len(world.Missions)}
			progress = append(progress, item)
		}
	}
	linuxMissions, dockerMissions := a.catalog.InTrack(mission.TrackLinux), a.catalog.InTrack(mission.TrackDocker)
	linuxCompleted, linuxTotal := completedMissions(linuxMissions, player), len(linuxMissions)
	dockerCompleted, dockerTotal := completedMissions(dockerMissions, player), len(dockerMissions)
	fmt.Fprintln(a.out, a.style.Header("PROFILE"))
	fmt.Fprintln(a.out)
	fmt.Fprintf(a.out, "%s %s\n", a.style.Accent("Operator:"), player.Name)
	fmt.Fprintf(a.out, "%s %s\n", a.style.Accent("Rank:"), player.Rank())
	fmt.Fprintf(a.out, "%s %d · %s\n", a.style.Accent("Level:"), player.Level(), a.style.Reward(fmt.Sprintf("%d XP", player.XP)))
	if nextRank, needed, exists := player.NextRank(); exists {
		fmt.Fprintf(a.out, "%s %s in %s\n", a.style.Muted("Next rank:"), nextRank, a.style.Reward(fmt.Sprintf("%d XP", needed)))
	}
	fmt.Fprintln(a.out)
	labelWidth := 0
	for _, world := range progress {
		label := fmt.Sprintf("World %d · %s", world.number, world.name)
		labelWidth = max(labelWidth, len(label))
	}
	printWorlds := func(track string) {
		for _, world := range progress {
			if world.track != track {
				continue
			}
			label := fmt.Sprintf("World %d · %s", world.number, world.name)
			fmt.Fprintf(a.out, "  %s %s %3d%%\n", a.style.World(fmt.Sprintf("%-*s", labelWidth, label)), styledProgressBar(a.style, world.completed, world.total, 10), percentage(world.completed, world.total))
		}
	}
	fmt.Fprintf(a.out, "%s  %s %3d%%\n", a.style.Section("Linux"), styledProgressBar(a.style, linuxCompleted, linuxTotal, 20), percentage(linuxCompleted, linuxTotal))
	printWorlds(mission.TrackLinux)
	dockerState := "readiness: run opsquest doctor"
	fmt.Fprintf(a.out, "%s %s %3d%%  %s\n", a.style.Section("Docker"), styledProgressBar(a.style, dockerCompleted, dockerTotal, 20), percentage(dockerCompleted, dockerTotal), dockerState)
	printWorlds(mission.TrackDocker)
	fmt.Fprintf(a.out, "%s\n\n", a.style.Muted(fmt.Sprintf("K8s    %s  locked", progressBar(0, 20, 20))))
	fmt.Fprintf(a.out, "Commands mastered: %d\n", len(player.Commands))
	fmt.Fprintf(a.out, "Missions completed: %d\n", completed)
	completedHints, activeHints := player.HintsUsed(), player.ActiveHints()
	fmt.Fprintf(a.out, "Hints used: %d (completed: %d · active: %d)\n", completedHints+activeHints, completedHints, activeHints)
	fmt.Fprintf(a.out, "Achievements: %d/%d\n", player.AchievementCount(), len(profile.AchievementDefinitions()))
	return nil
}

func (a *App) runCommands(args []string) error {
	flags := a.newFlagSet("commands", func() { fmt.Fprintln(a.errOut, "Usage: opsquest commands") })
	if help, err := parseFlags(flags, args); help || err != nil {
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
	flags := a.newFlagSet("achievements", func() { fmt.Fprintln(a.errOut, "Usage: opsquest achievements") })
	if help, err := parseFlags(flags, args); help || err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("achievements does not accept positional arguments")
	}
	player, err := a.loadPlayer()
	if err != nil {
		return err
	}
	definitions := profile.AchievementDefinitions()
	fmt.Fprintln(a.out, a.style.Header("ACHIEVEMENTS"))
	for _, achievement := range definitions {
		unlockedAt, unlocked := player.Unlocked[achievement.ID]
		if unlocked {
			title := fmt.Sprintf("%-22s", achievement.Title)
			fmt.Fprintf(a.out, "  %s %s %s  %s\n", a.style.Achievement("★"), a.style.Achievement(title), achievement.Description, a.style.Muted("["+unlockedAt.Local().Format("2006-01-02")+"]"))
		} else {
			fmt.Fprintf(a.out, "  %s %-22s %s\n", a.style.Muted("☆"), achievement.Title, achievement.Description)
		}
	}
	fmt.Fprintf(a.out, "\n%d/%d unlocked\n", player.AchievementCount(), len(definitions))
	return nil
}

func (a *App) runShow(args []string) error {
	flags := a.newFlagSet("show", func() { fmt.Fprintln(a.errOut, "Usage: opsquest show [MISSION]") })
	if help, err := parseFlags(flags, args); help || err != nil {
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
			item, found = a.catalog.LastInTrack(mission.TrackLinux)
		}
	}
	if !found {
		return fmt.Errorf("no missions are available")
	}
	completion, complete := player.Completed[item.ID]
	status := "not completed"
	if complete {
		status = "completed"
	}
	hintsUsed := player.MissionHints(item.ID)
	fmt.Fprintln(a.out, a.style.Header(fmt.Sprintf("MISSION %02d: %s", item.Number, item.Title)))
	if placement, exists := a.catalog.Placement(item.ID); exists {
		fmt.Fprintf(a.out, "Track: %s · World %d/%d: %s · Stage %d/%d\n",
			trackDisplayName(placement.Track), placement.WorldNumber, placement.WorldTotal,
			a.style.World(placement.WorldName), placement.StageNumber, placement.StageTotal)
	} else {
		fmt.Fprintf(a.out, "Track: %s · Campaign: %s\n", trackDisplayName(item.EffectiveTrack()), a.style.World(item.Campaign))
	}
	reward := a.style.Reward(fmt.Sprintf("%d XP", game.AdjustedReward(item, hintsUsed)))
	if complete {
		reward = a.style.Accent(fmt.Sprintf("claimed · %d XP earned", completion.XP))
	}
	fmt.Fprintf(a.out, "Difficulty: %s · Reward: %s\n", a.style.Difficulty(item.Difficulty), reward)
	styledStatus := a.style.Accent(status)
	if complete {
		styledStatus = a.style.Success(status)
	}
	remainingHints := max(len(item.Hints)-hintsUsed, 0)
	hintStatus := fmt.Sprintf("%d used · %d remaining · %d total", hintsUsed, remainingHints, len(item.Hints))
	if complete {
		hintStatus = fmt.Sprintf("%d used on first completion · replay hints do not change XP", completion.HintsUsed)
	}
	fmt.Fprintf(a.out, "Status: %s · Outcome checks: %d · Hints: %s\n\n", styledStatus, len(item.Validation.All), hintStatus)
	fmt.Fprintf(a.out, "%s\n%s\n\n", a.style.Section("INCIDENT"), item.Story)
	fmt.Fprintf(a.out, "%s\n%s\n\n%s\n", a.style.Section("OBJECTIVE"), item.Objective, a.style.CommandGuide(item.SuggestedCommands))
	if item.EffectiveEnvironment() == mission.EnvironmentDocker {
		availability := game.EnvironmentAvailability(a.ctx, a.factory, item)
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
	flags := a.newFlagSet("doctor", func() { fmt.Fprintln(a.errOut, "Usage: opsquest doctor") })
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
	if item, found := a.catalog.FirstInTrack(mission.TrackDocker); found {
		availability := game.EnvironmentAvailability(a.ctx, a.factory, item)
		if availability.Available {
			fmt.Fprintf(a.out, "  %s docker labs: ready · %s\n", check, availability.Detail)
		} else {
			fmt.Fprintf(a.out, "  %s docker labs: unavailable · %s\n", a.style.Warning("!"), availability.Detail)
		}
	}
	return nil
}

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
  opsquest doctor          Check the catalog, profile, and safety mode
  opsquest reset [--yes]   Reset local progress
  opsquest version         Print the version

New here: opsquest guide
Start playing: opsquest play`)
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
	return min(max(filled, 0), width)
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

func completedMissions(items []mission.Mission, player profile.Profile) int {
	completed := 0
	for _, item := range items {
		if player.IsComplete(item.ID) {
			completed++
		}
	}
	return completed
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
