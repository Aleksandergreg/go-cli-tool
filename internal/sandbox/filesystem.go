package sandbox

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

type EntryKind string

const (
	Directory EntryKind = "directory"
	Regular   EntryKind = "file"
)

type Entry struct {
	Kind    EntryKind
	Content string
	Mode    uint32
	Owner   string
}

type FileSystem struct {
	entries map[string]*Entry
}

const (
	maxVirtualEntries         = 4096
	maxVirtualFileBytes       = 2 * 1024 * 1024
	maxVirtualFileSystemBytes = 8 * 1024 * 1024
	maxVirtualPathBytes       = 4096
	maxVirtualOwnerBytes      = 256
)

func NewFileSystem() *FileSystem {
	return &FileSystem{entries: map[string]*Entry{
		"/": {Kind: Directory, Mode: 0o755, Owner: "root"},
	}}
}

func (f *FileSystem) clone() *FileSystem {
	entries := make(map[string]*Entry, len(f.entries))
	for name, entry := range f.entries {
		entryCopy := *entry
		entries[name] = &entryCopy
	}
	return &FileSystem{entries: entries}
}

func (f *FileSystem) commitSnapshot(snapshot *FileSystem) {
	f.entries = snapshot.entries
}

func (f *FileSystem) contentBytes() int {
	total := 0
	for _, entry := range f.entries {
		if entry.Kind == Regular {
			total += len(entry.Content)
		}
	}
	return total
}

func (f *FileSystem) checkEntryBudget(additional int) error {
	if additional > maxVirtualEntries-len(f.entries) {
		return fmt.Errorf("virtual filesystem entry limit of %d exceeded", maxVirtualEntries)
	}
	return nil
}

func (f *FileSystem) checkContentBudget(name string, oldBytes, newBytes int) error {
	if newBytes > maxVirtualFileBytes {
		return fmt.Errorf("%s: file exceeds the %d KiB virtual file limit", name, maxVirtualFileBytes/1024)
	}
	usedWithoutOld := f.contentBytes() - oldBytes
	if newBytes > maxVirtualFileSystemBytes-usedWithoutOld {
		return fmt.Errorf("virtual filesystem content limit of %d MiB exceeded", maxVirtualFileSystemBytes/(1024*1024))
	}
	return nil
}

func Clean(cwd, name string) string {
	if strings.TrimSpace(name) == "" {
		return path.Clean(cwd)
	}
	if strings.HasPrefix(name, "/") {
		return path.Clean(name)
	}
	return path.Clean(path.Join(cwd, name))
}

func (f *FileSystem) Entry(name string) (*Entry, bool) {
	entry, ok := f.entries[path.Clean(name)]
	return entry, ok
}

func (f *FileSystem) Exists(name string) bool {
	_, ok := f.Entry(name)
	return ok
}

func (f *FileSystem) IsDir(name string) bool {
	entry, ok := f.Entry(name)
	return ok && entry.Kind == Directory
}

func (f *FileSystem) Paths() []string {
	paths := make([]string, 0, len(f.entries))
	for name := range f.entries {
		paths = append(paths, name)
	}
	sort.Strings(paths)
	return paths
}
