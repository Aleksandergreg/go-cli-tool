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

func NewFileSystem() *FileSystem {
	return &FileSystem{entries: map[string]*Entry{
		"/": {Kind: Directory, Mode: 0o755, Owner: "root"},
	}}
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

func (f *FileSystem) EnsureDir(name string, mode uint32) error {
	name = path.Clean(name)
	if name == "/" {
		return nil
	}
	if entry, ok := f.entries[name]; ok {
		if entry.Kind != Directory {
			return fmt.Errorf("%s: not a directory", name)
		}
		return nil
	}
	if err := f.EnsureDir(path.Dir(name), 0o755); err != nil {
		return err
	}
	if mode == 0 {
		mode = 0o755
	}
	f.entries[name] = &Entry{Kind: Directory, Mode: mode, Owner: "operator"}
	return nil
}

func (f *FileSystem) Mkdir(name string, parents bool, mode uint32) error {
	name = path.Clean(name)
	if name == "/" {
		if parents {
			return nil
		}
		return fmt.Errorf("%s: file exists", name)
	}
	if _, ok := f.entries[name]; ok {
		if parents && f.IsDir(name) {
			return nil
		}
		return fmt.Errorf("%s: file exists", name)
	}
	parent := path.Dir(name)
	if !f.IsDir(parent) {
		if !parents {
			return fmt.Errorf("%s: no such directory", parent)
		}
		if err := f.EnsureDir(parent, 0o755); err != nil {
			return err
		}
	}
	if mode == 0 {
		mode = 0o755
	}
	f.entries[name] = &Entry{Kind: Directory, Mode: mode, Owner: "operator"}
	return nil
}

func (f *FileSystem) WriteFile(name, content string, mode uint32) error {
	name = path.Clean(name)
	if name == "/" {
		return fmt.Errorf("%s: is a directory", name)
	}
	if !f.IsDir(path.Dir(name)) {
		return fmt.Errorf("%s: no such directory", path.Dir(name))
	}
	if existing, ok := f.entries[name]; ok && existing.Kind == Directory {
		return fmt.Errorf("%s: is a directory", name)
	}
	if mode == 0 {
		if existing, ok := f.entries[name]; ok {
			mode = existing.Mode
		} else {
			mode = 0o644
		}
	}
	owner := "operator"
	if existing, ok := f.entries[name]; ok && existing.Owner != "" {
		owner = existing.Owner
	}
	f.entries[name] = &Entry{Kind: Regular, Content: content, Mode: mode, Owner: owner}
	return nil
}

func (f *FileSystem) AppendFile(name, content string) error {
	name = path.Clean(name)
	if existing, ok := f.entries[name]; ok {
		if existing.Kind != Regular {
			return fmt.Errorf("%s: is a directory", name)
		}
		existing.Content += content
		return nil
	}
	return f.WriteFile(name, content, 0o644)
}

func (f *FileSystem) ReadFile(name string) (string, error) {
	name = path.Clean(name)
	entry, ok := f.entries[name]
	if !ok {
		return "", fmt.Errorf("%s: no such file", name)
	}
	if entry.Kind != Regular {
		return "", fmt.Errorf("%s: is a directory", name)
	}
	return entry.Content, nil
}

func (f *FileSystem) Children(name string) ([]string, error) {
	name = path.Clean(name)
	if !f.IsDir(name) {
		if f.Exists(name) {
			return nil, fmt.Errorf("%s: not a directory", name)
		}
		return nil, fmt.Errorf("%s: no such directory", name)
	}
	children := make([]string, 0)
	for candidate := range f.entries {
		if candidate != name && path.Dir(candidate) == name {
			children = append(children, candidate)
		}
	}
	sort.Strings(children)
	return children, nil
}

func (f *FileSystem) Descendants(name string, includeRoot bool) ([]string, error) {
	name = path.Clean(name)
	if !f.Exists(name) {
		return nil, fmt.Errorf("%s: no such file or directory", name)
	}
	items := make([]string, 0)
	for candidate := range f.entries {
		if candidate == name {
			if includeRoot {
				items = append(items, candidate)
			}
			continue
		}
		if name == "/" || strings.HasPrefix(candidate, name+"/") {
			items = append(items, candidate)
		}
	}
	sort.Strings(items)
	return items, nil
}

func (f *FileSystem) Remove(name string, recursive, force bool) error {
	name = path.Clean(name)
	if name == "/" {
		return fmt.Errorf("refusing to remove /")
	}
	entry, ok := f.entries[name]
	if !ok {
		if force {
			return nil
		}
		return fmt.Errorf("%s: no such file or directory", name)
	}
	if entry.Kind == Directory {
		children, _ := f.Children(name)
		if len(children) > 0 && !recursive {
			return fmt.Errorf("%s: directory not empty", name)
		}
		if recursive {
			items, _ := f.Descendants(name, true)
			for i := len(items) - 1; i >= 0; i-- {
				delete(f.entries, items[i])
			}
			return nil
		}
	}
	delete(f.entries, name)
	return nil
}

func (f *FileSystem) Copy(source, destination string, recursive bool) error {
	source, destination = path.Clean(source), path.Clean(destination)
	src, ok := f.entries[source]
	if !ok {
		return fmt.Errorf("%s: no such file or directory", source)
	}
	if f.IsDir(destination) {
		destination = path.Join(destination, path.Base(source))
	}
	if src.Kind == Directory && !recursive {
		return fmt.Errorf("%s: is a directory (use -r)", source)
	}
	if !f.IsDir(path.Dir(destination)) {
		return fmt.Errorf("%s: no such directory", path.Dir(destination))
	}
	if existing, exists := f.entries[destination]; exists && existing.Kind != src.Kind {
		return fmt.Errorf("cannot overwrite %s with %s", existing.Kind, src.Kind)
	}
	if src.Kind == Regular {
		clone := *src
		f.entries[destination] = &clone
		return nil
	}
	if strings.HasPrefix(destination+"/", source+"/") {
		return fmt.Errorf("cannot copy a directory into itself")
	}
	items, _ := f.Descendants(source, true)
	for _, oldName := range items {
		rel := strings.TrimPrefix(oldName, source)
		newName := destination + rel
		if existing, exists := f.entries[newName]; exists && existing.Kind != f.entries[oldName].Kind {
			return fmt.Errorf("cannot overwrite %s with %s at %s", existing.Kind, f.entries[oldName].Kind, newName)
		}
	}
	for _, oldName := range items {
		rel := strings.TrimPrefix(oldName, source)
		newName := destination + rel
		clone := *f.entries[oldName]
		f.entries[newName] = &clone
	}
	return nil
}

func (f *FileSystem) Move(source, destination string) error {
	source, destination = path.Clean(source), path.Clean(destination)
	if !f.Exists(source) {
		return fmt.Errorf("%s: no such file or directory", source)
	}
	if f.IsDir(destination) {
		destination = path.Join(destination, path.Base(source))
	}
	if !f.IsDir(path.Dir(destination)) {
		return fmt.Errorf("%s: no such directory", path.Dir(destination))
	}
	if source == "/" || strings.HasPrefix(destination+"/", source+"/") {
		return fmt.Errorf("cannot move %s to %s", source, destination)
	}
	if f.Exists(destination) {
		sourceEntry, _ := f.Entry(source)
		destinationEntry, _ := f.Entry(destination)
		if sourceEntry.Kind != destinationEntry.Kind || destinationEntry.Kind == Directory {
			return fmt.Errorf("cannot replace %s with %s", destinationEntry.Kind, sourceEntry.Kind)
		}
		delete(f.entries, destination)
	}
	items, _ := f.Descendants(source, true)
	for _, oldName := range items {
		rel := strings.TrimPrefix(oldName, source)
		f.entries[destination+rel] = f.entries[oldName]
	}
	for i := len(items) - 1; i >= 0; i-- {
		delete(f.entries, items[i])
	}
	return nil
}

func (f *FileSystem) Chmod(name string, mode uint32) error {
	entry, ok := f.entries[path.Clean(name)]
	if !ok {
		return fmt.Errorf("%s: no such file or directory", name)
	}
	entry.Mode = mode
	return nil
}

func (f *FileSystem) Chown(name, owner string) error {
	entry, ok := f.entries[path.Clean(name)]
	if !ok {
		return fmt.Errorf("%s: no such file or directory", name)
	}
	entry.Owner = owner
	return nil
}

func (f *FileSystem) Glob(cwd, pattern string) []string {
	absPattern := Clean(cwd, pattern)
	matches := make([]string, 0)
	for candidate := range f.entries {
		matched, err := path.Match(absPattern, candidate)
		if err == nil && matched {
			if strings.HasPrefix(pattern, "/") {
				matches = append(matches, candidate)
			} else {
				relative := relativePath(cwd, candidate)
				if strings.HasPrefix(pattern, "./") && !strings.HasPrefix(relative, ".") {
					relative = "./" + relative
				}
				matches = append(matches, relative)
			}
		}
	}
	sort.Strings(matches)
	return matches
}

func relativePath(base, target string) string {
	base, target = path.Clean(base), path.Clean(target)
	if target == base {
		return "."
	}
	baseParts := strings.Split(strings.TrimPrefix(base, "/"), "/")
	targetParts := strings.Split(strings.TrimPrefix(target, "/"), "/")
	if base == "/" {
		baseParts = nil
	}
	if target == "/" {
		targetParts = nil
	}
	common := 0
	for common < len(baseParts) && common < len(targetParts) && baseParts[common] == targetParts[common] {
		common++
	}
	parts := make([]string, 0, len(baseParts)-common+len(targetParts)-common)
	for range baseParts[common:] {
		parts = append(parts, "..")
	}
	parts = append(parts, targetParts[common:]...)
	if len(parts) == 0 {
		return "."
	}
	return strings.Join(parts, "/")
}
