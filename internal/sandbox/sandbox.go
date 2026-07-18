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

const (
	maxVirtualArchiveEntries     = maxVirtualEntries
	maxVirtualArchiveBytes       = maxVirtualFileSystemBytes
	maxVirtualEnvironmentEntries = 256
	maxVirtualEnvironmentBytes   = 256 * 1024
)

type Sandbox struct {
	FS        *FileSystem
	CWD       string
	Env       map[string]string
	Processes map[int]*Process
	Archives  map[string]Archive
	History   []string
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
		if err := box.FS.Chmod(directory.Path, mode); err != nil {
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
			if err := box.FS.Chown(file.Path, file.Owner); err != nil {
				return nil, fmt.Errorf("file %s: %w", file.Path, err)
			}
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
		archives, err := box.planArchiveReplacement(archivePath, Archive{Entries: archive.Entries})
		if err != nil {
			return nil, err
		}
		if err := box.FS.EnsureDir(path.Dir(archivePath), 0o755); err != nil {
			return nil, err
		}
		if err := box.FS.WriteFile(archivePath, "OpsQuest virtual tar archive\n", 0o644); err != nil {
			return nil, err
		}
		box.Archives = archives
	}
	if !box.FS.IsDir(box.CWD) {
		return nil, fmt.Errorf("start directory %s does not exist", box.CWD)
	}
	box.Env["PWD"] = box.CWD
	if err := validateEnvironment(box.Env); err != nil {
		return nil, err
	}
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

func (s *Sandbox) finalDestination(source, destination string) string {
	if s.FS.IsDir(destination) {
		return path.Join(destination, path.Base(source))
	}
	return destination
}

func cloneArchives(source map[string]Archive) map[string]Archive {
	clone := make(map[string]Archive, len(source))
	for archivePath, archive := range source {
		clone[archivePath] = archive
	}
	return clone
}

func validateEnvironment(environment map[string]string) error {
	if len(environment) > maxVirtualEnvironmentEntries {
		return fmt.Errorf("virtual environment entry limit of %d exceeded", maxVirtualEnvironmentEntries)
	}
	total := 0
	for name, value := range environment {
		entryBytes := len(name) + len(value)
		if entryBytes > maxVirtualEnvironmentBytes-total {
			return fmt.Errorf("virtual environment size limit of %d KiB exceeded", maxVirtualEnvironmentBytes/1024)
		}
		total += entryBytes
	}
	return nil
}

func cloneEnvironment(source map[string]string) map[string]string {
	clone := make(map[string]string, len(source))
	for name, value := range source {
		clone[name] = value
	}
	return clone
}

func validateArchiveMetadata(archives map[string]Archive) error {
	entryCount := 0
	contentBytes := 0
	for archivePath, archive := range archives {
		for _, entry := range archive.Entries {
			if entryCount == maxVirtualArchiveEntries {
				return fmt.Errorf("virtual archive entry limit of %d exceeded", maxVirtualArchiveEntries)
			}
			entryCount++
			if len(entry.Content) > maxVirtualFileBytes {
				return fmt.Errorf("%s: archive entry %q exceeds the %d KiB content limit", archivePath, entry.Path, maxVirtualFileBytes/1024)
			}
			if len(entry.Content) > maxVirtualArchiveBytes-contentBytes {
				return fmt.Errorf("virtual archive content limit of %d MiB exceeded", maxVirtualArchiveBytes/(1024*1024))
			}
			contentBytes += len(entry.Content)
		}
	}
	return nil
}

func (s *Sandbox) planArchiveReplacement(archivePath string, archive Archive) (map[string]Archive, error) {
	archives := cloneArchives(s.Archives)
	archives[archivePath] = archive
	if err := validateArchiveMetadata(archives); err != nil {
		return nil, err
	}
	return archives, nil
}

func (s *Sandbox) planArchiveCopy(source, destination string) (map[string]Archive, error) {
	archives := cloneArchives(s.Archives)
	copies := make(map[string]Archive)
	for archivePath, archive := range s.Archives {
		if archivePath == source || strings.HasPrefix(archivePath, source+"/") {
			copies[destination+strings.TrimPrefix(archivePath, source)] = archive
		}
	}
	paths, err := s.FS.Descendants(source, true)
	if err != nil {
		return nil, err
	}
	for _, sourcePath := range paths {
		relative := strings.TrimPrefix(sourcePath, source)
		delete(archives, destination+relative)
	}
	for archivePath, archive := range copies {
		archives[archivePath] = archive
	}
	if err := validateArchiveMetadata(archives); err != nil {
		return nil, err
	}
	return archives, nil
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
	removeArchiveMetadata(s.Archives, target)
}

func removeArchiveMetadata(archives map[string]Archive, target string) {
	for archivePath := range archives {
		if archivePath == target || strings.HasPrefix(archivePath, target+"/") {
			delete(archives, archivePath)
		}
	}
}
