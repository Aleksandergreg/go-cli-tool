package game

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/aleksandergregersen/opsquest/internal/sandbox"
)

type viMode uint8

const (
	viNormalMode viMode = iota
	viInsertMode
	viCommandMode
)

type viEditor struct {
	request      sandbox.EditorRequest
	lines        [][]rune
	row          int
	column       int
	preferredCol int
	mode         viMode
	command      []rune
	pendingD     bool
	dirty        bool
	quit         bool
	message      string
	byteSize     int
}

func newViEditor(request sandbox.EditorRequest) (*viEditor, error) {
	if len(request.Content) > maxViFileBytes {
		return nil, fmt.Errorf("%w: %s exceeds the %d KiB teaching-editor limit", ErrUnsupportedEditorFile, request.DisplayPath, maxViFileBytes/1024)
	}
	if !utf8.ValidString(request.Content) {
		return nil, fmt.Errorf("%w: %s is not valid UTF-8 text", ErrUnsupportedEditorFile, request.DisplayPath)
	}
	parts := strings.Split(request.Content, "\n")
	lines := make([][]rune, len(parts))
	for index, line := range parts {
		lines[index] = []rune(line)
	}
	return &viEditor{
		request:  request,
		lines:    lines,
		message:  "NORMAL — i insert · h/j/k/l move · x delete · dd delete line · : commands",
		byteSize: len(request.Content),
	}, nil
}

func (e *viEditor) content() string {
	parts := make([]string, len(e.lines))
	for index, line := range e.lines {
		parts[index] = string(line)
	}
	return strings.Join(parts, "\n")
}

func (e *viEditor) handle(key viKey, save viSaveFunc) {
	switch e.mode {
	case viInsertMode:
		e.handleInsert(key)
	case viCommandMode:
		e.handleCommand(key, save)
	default:
		e.handleNormal(key)
	}
}

func (e *viEditor) handleNormal(key viKey) {
	e.message = ""
	if key.kind == viInterruptKey || key.kind == viEscapeKey {
		e.pendingD = false
		return
	}

	if key.kind != viRuneKey || key.rune != 'd' {
		e.pendingD = false
	}
	switch key.kind {
	case viLeftKey:
		e.moveHorizontal(-1)
		return
	case viRightKey:
		e.moveHorizontal(1)
		return
	case viUpKey:
		e.moveVertical(-1)
		return
	case viDownKey:
		e.moveVertical(1)
		return
	case viHomeKey:
		e.column = 0
		e.preferredCol = 0
		return
	case viEndKey:
		e.column = e.normalLineEnd()
		e.preferredCol = e.column
		return
	case viUnknownKey:
		e.message = fmt.Sprintf("Unsupported key sequence %s", strconv.QuoteToASCII(key.raw))
		return
	case viRuneKey:
	default:
		return
	}

	switch key.rune {
	case 'h':
		e.moveHorizontal(-1)
	case 'j':
		e.moveVertical(1)
	case 'k':
		e.moveVertical(-1)
	case 'l':
		e.moveHorizontal(1)
	case 'i':
		e.mode = viInsertMode
		e.pendingD = false
	case 'x':
		e.deleteAtCursor()
	case 'd':
		if e.pendingD {
			e.deleteLine()
			e.pendingD = false
		} else {
			e.pendingD = true
			e.message = "d"
		}
	case ':':
		e.mode = viCommandMode
		e.command = nil
		e.pendingD = false
	default:
		if unicode.IsPrint(key.rune) {
			e.message = fmt.Sprintf("%q is outside this vi teaching subset", key.rune)
		}
	}
}

func (e *viEditor) handleInsert(key viKey) {
	switch key.kind {
	case viEscapeKey, viInterruptKey:
		e.mode = viNormalMode
		if e.column > 0 {
			e.column--
		}
		e.clampNormalColumn()
		e.preferredCol = e.column
		e.message = ""
	case viLeftKey:
		if e.column > 0 {
			e.column--
		}
		e.preferredCol = e.column
	case viRightKey:
		if e.column < len(e.lines[e.row]) {
			e.column++
		}
		e.preferredCol = e.column
	case viUpKey:
		e.moveInsertVertical(-1)
	case viDownKey:
		e.moveInsertVertical(1)
	case viHomeKey:
		e.column = 0
		e.preferredCol = 0
	case viEndKey:
		e.column = len(e.lines[e.row])
		e.preferredCol = e.column
	case viBackspaceKey:
		e.insertBackspace()
	case viDeleteKey:
		e.insertDelete()
	case viEnterKey:
		if !e.canGrow(1) {
			return
		}
		line := e.lines[e.row]
		left := append([]rune(nil), line[:e.column]...)
		right := append([]rune(nil), line[e.column:]...)
		e.lines[e.row] = left
		e.lines = append(e.lines, nil)
		copy(e.lines[e.row+2:], e.lines[e.row+1:])
		e.lines[e.row+1] = right
		e.row++
		e.column = 0
		e.preferredCol = 0
		e.byteSize++
		e.dirty = true
	case viRuneKey:
		if key.rune < ' ' && key.rune != '\t' {
			e.message = "That control character is not supported in insert mode"
			return
		}
		growth := utf8.RuneLen(key.rune)
		if growth < 0 || !e.canGrow(growth) {
			return
		}
		line := e.lines[e.row]
		line = append(line, 0)
		copy(line[e.column+1:], line[e.column:])
		line[e.column] = key.rune
		e.lines[e.row] = line
		e.column++
		e.preferredCol = e.column
		e.byteSize += growth
		e.dirty = true
	case viUnknownKey:
		e.message = fmt.Sprintf("Unsupported key sequence %s", strconv.QuoteToASCII(key.raw))
	}
}

