package mission

import (
	"fmt"
	"path"
	"strconv"
	"strings"
)

func setupEmpty(setup Setup) bool {
	return len(setup.Directories) == 0 && len(setup.Files) == 0 && len(setup.Processes) == 0 &&
		len(setup.Environment) == 0 && len(setup.Archives) == 0
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
