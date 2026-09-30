package game

import (
	"errors"
	"fmt"
	"io"

	"github.com/aleksandergregersen/opsquest/internal/sandbox"
)

const maxViFileBytes = 256 * 1024

var (
	ErrInteractiveEditor     = errors.New("interactive editor requires a terminal")
	ErrUnsupportedEditorFile = errors.New("unsupported editor file")
)

type viSaveFunc func(path, content string) error

type viSizeFunc func() (width, height int)

func runViEditor(input *terminalKeyReader, output io.Writer, request sandbox.EditorRequest, save viSaveFunc, size viSizeFunc) error {
	editor, err := newViEditor(request)
	if err != nil {
		return err
	}
	pasting := false
	acceptPaste := false
	for {
		width, height := size()
		if err := renderViEditor(output, editor, width, height); err != nil {
			return err
		}
		if editor.quit {
			return nil
		}
		var key viKey
		if pasting {
			key, err = readViPasteKey(input)
		} else {
			key, err = readViKey(input)
		}
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("input ended before :q or :wq")
		}
		if err != nil {
			return fmt.Errorf("read vi key: %w", err)
		}
		if key.kind == viPasteStartKey {
			pasting = true
			acceptPaste = editor.mode == viInsertMode
			if !acceptPaste {
				editor.message = "Paste ignored in Normal mode; press i before pasting"
			}
			continue
		}
		if key.kind == viPasteEndKey {
			pasting = false
			acceptPaste = false
			continue
		}
		if pasting && !acceptPaste {
			continue
		}
		editor.handle(key, save)
	}
}
