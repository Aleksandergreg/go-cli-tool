package mission

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

//go:embed data/*.json
var missionFiles embed.FS

type Catalog struct {
	missions []Mission
	byID     map[string]Mission
}

func LoadCatalog() (Catalog, error) {
	paths, err := fs.Glob(missionFiles, "data/*.json")
	if err != nil {
		return Catalog{}, fmt.Errorf("find embedded missions: %w", err)
	}

	catalog := Catalog{byID: make(map[string]Mission, len(paths))}
	numbers := make(map[int]string, len(paths))
	for _, path := range paths {
		data, err := missionFiles.ReadFile(path)
		if err != nil {
			return Catalog{}, fmt.Errorf("read %s: %w", path, err)
		}
		var item Mission
		if err := json.Unmarshal(data, &item); err != nil {
			return Catalog{}, fmt.Errorf("decode %s: %w", path, err)
		}
		if err := validateMission(item); err != nil {
			return Catalog{}, fmt.Errorf("%s: %w", path, err)
		}
		if _, exists := catalog.byID[item.ID]; exists {
			return Catalog{}, fmt.Errorf("duplicate mission id %q", item.ID)
		}
		if other, exists := numbers[item.Number]; exists {
			return Catalog{}, fmt.Errorf("missions %q and %q both use number %d", other, item.ID, item.Number)
		}
		catalog.byID[item.ID] = item
		numbers[item.Number] = item.ID
		catalog.missions = append(catalog.missions, item)
	}
	sort.Slice(catalog.missions, func(i, j int) bool {
		return catalog.missions[i].Number < catalog.missions[j].Number
	})
	return catalog, nil
}

func validateMission(item Mission) error {
	if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Title) == "" {
		return fmt.Errorf("id and title are required")
	}
	if item.Number < 1 {
		return fmt.Errorf("number must be positive")
	}
	if !strings.HasPrefix(item.StartDir, "/") {
		return fmt.Errorf("start_dir must be absolute")
	}
	if item.Rewards.XP < 0 || item.Rewards.HintPenalty < 0 {
		return fmt.Errorf("rewards cannot be negative")
	}
	if len(item.Validation.All) == 0 {
		return fmt.Errorf("at least one validation condition is required")
	}
	return nil
}

func (c Catalog) All() []Mission {
	items := make([]Mission, len(c.missions))
	copy(items, c.missions)
	return items
}

func (c Catalog) Find(ref string) (Mission, bool) {
	ref = strings.TrimSpace(ref)
	if item, ok := c.byID[ref]; ok {
		return item, true
	}
	number, err := strconv.Atoi(strings.TrimLeft(ref, "0"))
	if err == nil {
		for _, item := range c.missions {
			if item.Number == number {
				return item, true
			}
		}
	}
	return Mission{}, false
}

func (c Catalog) Next(completed func(string) bool) (Mission, bool) {
	for _, item := range c.missions {
		if !completed(item.ID) {
			return item, true
		}
	}
	return Mission{}, false
}
