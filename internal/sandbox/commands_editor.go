package sandbox

import (
	"fmt"
	"path"
	"strings"
)

// EditorRequest describes an interactive editor that the game session should
// run. Paths and content are resolved entirely from the virtual filesystem.
type EditorRequest struct {
	Command     string
	Path        string
	DisplayPath string
	Content     string
}

func (s *Sandbox) cmdVi(args []string) (*EditorRequest, error) {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			return nil, fmt.Errorf("options are not supported; usage: vi FILE")
		}
	}
	if len(args) != 1 || args[0] == "" {
		return nil, fmt.Errorf("usage: vi FILE")
	}

	displayPath := args[0]
	resolved := s.Resolve(displayPath)
	content := ""
	if entry, exists := s.FS.Entry(resolved); exists {
		if entry.Kind != Regular {
			return nil, fmt.Errorf("%s: is a directory", displayPath)
		}
		content = entry.Content
	} else if !s.FS.IsDir(path.Dir(resolved)) {
		return nil, fmt.Errorf("%s: no such directory", path.Dir(displayPath))
	}

	return &EditorRequest{
		Command:     "vi",
		Path:        resolved,
		DisplayPath: displayPath,
		Content:     content,
	}, nil
}

// SaveEditorFile writes editor content to the virtual filesystem and removes
// stale virtual-archive metadata when an archive is overwritten. An absolute
// path still refers to the sandbox root, never the host filesystem.
func (s *Sandbox) SaveEditorFile(name, content string) error {
	target := s.Resolve(name)
	if err := s.FS.WriteFile(target, content, 0); err != nil {
		return err
	}
	s.removeArchiveMetadata(target)
	return nil
}
