package sandbox

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
)

func (s *Sandbox) cmdPwd(args []string) (string, error) {
	if len(args) > 0 {
		return "", fmt.Errorf("does not accept arguments")
	}
	return s.CWD + "\n", nil
}

func (s *Sandbox) cmdCD(args []string) (string, error) {
	if len(args) > 1 {
		return "", fmt.Errorf("too many arguments")
	}
	target := s.Env["HOME"]
	printTarget := false
	if len(args) == 1 {
		target = args[0]
		if target == "-" {
			var exists bool
			target, exists = s.Env["OLDPWD"]
			if !exists || target == "" {
				return "", fmt.Errorf("OLDPWD is not set")
			}
			printTarget = true
		}
	}
	resolved := s.Resolve(target)
	entry, exists := s.FS.Entry(resolved)
	if !exists {
		return "", fmt.Errorf("%s: no such directory", target)
	}
	if entry.Kind != Directory {
		return "", fmt.Errorf("%s: not a directory", target)
	}
	old := s.CWD
	environment := cloneEnvironment(s.Env)
	environment["OLDPWD"] = old
	environment["PWD"] = resolved
	if err := validateEnvironment(environment); err != nil {
		return "", err
	}
	s.CWD = resolved
	s.Env = environment
	if printTarget {
		return resolved + "\n", nil
	}
	return "", nil
}

func (s *Sandbox) cmdLS(args []string) (string, error) {
	long, showAll := false, false
	var names []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && arg != "-" {
			for _, option := range strings.TrimPrefix(arg, "-") {
				switch option {
				case 'l':
					long = true
				case 'a':
					showAll = true
				default:
					return "", fmt.Errorf("unknown option -%c", option)
				}
			}
		} else {
			names = append(names, arg)
		}
	}
	if len(names) == 0 {
		names = []string{"."}
	}
	var output commandOutputBuffer
	for index, name := range names {
		resolved := s.Resolve(name)
		entry, exists := s.FS.Entry(resolved)
		if !exists {
			return "", fmt.Errorf("%s: no such file or directory", name)
		}
		if len(names) > 1 && entry.Kind == Directory {
			if index > 0 {
				output.WriteByte('\n')
			}
			output.WriteString(name + ":\n")
		}
		items := []string{resolved}
		if entry.Kind == Directory {
			children, _ := s.FS.Children(resolved)
			items = children
		}
		for _, itemPath := range items {
			item, _ := s.FS.Entry(itemPath)
			base := path.Base(itemPath)
			if !showAll && strings.HasPrefix(base, ".") {
				continue
			}
			if long {
				kind := '-'
				if item.Kind == Directory {
					kind = 'd'
				}
				output.WriteString(fmt.Sprintf("%c%s %-8s %6d %s\n", kind, permissionString(item.Mode), item.Owner, len(item.Content), base))
			} else {
				output.WriteString(base + "\n")
			}
		}
	}
	return output.Result()
}

func permissionString(mode uint32) string {
	bits := []struct {
		mask uint32
		char byte
	}{
		{0o400, 'r'}, {0o200, 'w'}, {0o100, 'x'},
		{0o040, 'r'}, {0o020, 'w'}, {0o010, 'x'},
		{0o004, 'r'}, {0o002, 'w'}, {0o001, 'x'},
	}
	result := make([]byte, len(bits))
	for index, bit := range bits {
		result[index] = '-'
		if mode&bit.mask != 0 {
			result[index] = bit.char
		}
	}
	return string(result)
}

func (s *Sandbox) cmdMkdir(args []string) (string, error) {
	parents := false
	var names []string
	for _, arg := range args {
		if arg == "-p" || arg == "--parents" {
			parents = true
		} else if strings.HasPrefix(arg, "-") {
			return "", fmt.Errorf("unknown option %s", arg)
		} else {
			names = append(names, arg)
		}
	}
	if len(names) == 0 {
		return "", fmt.Errorf("missing directory operand")
	}
	for _, name := range names {
		if err := s.FS.Mkdir(s.Resolve(name), parents, 0o755); err != nil {
			return "", err
		}
	}
	return "", nil
}

func (s *Sandbox) cmdTouch(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("missing file operand")
	}
	for _, name := range args {
		resolved := s.Resolve(name)
		if entry, exists := s.FS.Entry(resolved); exists {
			if entry.Kind == Directory {
				continue
			}
			continue
		}
		if err := s.FS.WriteFile(resolved, "", 0o644); err != nil {
			return "", err
		}
	}
	return "", nil
}

