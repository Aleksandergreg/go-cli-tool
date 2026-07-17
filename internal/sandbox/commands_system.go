package sandbox

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/aleksandergregersen/opsquest/internal/mission"
)

func (s *Sandbox) cmdFind(args []string) (string, error) {
	var roots []string
	index := 0
	for index < len(args) && !strings.HasPrefix(args[index], "-") {
		roots = append(roots, args[index])
		index++
	}
	if len(roots) == 0 {
		roots = []string{"."}
	}

	namePattern := ""
	caseInsensitive := false
	entryType := ""
	var execArgs []string
	for index < len(args) {
		switch args[index] {
		case "-name", "-iname":
			caseInsensitive = args[index] == "-iname"
			if index+1 >= len(args) {
				return "", fmt.Errorf("%s requires a pattern", args[index])
			}
			index++
			namePattern = args[index]
		case "-type":
			if index+1 >= len(args) {
				return "", fmt.Errorf("-type requires f or d")
			}
			index++
			entryType = args[index]
			if entryType != "f" && entryType != "d" {
				return "", fmt.Errorf("unsupported type %q", entryType)
			}
		case "-print":
			// Printing is the default action.
		case "-exec":
			index++
			start := index
			for index < len(args) && args[index] != ";" {
				index++
			}
			if index >= len(args) {
				return "", fmt.Errorf("-exec must end with \\;")
			}
			execArgs = append([]string(nil), args[start:index]...)
			if len(execArgs) == 0 {
				return "", fmt.Errorf("-exec requires a command")
			}
		default:
			return "", fmt.Errorf("unsupported expression %s", args[index])
		}
		index++
	}

	var output strings.Builder
	for _, root := range roots {
		rootAbs := s.Resolve(root)
		items, err := s.FS.Descendants(rootAbs, true)
		if err != nil {
			return "", err
		}
		for _, candidate := range items {
			entry, _ := s.FS.Entry(candidate)
			if entryType == "f" && entry.Kind != Regular {
				continue
			}
			if entryType == "d" && entry.Kind != Directory {
				continue
			}
			if namePattern != "" {
				candidateName, pattern := path.Base(candidate), namePattern
				if caseInsensitive {
					candidateName, pattern = strings.ToLower(candidateName), strings.ToLower(pattern)
				}
				matched, err := path.Match(pattern, candidateName)
				if err != nil {
					return "", fmt.Errorf("invalid name pattern: %w", err)
				}
				if !matched {
					continue
				}
			}
			display := displayFindPath(root, rootAbs, candidate)
			if len(execArgs) == 0 {
				output.WriteString(display + "\n")
				continue
			}
			command := make([]string, len(execArgs))
			for position, arg := range execArgs {
				if arg == "{}" {
					command[position] = display
				} else {
					command[position] = arg
				}
			}
			result, err := s.run(command, "")
			if err != nil {
				return "", fmt.Errorf("-exec %s: %w", command[0], err)
			}
			output.WriteString(result)
		}
	}
	return output.String(), nil
}

func displayFindPath(root, rootAbs, candidate string) string {
	if strings.HasPrefix(root, "/") {
		return candidate
	}
	rel := strings.TrimPrefix(candidate, rootAbs)
	if rel == "" {
		return path.Clean(root)
	}
	if root == "." || root == "./" {
		return "." + rel
	}
	return strings.TrimSuffix(root, "/") + rel
}

func (s *Sandbox) cmdPS(args []string) (string, error) {
	for _, arg := range args {
		if arg != "-e" && arg != "-ef" && arg != "aux" && arg != "-A" {
			return "", fmt.Errorf("unsupported option %s", arg)
		}
	}
	pids := make([]int, 0, len(s.Processes))
	for pid, process := range s.Processes {
		if process.Running {
			pids = append(pids, pid)
		}
	}
	sort.Ints(pids)
	var output strings.Builder
	output.WriteString("  PID COMMAND\n")
	for _, pid := range pids {
		output.WriteString(fmt.Sprintf("%5d %s\n", pid, s.Processes[pid].Command))
	}
	return output.String(), nil
}

func (s *Sandbox) cmdKill(args []string) (string, error) {
	var pids []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			signal := strings.TrimPrefix(arg, "-")
			if signal != "9" && signal != "15" && signal != "TERM" && signal != "KILL" {
				return "", fmt.Errorf("unsupported signal %s", arg)
			}
			continue
		}
		pids = append(pids, arg)
	}
	if len(pids) == 0 {
		return "", fmt.Errorf("missing PID")
	}
	for _, value := range pids {
		pid, err := strconv.Atoi(value)
		if err != nil {
			return "", fmt.Errorf("%s: PID must be a number", value)
		}
		process, exists := s.Processes[pid]
		if !exists || !process.Running {
			return "", fmt.Errorf("%d: no such process", pid)
		}
		process.Running = false
	}
	return "", nil
}

