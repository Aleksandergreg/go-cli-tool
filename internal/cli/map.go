package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
)

func (a *App) runList(args []string) error {
	player, err := a.loadPlayer()
	if err != nil {
		return err
	}
	return a.listMissions(args, player, false)
}

// mapFilter is the validated scope and display options of one map listing.
type mapFilter struct {
	track         string
	campaign      string
	completedOnly bool
	remainingOnly bool
	showIDs       bool
}

// inScope reports whether item belongs to the selected track and campaign,
// regardless of completion.
func (f mapFilter) inScope(item mission.Mission) bool {
	if f.track != "all" && item.EffectiveTrack() != f.track {
		return false
	}
	return f.campaign == "" || strings.EqualFold(item.Campaign, f.campaign)
}

// shows reports whether item is listed once completion filters apply.
func (f mapFilter) shows(item mission.Mission, player profile.Profile) bool {
	if !f.inScope(item) {
		return false
	}
	complete := player.IsComplete(item.ID)
	return !(f.completedOnly && !complete || f.remainingOnly && complete)
}

func (a *App) listMissions(args []string, player profile.Profile, inMission bool) error {
	filter, help, err := a.parseMapFilter(args)
	if help || err != nil {
		return err
	}
	heading := trackHeading(filter.track)
	if strings.TrimSpace(filter.campaign) != "" {
		heading = "WORLD · " + filter.campaign
	}
	fmt.Fprintln(a.out, a.style.Header(heading))
	shown := 0
	displayedTracks := make(map[string]bool)
	tracks := []string{filter.track}
	if filter.track == "all" {
		tracks = []string{mission.TrackLinux, mission.TrackDocker}
	}
	for _, currentTrack := range tracks {
		worlds := a.catalog.Worlds(currentTrack)
		for _, world := range worlds {
			visible := slices.DeleteFunc(slices.Clone(world.Missions), func(item mission.Mission) bool { return !filter.shows(item, player) })
			if len(visible) == 0 {
				continue
			}
			displayedTracks[currentTrack] = true
			if shown > 0 {
				fmt.Fprintln(a.out)
			}
			a.printMapWorld(world, len(worlds), visible, filter, player)
			shown += len(visible)
		}
	}
	if shown == 0 {
		fmt.Fprintln(a.out, "  No missions match these filters.")
	}
	scoped := slices.DeleteFunc(a.catalog.All(), func(item mission.Mission) bool { return !filter.inScope(item) })
	fmt.Fprintf(a.out, "\n%s\n", a.style.Accent(fmt.Sprintf("%d/%d missions complete · Player total: %d XP", completedMissions(scoped, player), len(scoped), player.XP)))
	if shown > 0 {
		a.printMapNavigation(filter.track, displayedTracks, inMission)
	}
	if filter.track == mission.TrackDocker {
		a.printDockerReadiness()
	}
	return nil
}

func (a *App) parseMapFilter(args []string) (mapFilter, bool, error) {
	flags := a.newFlagSet("list", func() { a.printListUsage(a.errOut) })
	completedOnly := flags.Bool("completed", false, "show completed missions only")
	remainingOnly := flags.Bool("remaining", false, "show incomplete missions only")
	showIDs := flags.Bool("ids", false, "show stable mission IDs")
	campaign := flags.String("campaign", "", "filter by campaign name")
	track := flags.String("track", mission.TrackLinux, "filter by track: linux, docker, or all")
	if help, err := parseFlags(flags, args); help || err != nil {
		return mapFilter{}, help, err
	}
	if flags.NArg() > 0 {
		return mapFilter{}, false, fmt.Errorf("list does not accept positional arguments")
	}
	if *completedOnly && *remainingOnly {
		return mapFilter{}, false, fmt.Errorf("--completed and --remaining cannot be combined")
	}
	selectedTrack := strings.ToLower(strings.TrimSpace(*track))
	if *campaign != "" && !flagProvided(flags, "track") {
		selectedTrack = "all"
	}
	if selectedTrack != mission.TrackLinux && selectedTrack != mission.TrackDocker && selectedTrack != "all" {
		return mapFilter{}, false, fmt.Errorf("unknown track %q; use linux, docker, or all", *track)
	}
	return mapFilter{track: selectedTrack, campaign: *campaign, completedOnly: *completedOnly, remainingOnly: *remainingOnly, showIDs: *showIDs}, false, nil
}

// printMapWorld prints one world heading and its visible stages.
func (a *App) printMapWorld(world mission.World, worldTotal int, visible []mission.Mission, filter mapFilter, player profile.Profile) {
	prefix := fmt.Sprintf("WORLD %d/%d", world.Number, worldTotal)
	if filter.track == "all" {
		prefix = strings.ToUpper(trackDisplayName(world.Track)) + " · " + prefix
	}
	fmt.Fprintf(a.out, "%s  %s\n", a.style.World(prefix+" · "+world.Name), a.style.Muted(fmt.Sprintf("%d/%d complete", completedMissions(world.Missions, player), len(world.Missions))))
	for _, item := range visible {
		status := a.style.Muted("○")
		if player.IsComplete(item.ID) {
			status = a.style.Success("✓")
		}
		placement, _ := a.catalog.Placement(item.ID)
		difficulty := a.style.Difficulty(item.Difficulty)
		reward := a.style.Reward(fmt.Sprintf("%d XP", item.Rewards.XP))
		fmt.Fprintf(a.out, "  %s Stage %d/%d · #%02d · %s · %s · %s", status, placement.StageNumber, placement.StageTotal, item.Number, item.Title, difficulty, reward)
		if filter.showIDs {
			fmt.Fprintf(a.out, " · %s", a.style.Muted(item.ID))
		}
		fmt.Fprintln(a.out)
	}
}

// printMapNavigation prints the commands that continue from this listing:
// in-lab navigation inside a mission, or the matching play commands outside.
func (a *App) printMapNavigation(selectedTrack string, displayedTracks map[string]bool, inMission bool) {
	if inMission {
		fmt.Fprintf(a.out, "Navigate here: %s · %s · reveal IDs with %s\n",
			a.style.Accent("world N"), a.style.Accent("play STAGE/ID"), a.style.Accent("list --ids"))
		return
	}
	commandTrack := selectedTrack
	if commandTrack == "all" && len(displayedTracks) == 1 {
		for track := range displayedTracks {
			commandTrack = track
		}
	}
	follow, jump, ids := "opsquest play", "opsquest play --world N", "opsquest map --ids"
	switch commandTrack {
	case mission.TrackDocker:
		follow, jump, ids = "opsquest play --track docker", "opsquest play --track docker --world N", "opsquest map --track docker --ids"
	case "all":
		follow, jump, ids = "opsquest play --track TRACK", "opsquest play --track TRACK --world N", "opsquest map --track all --ids"
	}
	fmt.Fprintf(a.out, "Continue: %s · Jump: %s · IDs: %s\n",
		a.style.Accent(follow), a.style.Accent(jump), a.style.Accent(ids))
}

func (a *App) printDockerReadiness() {
	item, found := a.catalog.FirstInTrack(mission.TrackDocker)
	if !found {
		return
	}
	if availability := game.EnvironmentAvailability(a.ctx, a.factory, item); availability.Available {
		fmt.Fprintln(a.out, a.style.Success("Docker labs ready."))
	} else {
		fmt.Fprintln(a.out, a.style.Warning(availability.Detail))
	}
}
