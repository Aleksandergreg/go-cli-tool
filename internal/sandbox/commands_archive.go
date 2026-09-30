package sandbox

import (
	"fmt"
	"path"
	"strings"

	"github.com/aleksandergregersen/opsquest/internal/mission"
)

func (s *Sandbox) cmdTar(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("missing operation")
	}
	options, err := parseTarArgs(args)
	if err != nil {
		return "", err
	}
	if options.operation == 0 {
		return "", fmt.Errorf("choose one of -x, -c, or -t")
	}
	if options.archiveName == "" {
		return "", fmt.Errorf("archive file is required with -f")
	}
	if options.operation != 'x' && options.destinationSet {
		return "", fmt.Errorf("-C is supported only when extracting an archive")
	}
	archivePath := s.Resolve(options.archiveName)
	destination := s.Resolve(options.destination)
	archive, archiveExists := s.Archives[archivePath]
	if options.operation != 'c' && !archiveExists {
		return "", fmt.Errorf("%s: not a recognized archive", options.archiveName)
	}

	switch options.operation {
	case 'x':
		if !s.FS.IsDir(destination) {
			return "", fmt.Errorf("%s: extraction destination is not a directory", destination)
		}
		type extractionItem struct {
			relative string
			target   string
			content  string
			mode     uint32
		}
		items := make([]extractionItem, 0, len(archive.Entries))
		var output commandOutputBuffer
		for _, item := range archive.Entries {
			relative, err := safeArchivePath(item.Path)
			if err != nil {
				return "", fmt.Errorf("archive entry %q: %w", item.Path, err)
			}
			mode, err := parseMode(item.Mode, 0o644)
			if err != nil {
				return "", err
			}
			items = append(items, extractionItem{
				relative: relative,
				target:   path.Join(destination, relative),
				content:  item.Content,
				mode:     mode,
			})
			if options.verbose {
				output.WriteString(relative + "\n")
			}
		}
		verboseOutput, err := output.Result()
		if err != nil {
			return "", err
		}
		filesystem := s.FS.clone()
		archives := cloneArchives(s.Archives)
		for _, item := range items {
			if err := filesystem.EnsureDir(path.Dir(item.target), 0o755); err != nil {
				return "", err
			}
			if err := filesystem.WriteFile(item.target, item.content, item.mode); err != nil {
				return "", err
			}
			removeArchiveMetadata(archives, item.target)
		}
		// Extraction is published as one virtual-state transaction. A quota,
		// path, or type failure cannot leave a partly restored tree behind.
		s.FS.commitSnapshot(filesystem)
		s.Archives = archives
		return verboseOutput, nil
	case 't':
		var output commandOutputBuffer
		for _, item := range archive.Entries {
			output.WriteString(item.Path + "\n")
		}
		return output.Result()
	case 'c':
		if len(options.operands) == 0 {
			return "", fmt.Errorf("no files given for archive")
		}
		entries := make([]mission.ArchiveEntry, 0)
		for _, operand := range options.operands {
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
		archives, err := s.planArchiveReplacement(archivePath, Archive{Entries: entries})
		if err != nil {
			return "", err
		}
		verboseOutput := ""
		if options.verbose {
			var output commandOutputBuffer
			for _, item := range entries {
				output.WriteString(item.Path + "\n")
			}
			verboseOutput, err = output.Result()
			if err != nil {
				return "", err
			}
		}
		if err := s.FS.EnsureDir(path.Dir(archivePath), 0o755); err != nil {
			return "", err
		}
		if err := s.FS.WriteFile(archivePath, "OpsQuest virtual tar archive\n", 0o644); err != nil {
			return "", err
		}
		// Publish archive metadata only after its backing virtual file exists.
		// Collection and filesystem failures must leave an existing archive
		// untouched and must not create a metadata-only archive.
		s.Archives = archives
		return verboseOutput, nil
	}
	return "", nil
}

type tarOptions struct {
	operation      byte
	archiveName    string
	destination    string
	destinationSet bool
	verbose        bool
	operands       []string
}

func parseTarArgs(args []string) (tarOptions, error) {
	options := tarOptions{destination: "."}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "-C" {
			if options.destinationSet {
				return tarOptions{}, fmt.Errorf("multiple -C options are not supported")
			}
			if index+1 >= len(args) {
				return tarOptions{}, fmt.Errorf("-C requires a directory")
			}
			index++
			options.destination = args[index]
			options.destinationSet = true
			continue
		}
		isOptionGroup := strings.HasPrefix(arg, "-") || index == 0 && len(arg) > 0 && strings.ContainsRune("xct", rune(arg[0]))
		if !isOptionGroup || arg == "-" {
			options.operands = append(options.operands, arg)
			continue
		}
		group := strings.TrimPrefix(arg, "-")
		if group == "" {
			return tarOptions{}, fmt.Errorf("empty option group")
		}
		for optionIndex := 0; optionIndex < len(group); optionIndex++ {
			option := group[optionIndex]
			switch option {
			case 'x', 'c', 't':
				if options.operation != 0 && options.operation != option {
					return tarOptions{}, fmt.Errorf("choose exactly one of -x, -c, or -t")
				}
				options.operation = option
			case 'z':
				// Archives are represented virtually, so compression is transparent.
			case 'v':
				options.verbose = true
			case 'f':
				if optionIndex+1 < len(group) {
					options.archiveName = group[optionIndex+1:]
					optionIndex = len(group)
					continue
				}
				if index+1 >= len(args) {
					return tarOptions{}, fmt.Errorf("-f requires an archive")
				}
				index++
				options.archiveName = args[index]
			default:
				return tarOptions{}, fmt.Errorf("unknown option %c", option)
			}
		}
	}
	return options, nil
}

func safeArchivePath(name string) (string, error) {
	if name == "" || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("path must be relative")
	}
	cleaned := path.Clean(name)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("path escapes the extraction directory")
	}
	return cleaned, nil
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
