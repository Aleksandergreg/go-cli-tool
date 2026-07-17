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
	all, summary, human := false, false, false
	paths := make([]string, 0)
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && arg != "-" {
			for _, option := range strings.TrimPrefix(arg, "-") {
				switch option {
				case 'a':
					all = true
				case 's':
					summary = true
				case 'h':
					human = true
				case 'b':
					// The virtual filesystem reports bytes by default.
				default:
					return "", fmt.Errorf("unknown option -%c", option)
				}
			}
		} else {
			paths = append(paths, arg)
		}
	}
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
			items = append(items, usage{path: displayRoot, size: len([]byte(entry.Content))})
			continue
		}
		descendants, _ := s.FS.Descendants(resolved, false)
		if all {
			for _, candidate := range descendants {
				candidateEntry, _ := s.FS.Entry(candidate)
				if candidateEntry.Kind == Regular {
					items = append(items, usage{path: displayDescendant(displayRoot, resolved, candidate), size: len([]byte(candidateEntry.Content))})
				}
			}
		}
		items = append(items, usage{path: displayRoot, size: s.diskUsage(resolved)})
	}
	if summary {
		// The default output is already one summary per requested path.
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
			total += len([]byte(entry.Content))
		}
	}
	return total
}

func displayDescendant(root, rootAbs, candidate string) string {
	if strings.HasPrefix(root, "/") {
		return candidate
	}
	return strings.TrimSuffix(root, "/") + strings.TrimPrefix(candidate, rootAbs)
}

func humanSize(size int) string {
	if size < 1024 {
		return fmt.Sprintf("%dB", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%.1fK", float64(size)/1024)
	}
	return fmt.Sprintf("%.1fM", float64(size)/(1024*1024))
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
		kind, prefix, size := "regular file", '-', len([]byte(entry.Content))
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
		return "", fmt.Errorf("usage: %s PATH%s", command, map[bool]string{true: " [SUFFIX]"}[command == "basename"])
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
