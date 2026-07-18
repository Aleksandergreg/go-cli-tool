package mission

import "fmt"

// World is one ordered campaign within a learning track. Mission numbers stay
// global and stable; Number and the order of Missions describe the local
// curriculum position for presentation and navigation.
type World struct {
	Number   int
	Track    string
	Name     string
	Missions []Mission
}

// Placement describes one mission's track-local world and stage position.
// All positions and totals are one-based where applicable.
type Placement struct {
	Track       string
	WorldNumber int
	WorldTotal  int
	WorldName   string
	StageNumber int
	StageTotal  int
}

// Worlds returns the ordered worlds for track. An empty track retains the
// catalog's backwards-compatible Linux default. Returned missions are deep
// copies and cannot mutate catalog-owned content.
func (c Catalog) Worlds(track string) []World {
	track = defaultTrack(track)
	worlds := c.worlds[track]
	if len(worlds) == 0 {
		return nil
	}
	cloned := make([]World, len(worlds))
	for index, world := range worlds {
		cloned[index] = cloneWorld(world)
	}
	return cloned
}

// World returns one track-local world by its one-based number. An empty track
// selects Linux. Unknown tracks and out-of-range numbers return false.
func (c Catalog) World(track string, number int) (World, bool) {
	track = defaultTrack(track)
	worlds := c.worlds[track]
	if number < 1 || number > len(worlds) {
		return World{}, false
	}
	return cloneWorld(worlds[number-1]), true
}

// Placement returns the track-local world and stage position for missionID.
func (c Catalog) Placement(missionID string) (Placement, bool) {
	placement, found := c.placementByID[missionID]
	return placement, found
}

// NextInWorld returns the first incomplete mission in one track-local world.
// A nil completion predicate treats every mission as incomplete. The returned
// mission is a deep copy of catalog content.
func (c Catalog) NextInWorld(track string, number int, completed func(string) bool) (Mission, bool) {
	world, found := c.world(track, number)
	if !found {
		return Mission{}, false
	}
	for _, item := range world.Missions {
		if completed == nil || !completed(item.ID) {
			return cloneMission(item), true
		}
	}
	return Mission{}, false
}

// validateWorldContiguity prevents one campaign from producing two different
// worlds in the same track. Missions from another track do not interrupt a
// world's sequence because each track has its own curriculum.
func validateWorldContiguity(items []Mission) error {
	active := make(map[string]string)
	closed := make(map[string]map[string]bool)
	for _, item := range items {
		track := item.EffectiveTrack()
		if active[track] == item.Campaign {
			continue
		}
		if closed[track] == nil {
			closed[track] = make(map[string]bool)
		}
		if closed[track][item.Campaign] {
			return fmt.Errorf("campaign %q must be contiguous within track %q", item.Campaign, track)
		}
		if current := active[track]; current != "" {
			closed[track][current] = true
		}
		active[track] = item.Campaign
	}
	return nil
}

func (c *Catalog) indexWorlds() {
	c.worlds = make(map[string][]World)
	for _, item := range c.missions {
		track := item.EffectiveTrack()
		worlds := c.worlds[track]
		if len(worlds) == 0 || worlds[len(worlds)-1].Name != item.Campaign {
			worlds = append(worlds, World{
				Number: len(worlds) + 1,
				Track:  track,
				Name:   item.Campaign,
			})
		}
		last := len(worlds) - 1
		worlds[last].Missions = append(worlds[last].Missions, cloneMission(item))
		c.worlds[track] = worlds
	}

	c.placementByID = make(map[string]Placement, len(c.missions))
	for track, worlds := range c.worlds {
		for worldIndex, world := range worlds {
			for stageIndex, item := range world.Missions {
				c.placementByID[item.ID] = Placement{
					Track:       track,
					WorldNumber: worldIndex + 1,
					WorldTotal:  len(worlds),
					WorldName:   world.Name,
					StageNumber: stageIndex + 1,
					StageTotal:  len(world.Missions),
				}
			}
		}
	}
}

func (c Catalog) world(track string, number int) (World, bool) {
	track = defaultTrack(track)
	worlds := c.worlds[track]
	if number < 1 || number > len(worlds) {
		return World{}, false
	}
	return worlds[number-1], true
}

func defaultTrack(track string) string {
	if track == "" {
		return TrackLinux
	}
	return track
}

func cloneWorld(world World) World {
	cloned := world
	cloned.Missions = make([]Mission, len(world.Missions))
	for index, item := range world.Missions {
		cloned.Missions[index] = cloneMission(item)
	}
	return cloned
}
