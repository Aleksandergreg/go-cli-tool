package sandbox

import (
	"fmt"
	"path"
	"strings"
)

func cleanVirtualMutationPath(name string) (string, error) {
	name = path.Clean(name)
	if !strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("virtual path %q must be absolute", name)
	}
	if len(name) > maxVirtualPathBytes {
		return "", fmt.Errorf("virtual path exceeds the %d-byte limit", maxVirtualPathBytes)
	}
	return name, nil
}

func (f *FileSystem) EnsureDir(name string, mode uint32) error {
	var err error
	name, err = cleanVirtualMutationPath(name)
	if err != nil {
		return err
	}
	if name == "/" {
		return nil
	}
	if mode == 0 {
		mode = 0o755
	}

	missing := make([]string, 0)
	for candidate := name; ; candidate = path.Dir(candidate) {
		if entry, ok := f.entries[candidate]; ok {
			if entry.Kind != Directory {
				return fmt.Errorf("%s: not a directory", candidate)
			}
			break
		}
		missing = append(missing, candidate)
		if candidate == "/" {
			return fmt.Errorf("virtual filesystem root is missing")
		}
	}
	if err := f.checkEntryBudget(len(missing)); err != nil {
		return err
	}
	for index := len(missing) - 1; index >= 0; index-- {
		entryMode := uint32(0o755)
		if index == 0 {
			entryMode = mode
		}
		f.entries[missing[index]] = &Entry{Kind: Directory, Mode: entryMode, Owner: "operator"}
	}
	return nil
}

func (f *FileSystem) Mkdir(name string, parents bool, mode uint32) error {
	var err error
	name, err = cleanVirtualMutationPath(name)
	if err != nil {
		return err
	}
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
	if parents {
		return f.EnsureDir(name, mode)
	}
	parent := path.Dir(name)
	if !f.IsDir(parent) {
		return fmt.Errorf("%s: no such directory", parent)
	}
	if err := f.checkEntryBudget(1); err != nil {
		return err
	}
	if mode == 0 {
		mode = 0o755
	}
	f.entries[name] = &Entry{Kind: Directory, Mode: mode, Owner: "operator"}
	return nil
}

func (f *FileSystem) WriteFile(name, content string, mode uint32) error {
	var err error
	name, err = cleanVirtualMutationPath(name)
	if err != nil {
		return err
	}
	if name == "/" {
		return fmt.Errorf("%s: is a directory", name)
	}
	if !f.IsDir(path.Dir(name)) {
		return fmt.Errorf("%s: no such directory", path.Dir(name))
	}
	existing, exists := f.entries[name]
	if exists && existing.Kind == Directory {
		return fmt.Errorf("%s: is a directory", name)
	}
	oldBytes := 0
	if exists {
		oldBytes = len(existing.Content)
	} else if err := f.checkEntryBudget(1); err != nil {
		return err
	}
	if err := f.checkContentBudget(name, oldBytes, len(content)); err != nil {
		return err
	}
	if mode == 0 {
		if exists {
			mode = existing.Mode
		} else {
			mode = 0o644
		}
	}
	owner := "operator"
	if exists && existing.Owner != "" {
		owner = existing.Owner
	}
	f.entries[name] = &Entry{Kind: Regular, Content: content, Mode: mode, Owner: owner}
	return nil
}

