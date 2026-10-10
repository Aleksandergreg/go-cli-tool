package sandbox

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

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

func (f *FileSystem) Glob(cwd, pattern string) []string {
	absPattern := Clean(cwd, pattern)
	matches := make([]string, 0)
	for candidate := range f.entries {
		matched, err := matchShellPattern(absPattern, candidate)
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

// matchShellPattern is path.Match with the shell's [!...] negated bracket
// spelling, which path.Match only understands as [^...].
func matchShellPattern(pattern, name string) (bool, error) {
	if strings.Contains(pattern, "[!") {
		runes := []rune(pattern)
		for index := 0; index < len(runes); index++ {
			switch {
			case runes[index] == '\\':
				index++
			case runes[index] == '[' && index+1 < len(runes) && runes[index+1] == '!':
				runes[index+1] = '^'
			}
		}
		pattern = string(runes)
	}
	return path.Match(pattern, name)
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
