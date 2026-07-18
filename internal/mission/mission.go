package mission

const (
	TrackLinux  = "linux"
	TrackDocker = "docker"

	EnvironmentSimulated = "simulated"
	EnvironmentDocker    = "docker"
)

// Mission is a declarative OpsQuest exercise. Setup describes the isolated
// world the player receives; Validation describes the observable outcome that
// completes the mission.
type Mission struct {
	ID          string       `json:"id"`
	Number      int          `json:"number"`
	Track       string       `json:"track,omitempty"`
	Environment string       `json:"environment,omitempty"`
	Title       string       `json:"title"`
	Campaign    string       `json:"campaign"`
	Difficulty  string       `json:"difficulty"`
	Story       string       `json:"story"`
	Objective   string       `json:"objective"`
	StartDir    string       `json:"start_dir"`
	Hints       []string     `json:"hints"`
	Explanation string       `json:"explanation"`
	Setup       Setup        `json:"setup"`
	Docker      *DockerSetup `json:"docker,omitempty"`
	Validation  Validation   `json:"validation"`
	Rewards     Rewards      `json:"rewards"`
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
	Type      string   `json:"type"`
	Path      string   `json:"path,omitempty"`
	Value     string   `json:"value,omitempty"`
	Values    []string `json:"values,omitempty"`
	PID       int      `json:"pid,omitempty"`
	Container string   `json:"container,omitempty"`
	Count     *int     `json:"count,omitempty"`
}

type Rewards struct {
	XP          int `json:"xp"`
	HintPenalty int `json:"hint_penalty"`
}