func (f *FileSystem) AppendFile(name, content string) error {
	var err error
	name, err = cleanVirtualMutationPath(name)
	if err != nil {
		return err
	}
	if existing, ok := f.entries[name]; ok {
		if existing.Kind != Regular {
			return fmt.Errorf("%s: is a directory", name)
		}
		newBytes := len(existing.Content) + len(content)
		if err := f.checkContentBudget(name, len(existing.Content), newBytes); err != nil {
			return err
		}
		existing.Content += content
		return nil
	}
	return f.WriteFile(name, content, 0o644)
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
	source, destination, err := cleanVirtualMutationPaths(source, destination)
	if err != nil {
		return err
	}
	if source == "/" {
		return fmt.Errorf("refusing to copy /")
	}
	src, ok := f.entries[source]
	if !ok {
		return fmt.Errorf("%s: no such file or directory", source)
	}
	if f.IsDir(destination) {
		destination = path.Join(destination, path.Base(source))
	}
	if source == destination {
		return fmt.Errorf("%s and %s are the same path", source, destination)
	}
	if src.Kind == Directory && !recursive {
		return fmt.Errorf("%s: is a directory (use -r)", source)
	}
	if !f.IsDir(path.Dir(destination)) {
		return fmt.Errorf("%s: no such directory", path.Dir(destination))
	}
	if src.Kind == Directory && strings.HasPrefix(destination+"/", source+"/") {
		return fmt.Errorf("cannot copy a directory into itself")
	}
	items := []string{source}
	if src.Kind == Directory {
		items, err = f.Descendants(source, true)
		if err != nil {
			return err
		}
	}
	type copyItem struct {
		name  string
		entry Entry
	}
	plan := make([]copyItem, 0, len(items))
	additionalEntries := 0
	projectedContent := f.contentBytes()
	for _, oldName := range items {
		rel := strings.TrimPrefix(oldName, source)
		newName := destination + rel
		if _, err := cleanVirtualMutationPath(newName); err != nil {
			return err
		}
		sourceEntry := f.entries[oldName]
		if existing, exists := f.entries[newName]; exists {
			if existing.Kind != sourceEntry.Kind {
				return fmt.Errorf("cannot overwrite %s with %s at %s", existing.Kind, sourceEntry.Kind, newName)
			}
			if existing.Kind == Regular {
				projectedContent -= len(existing.Content)
			}
		} else {
			additionalEntries++
		}
		clone := *sourceEntry
		if clone.Kind == Regular {
			if len(clone.Content) > maxVirtualFileBytes {
				return fmt.Errorf("%s: file exceeds the %d KiB virtual file limit", newName, maxVirtualFileBytes/1024)
			}
			if len(clone.Content) > maxVirtualFileSystemBytes-projectedContent {
				return fmt.Errorf("virtual filesystem content limit of %d MiB exceeded", maxVirtualFileSystemBytes/(1024*1024))
			}
			projectedContent += len(clone.Content)
		}
		plan = append(plan, copyItem{name: newName, entry: clone})
	}
	if err := f.checkEntryBudget(additionalEntries); err != nil {
		return err
	}
	for _, item := range plan {
		clone := item.entry
		f.entries[item.name] = &clone
	}
	return nil
}

func (f *FileSystem) Move(source, destination string) error {
	source, destination, err := cleanVirtualMutationPaths(source, destination)
	if err != nil {
		return err
	}
	if !f.Exists(source) {
		return fmt.Errorf("%s: no such file or directory", source)
	}
	if f.IsDir(destination) {
		destination = path.Join(destination, path.Base(source))
	}
	if source == destination {
		return fmt.Errorf("%s and %s are the same path", source, destination)
	}
	if !f.IsDir(path.Dir(destination)) {
		return fmt.Errorf("%s: no such directory", path.Dir(destination))
	}
	if source == "/" || strings.HasPrefix(destination+"/", source+"/") {
		return fmt.Errorf("cannot move %s to %s", source, destination)
	}
	items, err := f.Descendants(source, true)
	if err != nil {
		return err
	}
	for _, oldName := range items {
		newName := destination + strings.TrimPrefix(oldName, source)
		if _, err := cleanVirtualMutationPath(newName); err != nil {
			return err
		}
	}
	if f.Exists(destination) {
		sourceEntry, _ := f.Entry(source)
		destinationEntry, _ := f.Entry(destination)
		if sourceEntry.Kind != destinationEntry.Kind || destinationEntry.Kind == Directory {
			return fmt.Errorf("cannot replace %s with %s", destinationEntry.Kind, sourceEntry.Kind)
		}
		delete(f.entries, destination)
	}
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
	entry, err := f.mutableEntry(name)
	if err != nil {
		return err
	}
	entry.Mode = mode
	return nil
}

func (f *FileSystem) Chown(name, owner string) error {
	if len(owner) > maxVirtualOwnerBytes {
		return fmt.Errorf("owner exceeds the %d-byte limit", maxVirtualOwnerBytes)
	}
	entry, err := f.mutableEntry(name)
	if err != nil {
		return err
	}
	entry.Owner = owner
	return nil
}

func (f *FileSystem) mutableEntry(name string) (*Entry, error) {
	entry, ok := f.entries[path.Clean(name)]
	if !ok {
		return nil, fmt.Errorf("%s: no such file or directory", name)
	}
	return entry, nil
}

func cleanVirtualMutationPaths(source, destination string) (string, string, error) {
	cleanSource, err := cleanVirtualMutationPath(source)
	if err != nil {
		return "", "", err
	}
	cleanDestination, err := cleanVirtualMutationPath(destination)
	return cleanSource, cleanDestination, err
}
