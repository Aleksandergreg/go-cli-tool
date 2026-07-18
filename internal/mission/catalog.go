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
	missionIDPattern            = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	variablePattern             = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	dockerLogicalNamePattern    = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
	dockerImageReferencePattern = regexp.MustCompile(`^[a-z0-9]+(?:[._/-][a-z0-9]+)*(?::[A-Za-z0-9_.-]+)?@sha256:[a-f0-9]{64}$`)
)

// ValidDockerLogicalName reports whether value is safe to use as a stable
// mission alias. Runtime adapters use this same rule so catalog-valid content
// cannot become unplayable at environment setup time.
func ValidDockerLogicalName(value string) bool {
	return dockerLogicalNamePattern.MatchString(value)
}

// ValidDockerImageReference accepts only explicit repository references
// pinned to a sha256 digest.
func ValidDockerImageReference(value string) bool {
	return dockerImageReferencePattern.MatchString(value)
}

func validateMission(item Mission) error {
	if !missionIDPattern.MatchString(item.ID) {
		return fmt.Errorf("id %q must use lowercase words separated by hyphens", item.ID)
	}
	track := item.EffectiveTrack()
	switch track {
	case TrackLinux, TrackDocker:
	default:
		return fmt.Errorf("unknown track %q", track)
	}
	environment := item.EffectiveEnvironment()
	switch environment {
	case EnvironmentSimulated, EnvironmentDocker:
	default:
		return fmt.Errorf("unknown environment %q", environment)
	}
	if (track == TrackDocker) != (environment == EnvironmentDocker) {
		return fmt.Errorf("track %q cannot use environment %q", track, environment)
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
	if item.Rewards.XP <= 0 || item.Rewards.HintPenalty < 0 {
		return fmt.Errorf("XP must be positive and hint penalty cannot be negative")
	}
	for index, hint := range item.Hints {
		if strings.TrimSpace(hint) == "" {
			return fmt.Errorf("hint %d is empty", index+1)
		}
	}
	if environment == EnvironmentSimulated {
		if item.Docker != nil {
			return fmt.Errorf("simulated mission cannot define docker setup")
		}
		if err := validateAbsolutePath("start_dir", item.StartDir, true); err != nil {
			return err
		}
		if err := validateSetup(item.Setup, item.StartDir); err != nil {
			return err
		}
	} else {
		if item.StartDir != "" {
			return fmt.Errorf("docker mission cannot define start_dir")
		}
		if !setupEmpty(item.Setup) {
			return fmt.Errorf("docker mission cannot define simulated setup")
		}
		if item.Docker == nil {
			return fmt.Errorf("docker mission requires docker setup")
		}
		if err := validateDockerSetup(*item.Docker); err != nil {
			return err
		}
	}
	if len(item.Validation.All) == 0 {
		return fmt.Errorf("at least one validation condition is required")
	}
	for index, condition := range item.Validation.All {
		if err := validateCondition(condition, environment); err != nil {
			return fmt.Errorf("validation condition %d: %w", index+1, err)
		}
		if condition.Type == "docker_container_running" && !dockerSetupHasContainer(item.Docker, condition.Container) {
			return fmt.Errorf("validation condition %d: unknown docker container %q", index+1, condition.Container)
		}
	}
	return nil
}

func setupEmpty(setup Setup) bool {
	return len(setup.Directories) == 0 && len(setup.Files) == 0 && len(setup.Processes) == 0 &&
		len(setup.Environment) == 0 && len(setup.Archives) == 0
}

func validateDockerSetup(setup DockerSetup) error {
	if len(setup.Images) == 0 {
		return fmt.Errorf("docker setup requires at least one image")
	}
	if len(setup.Containers) == 0 {
		return fmt.Errorf("docker setup requires at least one container")
	}
	images := make(map[string]bool, len(setup.Images))
	for _, image := range setup.Images {
		if !ValidDockerLogicalName(image.Alias) {
			return fmt.Errorf("docker image alias %q must be a lowercase logical name", image.Alias)
		}
		if images[image.Alias] {
			return fmt.Errorf("duplicate docker image alias %q", image.Alias)
		}
		if !ValidDockerImageReference(image.Reference) {
			return fmt.Errorf("docker image %q reference must be pinned by sha256 digest", image.Alias)
		}
		images[image.Alias] = true
	}
	containers := make(map[string]bool, len(setup.Containers))
	for _, container := range setup.Containers {
		if !ValidDockerLogicalName(container.Name) {
			return fmt.Errorf("docker container name %q must be a lowercase logical name", container.Name)
		}
		if containers[container.Name] {
			return fmt.Errorf("duplicate docker container name %q", container.Name)
		}
		if !images[container.Image] {
			return fmt.Errorf("docker container %q references unknown image alias %q", container.Name, container.Image)
		}
		switch container.State {
		case "running", "stopped":
		default:
			return fmt.Errorf("docker container %q has unknown state %q", container.Name, container.State)
		}
		containers[container.Name] = true
	}
	return nil
}

func dockerSetupHasContainer(setup *DockerSetup, name string) bool {
	if setup == nil {
		return false
	}
	for _, container := range setup.Containers {
		if container.Name == name {
			return true
		}
	}
	return false
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

func validateCondition(condition Condition, environment string) error {
	if condition.Container != "" && condition.Type != "docker_container_running" {
		return fmt.Errorf("container is not supported for type %q", condition.Type)
	}
	if condition.Count != nil && condition.Type != "docker_container_count_equals" {
		return fmt.Errorf("count is not supported for type %q", condition.Type)
	}
	pathTypes := map[string]bool{
		"file_exists": true, "dir_exists": true, "path_missing": true,
		"file_content_equals": true, "file_content_contains": true,
		"file_lines_equal": true, "file_mode_equals": true, "file_owner_equals": true,
	}
	if pathTypes[condition.Type] {
		if environment != EnvironmentSimulated {
			return fmt.Errorf("type %q requires a simulated environment", condition.Type)
		}
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
		for _, value := range condition.Values {
			if value == "" {
				return fmt.Errorf("values cannot contain an empty string")
			}
		}
	case "cwd_equals":
		if environment != EnvironmentSimulated {
			return fmt.Errorf("cwd_equals requires a simulated environment")
		}
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
		if environment != EnvironmentSimulated {
			return fmt.Errorf("%s requires a simulated environment", condition.Type)
		}
		if condition.PID <= 0 {
			return fmt.Errorf("PID must be positive")
		}
	case "env_equals":
		if environment != EnvironmentSimulated {
			return fmt.Errorf("env_equals requires a simulated environment")
		}
		name, _, found := strings.Cut(condition.Value, "=")
		if !found || !variablePattern.MatchString(name) {
			return fmt.Errorf("value must be NAME=value")
		}
	case "docker_container_running":
		if environment != EnvironmentDocker {
			return fmt.Errorf("docker_container_running requires a docker environment")
		}
		if !ValidDockerLogicalName(condition.Container) {
			return fmt.Errorf("container %q must be a lowercase logical name", condition.Container)
		}
		if condition.Path != "" || condition.Value != "" || len(condition.Values) != 0 || condition.PID != 0 || condition.Count != nil {
			return fmt.Errorf("docker_container_running accepts only container")
		}
	case "docker_container_count_equals":
		if environment != EnvironmentDocker {
			return fmt.Errorf("docker_container_count_equals requires a docker environment")
		}
		if condition.Count == nil || *condition.Count < 0 {
			return fmt.Errorf("count must be a non-negative integer")
		}
		if condition.Path != "" || condition.Value != "" || len(condition.Values) != 0 || condition.PID != 0 || condition.Container != "" {
			return fmt.Errorf("docker_container_count_equals accepts only count")
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

// InTrack returns catalog missions in their global order, filtered to one
// learning track. An empty track selects the backwards-compatible Linux
// default.
func (c Catalog) InTrack(track string) []Mission {
	if track == "" {
		track = TrackLinux
	}
	items := make([]Mission, 0)
	for _, item := range c.missions {
		if item.EffectiveTrack() == track {
			items = append(items, item)
		}
	}
	return items
}

// NextInTrack returns the first incomplete mission in one learning track.
func (c Catalog) NextInTrack(track string, completed func(string) bool) (Mission, bool) {
	for _, item := range c.InTrack(track) {
		if !completed(item.ID) {
			return item, true
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