func (s *Sandbox) cmdTar(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("missing operation")
	}
	operation := byte(0)
	archiveName := ""
	destination := s.CWD
	var operands []string

	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "-C" {
			if index+1 >= len(args) {
				return "", fmt.Errorf("-C requires a directory")
			}
			index++
			destination = s.Resolve(args[index])
			continue
		}
		if strings.HasPrefix(arg, "-") || (index == 0 && strings.ContainsAny(arg, "xctf")) {
			options := strings.TrimPrefix(arg, "-")
			for optionIndex, option := range options {
				switch option {
				case 'x', 'c', 't':
					operation = byte(option)
				case 'z', 'v':
					// Compression and verbose flags do not change the virtual archive format.
				case 'f':
					if optionIndex != len(options)-1 {
						archiveName = options[optionIndex+1:]
						break
					}
					if index+1 >= len(args) {
						return "", fmt.Errorf("-f requires an archive")
					}
					index++
					archiveName = args[index]
				default:
					return "", fmt.Errorf("unknown option %c", option)
				}
			}
			continue
		}
		operands = append(operands, arg)
	}
	if operation == 0 {
		return "", fmt.Errorf("choose one of -x, -c, or -t")
	}
	if archiveName == "" {
		return "", fmt.Errorf("archive file is required with -f")
	}
	archivePath := s.Resolve(archiveName)

	switch operation {
	case 'x':
		archive, exists := s.Archives[archivePath]
		if !exists {
			return "", fmt.Errorf("%s: not a recognized archive", archiveName)
		}
		if !s.FS.IsDir(destination) {
			return "", fmt.Errorf("%s: extraction destination is not a directory", destination)
		}
		for _, item := range archive.Entries {
			target := path.Join(destination, item.Path)
			if err := s.FS.EnsureDir(path.Dir(target), 0o755); err != nil {
				return "", err
			}
			mode, err := parseMode(item.Mode, 0o644)
			if err != nil {
				return "", err
			}
			if err := s.FS.WriteFile(target, item.Content, mode); err != nil {
				return "", err
			}
		}
		return "", nil
	case 't':
		archive, exists := s.Archives[archivePath]
		if !exists {
			return "", fmt.Errorf("%s: not a recognized archive", archiveName)
		}
		var output strings.Builder
		for _, item := range archive.Entries {
			output.WriteString(item.Path + "\n")
		}
		return output.String(), nil
	case 'c':
		if len(operands) == 0 {
			return "", fmt.Errorf("no files given for archive")
		}
		entries := make([]mission.ArchiveEntry, 0)
		for _, operand := range operands {
			resolved := s.Resolve(operand)
			paths, err := s.FS.Descendants(resolved, true)
			if err != nil {
				return "", err
			}
			for _, candidate := range paths {
				entry, _ := s.FS.Entry(candidate)
				if entry.Kind != Regular {
					continue
				}
				relative := path.Join(path.Base(resolved), strings.TrimPrefix(candidate, resolved))
				entries = append(entries, mission.ArchiveEntry{Path: relative, Content: entry.Content, Mode: fmt.Sprintf("%04o", entry.Mode)})
			}
		}
		s.Archives[archivePath] = Archive{Entries: entries}
		if err := s.FS.EnsureDir(path.Dir(archivePath), 0o755); err != nil {
			return "", err
		}
		if err := s.FS.WriteFile(archivePath, "OpsQuest virtual tar archive\n", 0o644); err != nil {
			return "", err
		}
		return "", nil
	}
	return "", nil
}

func (s *Sandbox) cmdGzip(command string, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("missing file operand")
	}
	for _, name := range args {
		source := s.Resolve(name)
		target := source + ".gz"
		if command == "gunzip" {
			if !strings.HasSuffix(source, ".gz") {
				return "", fmt.Errorf("%s: filename does not end in .gz", name)
			}
			target = strings.TrimSuffix(source, ".gz")
		}
		if err := s.FS.Move(source, target); err != nil {
			return "", err
		}
		s.moveArchiveMetadata(source, target)
	}
	return "", nil
}

func (s *Sandbox) cmdExport(args []string) (string, error) {
	if len(args) == 0 {
		return s.cmdEnv(nil)
	}
	for _, assignment := range args {
		key, value, found := strings.Cut(assignment, "=")
		if !found || !validVariableName(key) {
			return "", fmt.Errorf("expected NAME=value, got %q", assignment)
		}
		s.Env[key] = value
	}
	return "", nil
}

func (s *Sandbox) cmdEnv(args []string) (string, error) {
	if len(args) > 0 {
		return "", fmt.Errorf("running a command through env is not supported in this lab")
	}
	var output strings.Builder
	for _, key := range sortedKeys(s.Env) {
		output.WriteString(key + "=" + s.Env[key] + "\n")
	}
	return output.String(), nil
}

func validVariableName(name string) bool {
	if name == "" {
		return false
	}
	for index, char := range name {
		if !(char == '_' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || index > 0 && char >= '0' && char <= '9') {
			return false
		}
	}
	return true
}
