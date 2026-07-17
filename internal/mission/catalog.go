package mission

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
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
		item, err := decodeMission(data)
		if err != nil {
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
	for index, item := range catalog.missions {
		if item.Number != index+1 {
			return Catalog{}, fmt.Errorf("mission numbers must be contiguous: expected %d, found %d", index+1, item.Number)
		}
	}
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

var (
	missionIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	variablePattern  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func validateMission(item Mission) error {
	if !missionIDPattern.MatchString(item.ID) {
		return fmt.Errorf("id %q must use lowercase words separated by hyphens", item.ID)
	}
	for name, value := range map[string]string{
		"title": item.Title, "campaign": item.Campaign, "difficulty": item.Difficulty,
		"story": item.Story, "objective": item.Objective, "explanation": item.Explanation,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if item.Number < 1 {
		return fmt.Errorf("number must be positive")
	}
	if err := validateAbsolutePath("start_dir", item.StartDir, true); err != nil {
		return err
	}
	if item.Rewards.XP <= 0 || item.Rewards.HintPenalty < 0 {
		return fmt.Errorf("XP must be positive and hint penalty cannot be negative")
	}
	for index, hint := range item.Hints {
		if strings.TrimSpace(hint) == "" {
			return fmt.Errorf("hint %d is empty", index+1)
		}
	}
	if err := validateSetup(item.Setup, item.StartDir); err != nil {
		return err
	}
	if len(item.Validation.All) == 0 {
		return fmt.Errorf("at least one validation condition is required")
	}
	for index, condition := range item.Validation.All {
		if err := validateCondition(condition); err != nil {
			return fmt.Errorf("validation condition %d: %w", index+1, err)
		}
	}
	return nil
}

func validateSetup(setup Setup, startDir string) error {
	entries := map[string]string{"/": "directory"}
	addEntry := func(name, kind string) error {
		if existing, found := entries[name]; found {
			return fmt.Errorf("setup path %s is both %s and %s", name, existing, kind)
		}
		entries[name] = kind
		return nil
	}
	for _, directory := range setup.Directories {
		if err := validateAbsolutePath("directory", directory.Path, true); err != nil {
			return err
		}
		if err := validateMode(directory.Mode); err != nil {
			return fmt.Errorf("directory %s: %w", directory.Path, err)
		}
		if err := addEntry(directory.Path, "directory"); err != nil {
			return err
		}
	}
	for _, file := range setup.Files {
		if err := validateAbsolutePath("file", file.Path, false); err != nil {
			return err
		}
		if err := validateMode(file.Mode); err != nil {
			return fmt.Errorf("file %s: %w", file.Path, err)
		}
		if err := addEntry(file.Path, "file"); err != nil {
			return err
		}
	}
	for _, archive := range setup.Archives {
		if err := validateAbsolutePath("archive", archive.Path, false); err != nil {
			return err
		}
		if err := addEntry(archive.Path, "archive"); err != nil {
			return err
		}
		archiveEntries := make(map[string]bool)
		for _, entry := range archive.Entries {
			cleaned, err := validateArchiveEntry(entry.Path)
			if err != nil {
				return fmt.Errorf("archive %s entry %q: %w", archive.Path, entry.Path, err)
			}
			if archiveEntries[cleaned] {
				return fmt.Errorf("archive %s contains duplicate entry %s", archive.Path, cleaned)
			}
			archiveEntries[cleaned] = true
			if err := validateMode(entry.Mode); err != nil {
				return fmt.Errorf("archive %s entry %s: %w", archive.Path, entry.Path, err)
			}
		}
	}
	for entryPath, kind := range entries {
		if entryPath == "/" {
			continue
		}
		for parent := path.Dir(entryPath); parent != "/"; parent = path.Dir(parent) {
			if parentKind, exists := entries[parent]; exists && parentKind != "directory" {
				return fmt.Errorf("%s %s is nested beneath non-directory %s", kind, entryPath, parent)
			}
		}
	}
	for name := range setup.Environment {
		if !variablePattern.MatchString(name) {
			return fmt.Errorf("invalid environment variable name %q", name)
		}
	}
	pids := make(map[int]bool)
	for _, process := range setup.Processes {
		if process.PID <= 0 || strings.TrimSpace(process.Command) == "" {
			return fmt.Errorf("process PID and command are required")
		}
		if pids[process.PID] {
			return fmt.Errorf("duplicate process PID %d", process.PID)
		}
		pids[process.PID] = true
	}
	startExists := startDir == "/"
	if kind, exists := entries[startDir]; exists {
		if kind != "directory" {
			return fmt.Errorf("start_dir %s is not a directory", startDir)
		}
		startExists = true
	}
	for entryPath := range entries {
		if strings.HasPrefix(entryPath, startDir+"/") {
			startExists = true
			break
		}
	}
	if !startExists {
		return fmt.Errorf("start_dir %s is not created by setup", startDir)
	}
	return nil
}

func validateCondition(condition Condition) error {
	pathTypes := map[string]bool{
		"file_exists": true, "dir_exists": true, "path_missing": true,
		"file_content_equals": true, "file_content_contains": true,
		"file_lines_equal": true, "file_mode_equals": true, "file_owner_equals": true,
	}
	if pathTypes[condition.Type] {
		if err := validateAbsolutePath(condition.Type, condition.Path, condition.Type == "dir_exists"); err != nil {
			return err
		}
	}
	switch condition.Type {
	case "output_equals":
		return nil
	case "output_contains", "output_not_contains":
		if condition.Value == "" {
			return fmt.Errorf("value cannot be empty")
		}
	case "output_contains_all":
		if len(condition.Values) == 0 {
			return fmt.Errorf("values cannot be empty")
		}
	case "cwd_equals":
		return validateAbsolutePath("cwd_equals", condition.Value, true)
	case "file_exists", "dir_exists", "path_missing", "file_content_equals":
		return nil
	case "file_content_contains":
		if condition.Value == "" {
			return fmt.Errorf("value cannot be empty")
		}
	case "file_lines_equal":
		if len(condition.Values) == 0 {
			return fmt.Errorf("values cannot be empty")
		}
		for _, value := range condition.Values {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("line values cannot be empty")
			}
		}
	case "file_mode_equals":
		return validateMode(condition.Value)
	case "file_owner_equals":
		if strings.TrimSpace(condition.Value) == "" {
			return fmt.Errorf("owner cannot be empty")
		}
	case "process_stopped", "process_running":
		if condition.PID <= 0 {
			return fmt.Errorf("PID must be positive")
		}
	case "env_equals":
		name, _, found := strings.Cut(condition.Value, "=")
		if !found || !variablePattern.MatchString(name) {
			return fmt.Errorf("value must be NAME=value")
		}
	default:
		return fmt.Errorf("unknown type %q", condition.Type)
	}
	return nil
}

func validateAbsolutePath(kind, value string, allowRoot bool) error {
	if !strings.HasPrefix(value, "/") || path.Clean(value) != value {
		return fmt.Errorf("%s path %q must be absolute and clean", kind, value)
	}
	if value == "/" && !allowRoot {
		return fmt.Errorf("%s path cannot be /", kind)
	}
	return nil
}

func validateMode(value string) error {
	if value == "" {
		return nil
	}
	mode, err := strconv.ParseUint(value, 8, 12)
	if err != nil || mode > 0o7777 {
		return fmt.Errorf("invalid mode %q", value)
	}
	return nil
}

func validateArchiveEntry(value string) (string, error) {
	if value == "" || strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("path must be relative")
	}
	cleaned := path.Clean(value)
	if cleaned != value || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("path must be clean and remain inside the archive")
	}
	return cleaned, nil
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