func (s *Sandbox) cmdCopy(args []string) (string, error) {
	recursive := false
	var operands []string
	for _, arg := range args {
		if arg == "-r" || arg == "-R" || arg == "--recursive" {
			recursive = true
		} else if strings.HasPrefix(arg, "-") {
			return "", fmt.Errorf("unknown option %s", arg)
		} else {
			operands = append(operands, arg)
		}
	}
	if len(operands) < 2 {
		return "", fmt.Errorf("missing source or destination")
	}
	destination := s.Resolve(operands[len(operands)-1])
	if len(operands) > 2 && !s.FS.IsDir(destination) {
		return "", fmt.Errorf("destination must be a directory when copying multiple files")
	}
	for _, source := range operands[:len(operands)-1] {
		sourcePath := s.Resolve(source)
		if sourcePath == "/" {
			return "", fmt.Errorf("refusing to copy /")
		}
		finalDestination := s.finalDestination(sourcePath, destination)
		archives, err := s.planArchiveCopy(sourcePath, finalDestination)
		if err != nil {
			return "", err
		}
		if err := s.FS.Copy(sourcePath, destination, recursive); err != nil {
			return "", err
		}
		s.Archives = archives
	}
	return "", nil
}

func (s *Sandbox) cmdMove(args []string) (string, error) {
	operands := args
	if len(operands) < 2 {
		return "", fmt.Errorf("missing source or destination")
	}
	destination := s.Resolve(operands[len(operands)-1])
	if len(operands) > 2 && !s.FS.IsDir(destination) {
		return "", fmt.Errorf("destination must be a directory when moving multiple files")
	}
	for _, source := range operands[:len(operands)-1] {
		sourcePath := s.Resolve(source)
		if sourcePath == s.CWD || strings.HasPrefix(s.CWD, sourcePath+"/") {
			return "", fmt.Errorf("cannot move the current directory or one of its parents")
		}
		finalDestination := s.finalDestination(sourcePath, destination)
		if err := s.FS.Move(sourcePath, destination); err != nil {
			return "", err
		}
		s.moveArchiveMetadata(sourcePath, finalDestination)
	}
	return "", nil
}

func (s *Sandbox) cmdRemove(command string, args []string) (string, error) {
	recursive, force := false, false
	var names []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && arg != "-" {
			for _, option := range strings.TrimPrefix(arg, "-") {
				if command == "rmdir" {
					return "", fmt.Errorf("rmdir does not support -%c in this lab", option)
				}
				switch option {
				case 'r', 'R':
					recursive = true
				case 'f':
					force = true
				default:
					return "", fmt.Errorf("unknown option -%c", option)
				}
			}
		} else {
			names = append(names, arg)
		}
	}
	if len(names) == 0 {
		return "", fmt.Errorf("missing operand")
	}
	for _, name := range names {
		resolved := s.Resolve(name)
		if resolved == s.CWD || strings.HasPrefix(s.CWD, resolved+"/") {
			return "", fmt.Errorf("cannot remove the current directory or one of its parents")
		}
		if command == "rmdir" && !s.FS.IsDir(resolved) {
			return "", fmt.Errorf("%s: not a directory", name)
		}
		if err := s.FS.Remove(resolved, recursive, force); err != nil {
			return "", err
		}
		s.removeArchiveMetadata(resolved)
	}
	return "", nil
}

func (s *Sandbox) cmdChmod(args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("usage: chmod MODE FILE...")
	}
	mode, err := strconv.ParseUint(strings.TrimPrefix(args[0], "0o"), 8, 12)
	if err != nil {
		return "", fmt.Errorf("invalid mode %q; use an octal mode such as 750", args[0])
	}
	for _, name := range args[1:] {
		if err := s.FS.Chmod(s.Resolve(name), uint32(mode)); err != nil {
			return "", err
		}
	}
	return "", nil
}

func (s *Sandbox) cmdChown(args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("usage: chown OWNER FILE...")
	}
	owner := strings.Split(args[0], ":")[0]
	if owner == "" {
		return "", fmt.Errorf("owner cannot be empty")
	}
	for _, name := range args[1:] {
		if err := s.FS.Chown(s.Resolve(name), owner); err != nil {
			return "", err
		}
	}
	return "", nil
}

func sortedKeys[V any](items map[string]V) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
