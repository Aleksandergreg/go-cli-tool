package game

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/aleksandergregersen/opsquest/internal/mission"
)

func missionNavigationFields(line string) ([]string, bool, error) {
	fields, err := splitMissionNavigation(line)
	if len(fields) == 0 {
		return nil, false, nil
	}
	prefixed := fields[0] == "opsquest"
	if prefixed {
		fields = fields[1:]
		if len(fields) == 0 {
			if err != nil {
				return nil, true, err
			}
			return []string{"missions"}, true, nil
		}
	}
	switch fields[0] {
	case "list", "missions", "map", "worlds", "world", "play", "next", "previous", "prev":
		return fields, true, err
	default:
		if prefixed && err != nil {
			return nil, true, err
		}
		// Non-navigation input belongs to the active environment, including its
		// own quote and escape errors.
		return nil, false, nil
	}
}

// splitMissionNavigation parses the small, host-independent argument syntax
// accepted by in-mission navigation. It deliberately performs no expansion or
// command execution: whitespace separates fields, quotes group text, and a
// backslash escapes the next character outside single quotes.
func splitMissionNavigation(line string) ([]string, error) {
	fields := make([]string, 0)
	var field strings.Builder
	var quote rune
	escaped := false
	started := false

	flush := func() {
		if !started {
			return
		}
		fields = append(fields, field.String())
		field.Reset()
		started = false
	}

	for _, char := range line {
		if escaped {
			field.WriteRune(char)
			escaped = false
			started = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
				continue
			}
			if quote == '"' && char == '\\' {
				escaped = true
				continue
			}
			field.WriteRune(char)
			continue
		}

		switch {
		case unicode.IsSpace(char):
			flush()
		case char == '\'' || char == '"':
			quote = char
			started = true
		case char == '\\':
			escaped = true
			started = true
		default:
			field.WriteRune(char)
			started = true
		}
	}

	if escaped {
		flush()
		return fields, fmt.Errorf("unfinished escape")
	}
	if quote != 0 {
		flush()
		name := "single"
		if quote == '"' {
			name = "double"
		}
		return fields, fmt.Errorf("unterminated %s quote", name)
	}
	flush()
	return fields, nil
}

// navigate handles one in-lab navigation request. handled is false only for
// fields that are not a navigation command.
func (a *attempt) navigate(fields []string) (step, bool) {
	switch fields[0] {
	case "list", "missions", "map", "worlds":
		a.listMissions(fields)
		return step{}, true
	case "world":
		return a.switchWorld(fields), true
	case "play":
		return a.switchMission(fields), true
	case "next", "previous", "prev":
		return a.switchAdjacent(fields), true
	default:
		return step{}, false
	}
}

func (a *attempt) listMissions(fields []string) {
	if a.ListMissions == nil {
		a.fail("mission listing is unavailable in this session")
		return
	}
	listArgs := fields[1:]
	if fields[0] == "map" || fields[0] == "worlds" {
		if len(fields) != 1 {
			a.fail(fields[0] + " does not accept arguments")
			return
		}
		listArgs = nil
	}
	if err := a.ListMissions(listArgs); err != nil {
		a.fail(err.Error())
	}
}

// switchWorld starts the next incomplete stage of a world, or replays Stage 1
// of a completed one, and keeps continuous play inside that world.
func (a *attempt) switchWorld(fields []string) step {
	if len(fields) != 2 {
		a.fail("usage inside a mission: world NUMBER")
		return step{}
	}
	worldNumber, err := strconv.Atoi(fields[1])
	if err != nil || worldNumber < 1 {
		a.fail("world number must be a positive integer; use map to see worlds")
		return step{}
	}
	track := a.Mission.EffectiveTrack()
	target, found := a.Catalog.NextInWorld(track, worldNumber, a.Player.IsComplete)
	replayingCompletedWorld := false
	if !found {
		if world, exists := a.Catalog.World(track, worldNumber); exists && len(world.Missions) > 0 {
			target, found = world.Missions[0], true
			replayingCompletedWorld = true
		}
	}
	if !found {
		a.fail(fmt.Sprintf("world %d does not exist in the %s track; use map to see worlds", worldNumber, track))
		return step{}
	}
	if target.ID == a.Mission.ID {
		a.worldRoute = worldNumber
		message := fmt.Sprintf("World %d route selected; already playing the recommended stage.", worldNumber)
		if replayingCompletedWorld {
			message = fmt.Sprintf("World %d replay selected; already playing Stage 1.", worldNumber)
		}
		fmt.Fprintln(a.Out, a.Style.Accent(message))
		return step{}
	}
	if replayingCompletedWorld {
		fmt.Fprintln(a.Out, a.Style.Accent(fmt.Sprintf("World %d is complete; replaying Stage 1.", worldNumber)))
	}
	printMissionSwitch(a.Out, target, a.Style)
	return finish(SessionResult{SwitchMission: target.ID, WorldRoute: worldNumber, HintsUsed: a.hintsUsed})
}

// switchMission jumps to a stage of the current world by number, or to any
// mission by ID.
func (a *attempt) switchMission(fields []string) step {
	if len(fields) != 2 {
		a.fail("usage inside a mission: play STAGE_OR_ID")
		return step{}
	}
	ref := fields[1]
	target, found := mission.Mission{}, false
	if stageNumber, err := strconv.Atoi(ref); err == nil {
		placement, placed := a.Catalog.Placement(a.Mission.ID)
		if placed {
			world, worldFound := a.Catalog.World(placement.Track, placement.WorldNumber)
			if worldFound && stageNumber >= 1 && stageNumber <= len(world.Missions) {
				target, found = world.Missions[stageNumber-1], true
			}
			if !found {
				a.fail(fmt.Sprintf("stage %q does not exist in World %d; choose a stage from 1 to %d or use map", ref, placement.WorldNumber, placement.StageTotal))
				return step{}
			}
		}
	} else {
		target, found = a.Catalog.Find(ref)
	}
	if !found {
		a.fail(fmt.Sprintf("mission %q not found; use list --ids to see available mission IDs", ref))
		return step{}
	}
	if target.ID == a.Mission.ID {
		fmt.Fprintln(a.Out, a.Style.Accent(fmt.Sprintf("Already playing Mission %02d: %s.", target.Number, target.Title)))
		return step{}
	}
	printMissionSwitch(a.Out, target, a.Style)
	return finish(SessionResult{SwitchMission: target.ID, HintsUsed: a.hintsUsed})
}

func (a *attempt) switchAdjacent(fields []string) step {
	direction := 1
	if fields[0] != "next" {
		direction = -1
	}
	if len(fields) != 1 {
		a.fail(fmt.Sprintf("%s does not accept arguments", fields[0]))
		return step{}
	}
	target, found := a.Catalog.AdjacentInTrack(a.Mission.ID, direction)
	if !found {
		fmt.Fprintln(a.Out, a.Style.Warning(fmt.Sprintf("Mission %02d is already at this end of the catalog.", a.Mission.Number)))
		return step{}
	}
	printMissionSwitch(a.Out, target, a.Style)
	return finish(SessionResult{SwitchMission: target.ID, HintsUsed: a.hintsUsed})
}