func (e *viEditor) insertBackspace() {
	if e.column > 0 {
		line := e.lines[e.row]
		removed := line[e.column-1]
		e.lines[e.row] = append(line[:e.column-1], line[e.column:]...)
		e.column--
		e.preferredCol = e.column
		e.byteSize -= utf8.RuneLen(removed)
		e.dirty = true
		return
	}
	if e.row == 0 {
		return
	}
	previousLength := len(e.lines[e.row-1])
	e.lines[e.row-1] = append(e.lines[e.row-1], e.lines[e.row]...)
	e.lines = append(e.lines[:e.row], e.lines[e.row+1:]...)
	e.row--
	e.column = previousLength
	e.preferredCol = e.column
	e.byteSize--
	e.dirty = true
}

func (e *viEditor) insertDelete() {
	e.removeRuneAtCursor()
}

func (e *viEditor) handleCommand(key viKey, save viSaveFunc) {
	switch key.kind {
	case viEscapeKey, viInterruptKey:
		e.mode = viNormalMode
		e.command = nil
		e.message = ""
	case viBackspaceKey:
		if len(e.command) == 0 {
			e.mode = viNormalMode
			return
		}
		e.command = e.command[:len(e.command)-1]
	case viEnterKey:
		e.executeCommand(save)
	case viRuneKey:
		if unicode.IsPrint(key.rune) {
			e.command = append(e.command, key.rune)
		}
	}
}

func (e *viEditor) executeCommand(save viSaveFunc) {
	command := string(e.command)
	e.command = nil
	e.mode = viNormalMode
	switch command {
	case "w":
		e.write(save)
	case "q":
		if e.dirty {
			e.message = "E37: No write since last change (use :q! to discard)"
			return
		}
		e.quit = true
	case "wq":
		if e.write(save) {
			e.quit = true
		}
	case "q!":
		e.quit = true
	case "":
		e.message = ""
	default:
		e.message = fmt.Sprintf("E492: Not an editor command: %s", command)
	}
}

func (e *viEditor) write(save viSaveFunc) bool {
	content := e.content()
	if err := save(e.request.Path, content); err != nil {
		e.message = fmt.Sprintf("E212: cannot write %s: %v", e.request.DisplayPath, err)
		return false
	}
	e.dirty = false
	e.byteSize = len(content)
	lineCount := len(e.lines)
	if lineCount > 1 && strings.HasSuffix(content, "\n") {
		lineCount--
	}
	e.message = fmt.Sprintf("%q %dL, %dB written", e.request.DisplayPath, lineCount, e.byteSize)
	return true
}

func (e *viEditor) moveHorizontal(delta int) {
	e.column = min(max(e.column+delta, 0), e.normalLineEnd())
	e.preferredCol = e.column
}

func (e *viEditor) moveVertical(delta int) {
	e.moveRow(delta)
	e.clampNormalColumn()
}

func (e *viEditor) moveInsertVertical(delta int) {
	e.moveRow(delta)
	e.column = min(e.column, len(e.lines[e.row]))
}

func (e *viEditor) moveRow(delta int) {
	e.row = min(max(e.row+delta, 0), len(e.lines)-1)
	e.column = e.preferredCol
}

func (e *viEditor) normalLineEnd() int {
	if len(e.lines[e.row]) == 0 {
		return 0
	}
	return len(e.lines[e.row]) - 1
}

func (e *viEditor) clampNormalColumn() {
	e.column = min(max(e.column, 0), e.normalLineEnd())
}

func (e *viEditor) deleteAtCursor() {
	if !e.removeRuneAtCursor() {
		return
	}
	e.clampNormalColumn()
	e.preferredCol = e.column
}

func (e *viEditor) removeRuneAtCursor() bool {
	line := e.lines[e.row]
	if e.column >= len(line) {
		return false
	}
	removed := line[e.column]
	e.lines[e.row] = append(line[:e.column], line[e.column+1:]...)
	e.byteSize -= utf8.RuneLen(removed)
	e.dirty = true
	return true
}

func (e *viEditor) deleteLine() {
	before := e.byteSize
	if len(e.lines) == 1 {
		e.lines[0] = nil
		e.row = 0
		e.column = 0
	} else {
		e.lines = append(e.lines[:e.row], e.lines[e.row+1:]...)
		if e.row >= len(e.lines) {
			e.row = len(e.lines) - 1
		}
		e.clampNormalColumn()
	}
	e.preferredCol = e.column
	e.byteSize = len(e.content())
	if e.byteSize != before {
		e.dirty = true
	}
}

func (e *viEditor) canGrow(bytes int) bool {
	if e.byteSize+bytes <= maxViFileBytes {
		return true
	}
	e.message = fmt.Sprintf("E340: editor limit is %d KiB", maxViFileBytes/1024)
	return false
}
