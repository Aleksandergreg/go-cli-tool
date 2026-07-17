package sandbox

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/aleksandergregersen/opsquest/internal/mission"
)

type Process struct {
	PID     int
	Command string
	Running bool
}

type Archive struct {
	Entries []mission.ArchiveEntry
}

type Sandbox struct {
	FS        *FileSystem
	CWD       string
	Previous  string
	Env       map[string]string
	Processes map[int]*Process
	Archives  map[string]Archive

	commandTrace []string
}

func New(setup mission.Setup, startDir string) (*Sandbox, error) {
	box := &Sandbox{
		FS:        NewFileSystem(),
		CWD:       path.Clean(startDir),
		Env:       map[string]string{"HOME": "/home/operator", "USER": "operator"},
		Processes: make(map[int]*Process),
		Archives:  make(map[string]Archive),
	}
	for key, value := range setup.Environment {
		box.Env[key] = value
	}
	for _, directory := range setup.Directories {
		mode, err := parseMode(directory.Mode, 0o755)
		if err != nil {
			return nil, fmt.Errorf("directory %s: %w", directory.Path, err)
		}
		if err := box.FS.EnsureDir(directory.Path, mode); err != nil {
			return nil, err
		}
	}
	for _, file := range setup.Files {
		mode, err := parseMode(file.Mode, 0o644)
		if err != nil {
			return nil, fmt.Errorf("file %s: %w", file.Path, err)
		}
		if err := box.FS.EnsureDir(path.Dir(file.Path), 0o755); err != nil {
			return nil, err
		}
		if err := box.FS.WriteFile(file.Path, file.Content, mode); err != nil {
			return nil, err
		}
		if file.Owner != "" {
			_ = box.FS.Chown(file.Path, file.Owner)
		}
	}
	for _, process := range setup.Processes {
		if _, exists := box.Processes[process.PID]; exists {
			return nil, fmt.Errorf("duplicate process PID %d", process.PID)
		}
		item := Process(process)
		box.Processes[process.PID] = &item
	}
	for _, archive := range setup.Archives {
		archivePath := path.Clean(archive.Path)
		if err := box.FS.EnsureDir(path.Dir(archivePath), 0o755); err != nil {
			return nil, err
		}
		if err := box.FS.WriteFile(archivePath, "OpsQuest virtual tar archive\n", 0o644); err != nil {
			return nil, err
		}
		box.Archives[archivePath] = Archive{Entries: archive.Entries}
	}
	if !box.FS.IsDir(box.CWD) {
		return nil, fmt.Errorf("start directory %s does not exist", box.CWD)
	}
	box.Env["PWD"] = box.CWD
	return box, nil
}

func parseMode(value string, fallback uint32) (uint32, error) {
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseUint(value, 8, 12)
	if err != nil {
		return 0, fmt.Errorf("invalid mode %q", value)
	}
	return uint32(parsed), nil
}

func (s *Sandbox) Resolve(name string) string {
	if name == "~" {
		name = s.Env["HOME"]
	} else if strings.HasPrefix(name, "~/") {
		name = path.Join(s.Env["HOME"], strings.TrimPrefix(name, "~/"))
	}
	return Clean(s.CWD, name)
}

func (s *Sandbox) trace() []string {
	commands := make([]string, len(s.commandTrace))
	copy(commands, s.commandTrace)
	return commands
}

func (s *Sandbox) finalDestination(source, destination string) string {
	if s.FS.IsDir(destination) {
		return path.Join(destination, path.Base(source))
	}
	return destination
}

func (s *Sandbox) copyArchiveMetadata(source, destination string) {
	copies := make(map[string]Archive)
	for archivePath, archive := range s.Archives {
		if archivePath == source || strings.HasPrefix(archivePath, source+"/") {
			copies[destination+strings.TrimPrefix(archivePath, source)] = archive
		}
	}
	s.removeArchiveMetadata(destination)
	for archivePath, archive := range copies {
		s.Archives[archivePath] = archive
	}
}

func (s *Sandbox) moveArchiveMetadata(source, destination string) {
	moves := make(map[string]Archive)
	for archivePath, archive := range s.Archives {
		if archivePath == source || strings.HasPrefix(archivePath, source+"/") {
			moves[destination+strings.TrimPrefix(archivePath, source)] = archive
		}
	}
	s.removeArchiveMetadata(destination)
	s.removeArchiveMetadata(source)
	for archivePath, archive := range moves {
		s.Archives[archivePath] = archive
	}
}

func (s *Sandbox) removeArchiveMetadata(target string) {
	for archivePath := range s.Archives {
		if archivePath == target || strings.HasPrefix(archivePath, target+"/") {
			delete(s.Archives, archivePath)
		}
	}
}
