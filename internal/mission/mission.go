package mission

import (
	"encoding/json"
	"fmt"
)

const (
	TrackLinux  = "linux"
	TrackDocker = "docker"

	EnvironmentSimulated = "simulated"
	EnvironmentDocker    = "docker"

	DifficultyBeginner     = "beginner"
	DifficultyIntermediate = "intermediate"
	DifficultyAdvanced     = "advanced"

	DockerStateRunning = "running"
	DockerStateStopped = "stopped"
)

// ConditionType identifies one observable outcome supported by a mission
// environment. Keeping this vocabulary in the mission package prevents
// loaders and environment adapters from inventing subtly different schemas.
type ConditionType string

const (
	ConditionOutputEquals              ConditionType = "output_equals"
	ConditionOutputContains            ConditionType = "output_contains"
	ConditionOutputContainsAll         ConditionType = "output_contains_all"
	ConditionOutputNotContains         ConditionType = "output_not_contains"
	ConditionCWDEquals                 ConditionType = "cwd_equals"
	ConditionFileExists                ConditionType = "file_exists"
	ConditionDirectoryExists           ConditionType = "dir_exists"
	ConditionPathMissing               ConditionType = "path_missing"
	ConditionFileContentEquals         ConditionType = "file_content_equals"
	ConditionFileContentContains       ConditionType = "file_content_contains"
	ConditionFileLinesEqual            ConditionType = "file_lines_equal"
	ConditionFileModeEquals            ConditionType = "file_mode_equals"
	ConditionFileOwnerEquals           ConditionType = "file_owner_equals"
	ConditionProcessStopped            ConditionType = "process_stopped"
	ConditionProcessRunning            ConditionType = "process_running"
	ConditionEnvironmentEquals         ConditionType = "env_equals"
	ConditionDockerContainerRunning    ConditionType = "docker_container_running"
	ConditionDockerContainerCountEqual ConditionType = "docker_container_count_equals"
)

// Mission is a declarative OpsQuest exercise. Setup describes the isolated
// world the player receives; Validation describes the observable outcome that
// completes the mission.
type Mission struct {
	ID                string       `json:"id"`
	Number            int          `json:"number"`
	Track             string       `json:"track,omitempty"`
	Environment       string       `json:"environment,omitempty"`
	Title             string       `json:"title"`
	Campaign          string       `json:"campaign"`
	Difficulty        string       `json:"difficulty"`
	Story             string       `json:"story"`
	Objective         string       `json:"objective"`
	StartDir          string       `json:"start_dir"`
	SuggestedCommands []string     `json:"suggested_commands"`
	Hints             []string     `json:"hints"`
	Explanation       string       `json:"explanation"`
	Setup             Setup        `json:"setup"`
	Docker            *DockerSetup `json:"docker,omitempty"`
	Validation        Validation   `json:"validation"`
	Rewards           Rewards      `json:"rewards"`
}

// EffectiveTrack preserves compatibility with missions written before tracks
// were explicit in the schema.
func (m Mission) EffectiveTrack() string {
	if m.Track == "" {
		return TrackLinux
	}
	return m.Track
}

// EffectiveEnvironment preserves the in-memory sandbox as the default for
// existing mission definitions.
func (m Mission) EffectiveEnvironment() string {
	if m.Environment == "" {
		return EnvironmentSimulated
	}
	return m.Environment
}

type Setup struct {
	Directories []DirectorySpec   `json:"directories"`
	Files       []FileSpec        `json:"files"`
	Processes   []ProcessSpec     `json:"processes"`
	Environment map[string]string `json:"environment"`
	Archives    []ArchiveSpec     `json:"archives"`
}

type DirectorySpec struct {
	Path string `json:"path"`
	Mode string `json:"mode,omitempty"`
}

type FileSpec struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Mode    string `json:"mode,omitempty"`
	Owner   string `json:"owner,omitempty"`
}

type ProcessSpec struct {
	PID     int    `json:"pid"`
	Command string `json:"command"`
	Running bool   `json:"running"`
}

type ArchiveSpec struct {
	Path    string         `json:"path"`
	Entries []ArchiveEntry `json:"entries"`
}

type ArchiveEntry struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Mode    string `json:"mode,omitempty"`
}

// DockerSetup describes logical, attempt-scoped Docker resources. Runtime
// code maps these aliases and names to isolated engine resources.
type DockerSetup struct {
	Images     []DockerImageSpec     `json:"images"`
	Containers []DockerContainerSpec `json:"containers"`
}

type DockerImageSpec struct {
	Alias     string `json:"alias"`
	Reference string `json:"reference"`
}

type DockerContainerSpec struct {
	Name  string `json:"name"`
	Image string `json:"image"`
	State string `json:"state"`
}

type Validation struct {
	All []Condition `json:"all"`
}

type Condition struct {
	Type      ConditionType `json:"type"`
	Path      string        `json:"path,omitempty"`
	Value     string        `json:"value,omitempty"`
	Values    []string      `json:"values,omitempty"`
	PID       int           `json:"pid,omitempty"`
	Container string        `json:"container,omitempty"`
	Count     *int          `json:"count,omitempty"`
	present   conditionFields
}

// UnmarshalJSON retains field presence so validation can reject an unsupported
// field even when its explicit JSON value is empty or zero. Condition owns a
// custom decoder, so it also preserves the catalog's unknown-field rejection.
func (c *Condition) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	present := conditionFields(0)
	for name := range fields {
		if name == "type" {
			continue
		}
		flag, known := conditionFieldFlags[name]
		if !known {
			return fmt.Errorf("json: unknown field %q", name)
		}
		present |= flag
	}
	type wireCondition Condition
	var decoded wireCondition
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*c = Condition(decoded)
	c.present = present
	return nil
}

type Rewards struct {
	XP          int `json:"xp"`
	HintPenalty int `json:"hint_penalty"`
}
