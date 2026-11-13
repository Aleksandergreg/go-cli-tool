package mission

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

//go:embed data/*.json
var missionFiles embed.FS

type Catalog struct {
	missions      []Mission
	byID          map[string]int
	byNumber      map[int]int
	worlds        map[string][]World
	placementByID map[string]Placement
}

func LoadCatalog() (Catalog, error) {
	paths, err := fs.Glob(missionFiles, "data/*.json")
	if err != nil {
		return Catalog{}, fmt.Errorf("find embedded missions: %w", err)
	}

	catalog := Catalog{}
	ids := make(map[string]bool, len(paths))
	numbers := make(map[int]string, len(paths))
	for _, path := range paths {
		data, err := missionFiles.ReadFile(path)
		if err != nil {
			return Catalog{}, fmt.Errorf("read %s: %w", path, err)
		}
		item, err := decodeMission(data)
		if err != nil {
			return Catalog{}, fmt.Errorf("decode %s: %w", path, err)
		}
		if err := validateMission(item); err != nil {
			return Catalog{}, fmt.Errorf("%s: %w", path, err)
		}
		if ids[item.ID] {
			return Catalog{}, fmt.Errorf("duplicate mission id %q", item.ID)
		}
		if other, exists := numbers[item.Number]; exists {
			return Catalog{}, fmt.Errorf("missions %q and %q both use number %d", other, item.ID, item.Number)
		}
		ids[item.ID] = true
		numbers[item.Number] = item.ID
		catalog.missions = append(catalog.missions, item)
	}
	sort.Slice(catalog.missions, func(i, j int) bool {
		return catalog.missions[i].Number < catalog.missions[j].Number
	})
	for index, item := range catalog.missions {
		if item.Number != index+1 {
			return Catalog{}, fmt.Errorf("mission numbers must be contiguous: expected %d, found %d", index+1, item.Number)
		}
	}
	if err := validateWorldContiguity(catalog.missions); err != nil {
		return Catalog{}, err
	}
	catalog.byID = make(map[string]int, len(catalog.missions))
	catalog.byNumber = make(map[int]int, len(catalog.missions))
	for index, item := range catalog.missions {
		catalog.byID[item.ID] = index
		catalog.byNumber[item.Number] = index
	}
	catalog.indexWorlds()
	return catalog, nil
}

func decodeMission(data []byte) (Mission, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var item Mission
	if err := decoder.Decode(&item); err != nil {
		return Mission{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return Mission{}, fmt.Errorf("multiple JSON values")
		}
		return Mission{}, err
	}
	return item, nil
}

func (c Catalog) All() []Mission {
	return cloneMissions(c.missions)
}

func (c Catalog) Find(ref string) (Mission, bool) {
	ref = strings.TrimSpace(ref)
	if index, ok := c.byID[ref]; ok {
		return cloneMission(c.missions[index]), true
	}
	number, err := strconv.Atoi(strings.TrimLeft(ref, "0"))
	if err == nil {
		if index, ok := c.byNumber[number]; ok {
			return cloneMission(c.missions[index]), true
		}
	}
	return Mission{}, false
}

// InTrack returns catalog missions in their global order, filtered to one
// learning track. An empty track selects the backwards-compatible Linux
// default.
func (c Catalog) InTrack(track string) []Mission {
	track = defaultTrack(track)
	items := make([]Mission, 0)
	for _, item := range c.missions {
		if item.EffectiveTrack() == track {
			items = append(items, cloneMission(item))
		}
	}
	return items
}

// NextInTrack returns the first incomplete mission in one learning track.
func (c Catalog) NextInTrack(track string, completed func(string) bool) (Mission, bool) {
	track = defaultTrack(track)
	for _, item := range c.missions {
		if item.EffectiveTrack() == track && !completed(item.ID) {
			return cloneMission(item), true
		}
	}
	return Mission{}, false
}

func (c Catalog) Next(completed func(string) bool) (Mission, bool) {
	for _, item := range c.missions {
		if !completed(item.ID) {
			return cloneMission(item), true
		}
	}
	return Mission{}, false
}

// FirstInTrack returns the first mission in global catalog order for track.
func (c Catalog) FirstInTrack(track string) (Mission, bool) {
	return c.findInTrack(defaultTrack(track), 0, 1)
}

// LastInTrack returns the last mission in global catalog order for track.
func (c Catalog) LastInTrack(track string) (Mission, bool) {
	return c.findInTrack(defaultTrack(track), len(c.missions)-1, -1)
}

// AdjacentInTrack returns the previous or next mission in the current
// mission's learning track. A negative direction moves backward and a
// positive direction moves forward; zero has no adjacent mission.
func (c Catalog) AdjacentInTrack(currentID string, direction int) (Mission, bool) {
	if direction == 0 {
		return Mission{}, false
	}
	currentIndex, found := c.byID[currentID]
	if !found {
		return Mission{}, false
	}
	step := 1
	if direction < 0 {
		step = -1
	}
	return c.findInTrack(c.missions[currentIndex].EffectiveTrack(), currentIndex+step, step)
}

func (c Catalog) findInTrack(track string, start, step int) (Mission, bool) {
	for index := start; index >= 0 && index < len(c.missions); index += step {
		if c.missions[index].EffectiveTrack() == track {
			return cloneMission(c.missions[index]), true
		}
	}
	return Mission{}, false
}
