package sandbox

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
)

func (s *Sandbox) cmdHistory(args []string) (string, error) {
	if len(args) > 0 {
		return "", fmt.Errorf("does not accept arguments")
	}
	var output commandOutputBuffer
	for index, command := range s.History {
		output.WriteString(fmt.Sprintf("%4s  %s\n", strconv.Itoa(index+1), command))
	}
	return output.Result()
}

func (s *Sandbox) cmdDU(args []string) (string, error) {
	options, paths, err := parseShortOptions(args, "ashb", true)
	if err != nil {
		return "", err
	}
	all := strings.ContainsRune(options, 'a')
	summary := strings.ContainsRune(options, 's')
	human := strings.ContainsRune(options, 'h')
	if all && summary {
		return "", fmt.Errorf("-a and -s cannot be used together")
	}
	if len(paths) == 0 {
		paths = []string{"."}
	}
	type usage struct {
		path string
		size int
	}
	items := make([]usage, 0)
	for _, displayRoot := range paths {
		resolved := s.Resolve(displayRoot)
		entry, exists := s.FS.Entry(resolved)
		if !exists {
			return "", fmt.Errorf("%s: no such file or directory", displayRoot)
		}
		if entry.Kind == Regular {
			items = append(items, usage{path: displayRoot, size: len(entry.Content)})
			continue
		}
		descendants, _ := s.FS.Descendants(resolved, false)
		if all {
			for _, candidate := range descendants {
				candidateEntry, _ := s.FS.Entry(candidate)
				if candidateEntry.Kind == Regular {
					items = append(items, usage{path: displayFindPath(displayRoot, resolved, candidate), size: len(candidateEntry.Content)})
				}
			}
		}
		items = append(items, usage{path: displayRoot, size: s.diskUsage(resolved)})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].path < items[j].path })
	var output commandOutputBuffer
	for _, item := range items {
		size := fmt.Sprintf("%d", item.size)
		if human {
			size = humanSize(item.size)
		}
		output.WriteString(size + "\t" + item.path + "\n")
	}
	return output.Result()
}

func (s *Sandbox) diskUsage(root string) int {
	paths, _ := s.FS.Descendants(root, true)
	total := 0
	for _, candidate := range paths {
		entry, _ := s.FS.Entry(candidate)
		if entry.Kind == Regular {
			total += len(entry.Content)
		}
	}
	return total
}

func humanSize(size int) string {
	switch {
	case size < 1024:
		return fmt.Sprintf("%dB", size)
	case size < 1024*1024:
		return fmt.Sprintf("%.1fK", float64(size)/1024)
	default:
		return fmt.Sprintf("%.1fM", float64(size)/(1024*1024))
	}
}

func (s *Sandbox) cmdStat(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("missing file operand")
	}
	var output commandOutputBuffer
	for _, name := range args {
		resolved := s.Resolve(name)
		entry, exists := s.FS.Entry(resolved)
		if !exists {
			return "", fmt.Errorf("%s: no such file or directory", name)
		}
		kind, prefix, size := "regular file", '-', len(entry.Content)
		if entry.Kind == Directory {
			kind, prefix, size = "directory", 'd', 0
		}
		output.WriteString(fmt.Sprintf("  File: %s\n", name))
		output.WriteString(fmt.Sprintf("  Type: %s\n", kind))
		output.WriteString(fmt.Sprintf("  Size: %d\n", size))
		output.WriteString(fmt.Sprintf("Access: (%04o/%c%s)  Owner: %s\n", entry.Mode, prefix, permissionString(entry.Mode), entry.Owner))
	}
	return output.Result()
}

func cmdPathPart(command string, args []string) (string, error) {
	if len(args) == 0 || len(args) > 2 || command == "dirname" && len(args) != 1 {
		suffix := ""
		if command == "basename" {
			suffix = " [SUFFIX]"
		}
		return "", fmt.Errorf("usage: %s PATH%s", command, suffix)
	}
	if command == "dirname" {
		return path.Dir(path.Clean(args[0])) + "\n", nil
	}
	base := path.Base(path.Clean(args[0]))
	if len(args) == 2 && args[1] != "" {
		base = strings.TrimSuffix(base, args[1])
	}
	return base + "\n", nil
}
