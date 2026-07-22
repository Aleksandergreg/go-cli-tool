package mission

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"path"
	"regexp"
	"slices"
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

var (
	missionIDPattern            = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	commandNamePattern          = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	variablePattern             = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	dockerLogicalNamePattern    = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
	dockerImageReferencePattern = regexp.MustCompile(`^[a-z0-9]+(?:[._/-][a-z0-9]+)*(?::[A-Za-z0-9_.-]+)?@sha256:[a-f0-9]{64}$`)
)

const (
	maxDockerImagesPerMission     = 16
	maxDockerContainersPerMission = 32
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
	switch item.Difficulty {
	case DifficultyBeginner, DifficultyIntermediate, DifficultyAdvanced:
	default:
		return fmt.Errorf("unknown difficulty %q", item.Difficulty)
	}
	if item.Rewards.XP <= 0 || item.Rewards.HintPenalty < 0 {
		return fmt.Errorf("XP must be positive and hint penalty cannot be negative")
	}
	if len(item.SuggestedCommands) == 0 {
		return fmt.Errorf("at least one suggested command is required")
	}
	seenCommands := make(map[string]bool, len(item.SuggestedCommands))
	for index, command := range item.SuggestedCommands {
		if !commandNamePattern.MatchString(command) {
			return fmt.Errorf("suggested command %d %q must be a lowercase command name", index+1, command)
		}
		if seenCommands[command] {
			return fmt.Errorf("duplicate suggested command %q", command)
		}
		seenCommands[command] = true
	}
	if len(item.Hints) < 1 || len(item.Hints) > 5 {
		return fmt.Errorf("missions require between 1 and 5 hints")
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
		if err := ValidateDockerSetup(*item.Docker); err != nil {
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
		if condition.Type == ConditionDockerContainerRunning && !dockerSetupHasContainer(item.Docker, condition.Container) {
			return fmt.Errorf("validation condition %d: unknown docker container %q", index+1, condition.Container)
		}
	}
	return nil
}

func setupEmpty(setup Setup) bool {
	return len(setup.Directories) == 0 && len(setup.Files) == 0 && len(setup.Processes) == 0 &&
		len(setup.Environment) == 0 && len(setup.Archives) == 0
}

// ValidateDockerSetup validates the declarative Docker fixture contract shared
// by catalog loading and the runtime adapter. It does not contact Docker.
func ValidateDockerSetup(setup DockerSetup) error {
	if len(setup.Images) == 0 {
		return fmt.Errorf("docker setup requires at least one image")
	}
	if len(setup.Images) > maxDockerImagesPerMission {
		return fmt.Errorf("docker setup exceeds the %d-image limit", maxDockerImagesPerMission)
	}
	if len(setup.Containers) == 0 {
		return fmt.Errorf("docker setup requires at least one container")
	}
	if len(setup.Containers) > maxDockerContainersPerMission {
		return fmt.Errorf("docker setup exceeds the %d-container limit", maxDockerContainersPerMission)
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
		case DockerStateRunning, DockerStateStopped:
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

type conditionFields uint8

const (
	conditionPath conditionFields = 1 << iota
	conditionValue
	conditionValues
	conditionPID
	conditionContainer
	conditionCount
)

var allowedConditionFields = map[ConditionType]conditionFields{
	ConditionOutputEquals:              conditionValue,
	ConditionOutputContains:            conditionValue,
	ConditionOutputContainsAll:         conditionValues,
	ConditionOutputNotContains:         conditionValue,
	ConditionCWDEquals:                 conditionValue,
	ConditionFileExists:                conditionPath,
	ConditionDirectoryExists:           conditionPath,
	ConditionPathMissing:               conditionPath,
	ConditionFileContentEquals:         conditionPath | conditionValue,
	ConditionFileContentContains:       conditionPath | conditionValue,
	ConditionFileLinesEqual:            conditionPath | conditionValues,
	ConditionFileModeEquals:            conditionPath | conditionValue,
	ConditionFileOwnerEquals:           conditionPath | conditionValue,
	ConditionProcessStopped:            conditionPID,
	ConditionProcessRunning:            conditionPID,
	ConditionEnvironmentEquals:         conditionValue,
	ConditionDockerContainerRunning:    conditionContainer,
	ConditionDockerContainerCountEqual: conditionCount,
}

var conditionFieldFlags = map[string]conditionFields{
	"path":      conditionPath,
	"value":     conditionValue,
	"values":    conditionValues,
	"pid":       conditionPID,
	"container": conditionContainer,
	"count":     conditionCount,
}

func validateCondition(condition Condition, environment string) error {
	allowed, known := allowedConditionFields[condition.Type]
	if !known {
		return fmt.Errorf("unknown type %q", condition.Type)
	}
	present := condition.present
	for _, field := range []struct {
		name    string
		present bool
		flag    conditionFields
	}{
		{name: "path", present: condition.Path != "", flag: conditionPath},
		{name: "value", present: condition.Value != "", flag: conditionValue},
		{name: "values", present: len(condition.Values) != 0, flag: conditionValues},
		{name: "pid", present: condition.PID != 0, flag: conditionPID},
		{name: "container", present: condition.Container != "", flag: conditionContainer},
		{name: "count", present: condition.Count != nil, flag: conditionCount},
	} {
		if field.present {
			present |= field.flag
		}
		if present&field.flag != 0 && allowed&field.flag == 0 {
			return fmt.Errorf("type %q does not support %s", condition.Type, field.name)
		}
	}
	if allowed&conditionPath != 0 {
		if environment != EnvironmentSimulated {
			return fmt.Errorf("type %q requires a simulated environment", condition.Type)
		}
		if err := validateAbsolutePath(string(condition.Type), condition.Path, condition.Type == ConditionDirectoryExists); err != nil {
			return err
		}
	}
	switch condition.Type {
	case ConditionOutputEquals, ConditionFileExists, ConditionDirectoryExists, ConditionPathMissing, ConditionFileContentEquals:
		return nil
	case ConditionOutputContains, ConditionOutputNotContains, ConditionFileContentContains:
		if condition.Value == "" {
			return fmt.Errorf("value cannot be empty")
		}
	case ConditionOutputContainsAll:
		if len(condition.Values) == 0 {
			return fmt.Errorf("values cannot be empty")
		}
		for _, value := range condition.Values {
			if value == "" {
				return fmt.Errorf("values cannot contain an empty string")
			}
		}
	case ConditionCWDEquals:
		if environment != EnvironmentSimulated {
			return fmt.Errorf("cwd_equals requires a simulated environment")
		}
		return validateAbsolutePath("cwd_equals", condition.Value, true)
	case ConditionFileLinesEqual:
		if len(condition.Values) == 0 {
			return fmt.Errorf("values cannot be empty")
		}
		for _, value := range condition.Values {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("line values cannot be empty")
			}
		}
	case ConditionFileModeEquals:
		return validateMode(condition.Value)
	case ConditionFileOwnerEquals:
		if strings.TrimSpace(condition.Value) == "" {
			return fmt.Errorf("owner cannot be empty")
		}
	case ConditionProcessStopped, ConditionProcessRunning:
		if environment != EnvironmentSimulated {
			return fmt.Errorf("%s requires a simulated environment", condition.Type)
		}
		if condition.PID <= 0 {
			return fmt.Errorf("PID must be positive")
		}
	case ConditionEnvironmentEquals:
		if environment != EnvironmentSimulated {
			return fmt.Errorf("env_equals requires a simulated environment")
		}
		name, _, found := strings.Cut(condition.Value, "=")
		if !found || !variablePattern.MatchString(name) {
			return fmt.Errorf("value must be NAME=value")
		}
	case ConditionDockerContainerRunning:
		if environment != EnvironmentDocker {
			return fmt.Errorf("docker_container_running requires a docker environment")
		}
		if !ValidDockerLogicalName(condition.Container) {
			return fmt.Errorf("container %q must be a lowercase logical name", condition.Container)
		}
	case ConditionDockerContainerCountEqual:
		if environment != EnvironmentDocker {
			return fmt.Errorf("docker_container_count_equals requires a docker environment")
		}
		if condition.Count == nil || *condition.Count < 0 {
			return fmt.Errorf("count must be a non-negative integer")
		}
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

func cloneMission(item Mission) Mission {
	cloned := item
	cloned.SuggestedCommands = slices.Clone(item.SuggestedCommands)
	cloned.Hints = slices.Clone(item.Hints)
	cloned.Setup.Directories = slices.Clone(item.Setup.Directories)
	cloned.Setup.Files = slices.Clone(item.Setup.Files)
	cloned.Setup.Processes = slices.Clone(item.Setup.Processes)
	cloned.Setup.Archives = make([]ArchiveSpec, len(item.Setup.Archives))
	for index, archive := range item.Setup.Archives {
		cloned.Setup.Archives[index] = archive
		cloned.Setup.Archives[index].Entries = slices.Clone(archive.Entries)
	}
	cloned.Setup.Environment = maps.Clone(item.Setup.Environment)
	if item.Docker != nil {
		dockerSetup := *item.Docker
		dockerSetup.Images = slices.Clone(item.Docker.Images)
		dockerSetup.Containers = slices.Clone(item.Docker.Containers)
		cloned.Docker = &dockerSetup
	}
	cloned.Validation.All = make([]Condition, len(item.Validation.All))
	for index, condition := range item.Validation.All {
		cloned.Validation.All[index] = condition
		cloned.Validation.All[index].Values = slices.Clone(condition.Values)
		if condition.Count != nil {
			count := *condition.Count
			cloned.Validation.All[index].Count = &count
		}
	}
	return cloned
}

func cloneMissions(items []Mission) []Mission {
	cloned := make([]Mission, len(items))
	for index, item := range items {
		cloned[index] = cloneMission(item)
	}
	return cloned
}
