package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
)

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

// playOptions holds validated `opsquest play` flags and the optional
// positional mission reference.
type playOptions struct {
	track       string
	worldNumber int
	once        bool
	web         bool
	mission     string
}

func (a *App) runPlay(args []string) (returnErr error) {
	options, help, err := a.parsePlayOptions(args)
	if help || err != nil {
		return err
	}
	player, err := a.loadPlayer()
	if err != nil {
		return err
	}
	item, route, found, err := a.startingMission(options, player)
	if err != nil || !found {
		return err
	}
	var activeCompanion Companion
	if options.web {
		activeCompanion, err = a.startCompanion(a.ctx)
		if err != nil {
			return fmt.Errorf("start web companion: %w", err)
		}
		defer func() {
			if err := closeCompanion(activeCompanion); err != nil {
				returnErr = errors.Join(returnErr, err)
			}
		}()
		a.announceCompanion(activeCompanion)
	}
	if err := a.onboard(item, &player, options.web); err != nil {
		return err
	}
	reader := game.NewCommandLineReader(a.in, a.out)
	for {
		wasComplete := player.IsComplete(item.ID)
		result, err := a.newSession(item, &player, reader, activeCompanion).Run()
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
		if options.once {
			a.printNextRecommendation(item, player)
			return nil
		}
		next, found := a.nextOnRoute(route, item, player)
		if !found {
			a.printRouteFinished(route, item, player)
			return nil
		}
		a.announceWorldTransition(item, next, player, wasComplete)
		fmt.Fprintf(a.out, "\n%s\n", a.style.Accent(fmt.Sprintf("→ Continuing to Mission %02d: %s", next.Number, next.Title)))
		item = next
	}
}

func (a *App) parsePlayOptions(args []string) (playOptions, bool, error) {
	flags := a.newFlagSet("play", func() { a.printPlayUsage(a.errOut) })
	track := flags.String("track", mission.TrackLinux, "play the linux or docker track")
	worldNumber := flags.Int("world", 0, "start the next incomplete stage in a world")
	once := flags.Bool("once", false, "return after one completed mission")
	web := flags.Bool("web", false, "show mission guidance in a local browser companion")
	if help, err := parseFlags(flags, args); help || err != nil {
		return playOptions{}, help, err
	}
	if flags.NArg() > 1 {
		return playOptions{}, false, fmt.Errorf("usage: opsquest play [--track linux|docker] [--world NUMBER] [--once] [--web] [MISSION]")
	}
	trackProvided, worldProvided := flagProvided(flags, "track"), flagProvided(flags, "world")
	options := playOptions{
		track:       strings.ToLower(strings.TrimSpace(*track)),
		worldNumber: *worldNumber,
		once:        *once,
		web:         *web,
		mission:     flags.Arg(0),
	}
	if options.track != mission.TrackLinux && options.track != mission.TrackDocker {
		return playOptions{}, false, fmt.Errorf("unknown track %q; use linux or docker", *track)
	}
	if worldProvided && options.worldNumber < 1 {
		return playOptions{}, false, fmt.Errorf("world number must be positive")
	}
	if flags.NArg() == 1 && (worldProvided || trackProvided) {
		return playOptions{}, false, fmt.Errorf("MISSION cannot be combined with --track or --world")
	}
	return options, false, nil
}

// startingMission resolves where play begins and which route continuous play
// follows. It reports found=false without error when the track is complete.
func (a *App) startingMission(options playOptions, player profile.Profile) (mission.Mission, playRoute, bool, error) {
	switch {
	case options.mission != "":
		item, found := a.catalog.Find(options.mission)
		if !found {
			return mission.Mission{}, playRoute{}, false, fmt.Errorf("mission %q not found; run 'opsquest list'", options.mission)
		}
		return item, playRoute{kind: playRouteSequential}, true, nil
	case options.worldNumber > 0:
		route := playRoute{kind: playRouteWorld, worldNumber: options.worldNumber}
		if item, found := a.catalog.NextInWorld(options.track, options.worldNumber, player.IsComplete); found {
			return item, route, true, nil
		}
		if world, exists := a.catalog.World(options.track, options.worldNumber); exists && len(world.Missions) > 0 {
			fmt.Fprintf(a.out, "%s\n", a.style.Accent(fmt.Sprintf("World %d is complete; replaying Stage 1.", options.worldNumber)))
			return world.Missions[0], route, true, nil
		}
		return mission.Mission{}, playRoute{}, false, fmt.Errorf("world %d does not exist in the %s track; run 'opsquest map --track %s'", options.worldNumber, options.track, options.track)
	default:
		item, found := a.catalog.NextInTrack(options.track, player.IsComplete)
		if !found {
			fmt.Fprintf(a.out, "%s\n", a.style.Success(fmt.Sprintf("%s track complete! Replay a mission or inspect other worlds with 'opsquest map'.", trackDisplayName(options.track))))
			return mission.Mission{}, playRoute{}, false, nil
		}
		return item, playRoute{kind: playRouteRecommended}, true, nil
	}
}

func (a *App) announceCompanion(companion Companion) {
	fmt.Fprintln(a.out, a.style.Header("WEB MISSION COMPANION"))
	fmt.Fprintf(a.out, "Open this one-time local URL in your browser:\n%s\n", a.style.Accent(companion.URL()))
	fmt.Fprintln(a.out, a.style.Muted("Keep this terminal for commands. The browser can display progress but cannot execute anything."))
}

func closeCompanion(companion Companion) error {
	closeContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := companion.Close(closeContext); err != nil {
		return fmt.Errorf("close web companion: %w", err)
	}
	return nil
}

// onboard shows the quick start once, before a new player's first Linux
// mission, and records that it was shown.
func (a *App) onboard(item mission.Mission, player *profile.Profile, web bool) error {
	if item.EffectiveTrack() != mission.TrackLinux || !isPristineProfile(*player) {
		return nil
	}
	if !web {
		a.printQuickStart()
	}
	player.Onboarded = true
	return a.store.Save(*player)
}

// newSession builds one mission attempt. Its mission list reads the live
// player, so in-lab maps reflect progress made earlier in the same run.
func (a *App) newSession(item mission.Mission, player *profile.Profile, reader game.CommandLineReader, companion Companion) *game.Session {
	currentTrack := item.EffectiveTrack()
	return &game.Session{
		Mission:   item,
		Player:    player,
		Saver:     a.store,
		Reporter:  companion,
		Companion: companion != nil,
		In:        a.in,
		Out:       a.out,
		ErrOut:    a.errOut,
		Reader:    reader,
		Catalog:   a.catalog,
		ListMissions: func(args []string) error {
			if !hasFlag(args, "--track") {
				args = append([]string{"--track", currentTrack}, args...)
			}
			return a.listMissions(args, *player, true)
		},
		Context:    a.ctx,
		Factory:    a.factory,
		Style:      a.style,
		ErrorStyle: a.errorStyle,
	}
}

// announceWorldTransition celebrates a world the player just finished for
// the first time before continuous play moves into the next world.
func (a *App) announceWorldTransition(completed, next mission.Mission, player profile.Profile, wasComplete bool) {
	current, ok := a.catalog.Placement(completed.ID)
	if !ok {
		return
	}
	upcoming, ok := a.catalog.Placement(next.ID)
	if !ok || upcoming.WorldNumber == current.WorldNumber || wasComplete || !a.worldComplete(current, player) {
		return
	}
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
