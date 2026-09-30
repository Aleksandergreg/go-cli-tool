package sandbox

import (
	"fmt"
	"strings"
)

func cloneArchives(source map[string]Archive) map[string]Archive {
	return cloneMap(source)
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
