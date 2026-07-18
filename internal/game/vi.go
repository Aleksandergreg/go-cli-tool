package game

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/aleksandergregersen/opsquest/internal/sandbox"
)

const maxViFileBytes = 256 * 1024

var (
	ErrInteractiveEditor     = errors.New("interactive editor requires a terminal")
	ErrUnsupportedEditorFile = errors.New("unsupported editor file")
)

type viSaveFunc func(path, content string) error

type viMode uint8

const (
	viNormalMode viMode = iota
	viInsertMode
	viCommandMode
)

type viKeyKind uint8

const (
	viRuneKey viKeyKind = iota
	viEscapeKey
	viEnterKey
	viBackspaceKey
	viDeleteKey
	viUpKey
	viDownKey
	viLeftKey
	viRightKey
	viHomeKey
	viEndKey
	viInterruptKey
	viPasteStartKey
	viPasteEndKey
	viUnknownKey
)

type viKey struct {
	kind viKeyKind
	rune rune
	raw  string
}

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
		insertBackspace(e)
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

func insertBackspace(e *viEditor) {
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
	line := e.lines[e.row]
	if e.column >= len(line) {
		return
	}
	removed := line[e.column]
	e.lines[e.row] = append(line[:e.column], line[e.column+1:]...)
	e.byteSize -= utf8.RuneLen(removed)
	e.dirty = true
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
	limit := e.normalLineEnd()
	e.column += delta
	if e.column < 0 {
		e.column = 0
	}
	if e.column > limit {
		e.column = limit
	}
	e.preferredCol = e.column
}

func (e *viEditor) moveVertical(delta int) {
	row := e.row + delta
	if row < 0 {
		row = 0
	}
	if row >= len(e.lines) {
		row = len(e.lines) - 1
	}
	e.row = row
	e.column = e.preferredCol
	e.clampNormalColumn()
}

func (e *viEditor) moveInsertVertical(delta int) {
	row := e.row + delta
	if row < 0 {
		row = 0
	}
	if row >= len(e.lines) {
		row = len(e.lines) - 1
	}
	e.row = row
	e.column = e.preferredCol
	if e.column > len(e.lines[e.row]) {
		e.column = len(e.lines[e.row])
	}
}

func (e *viEditor) normalLineEnd() int {
	if len(e.lines[e.row]) == 0 {
		return 0
	}
	return len(e.lines[e.row]) - 1
}

func (e *viEditor) clampNormalColumn() {
	if e.column < 0 {
		e.column = 0
	}
	if limit := e.normalLineEnd(); e.column > limit {
		e.column = limit
	}
}

func (e *viEditor) deleteAtCursor() {
	line := e.lines[e.row]
	if len(line) == 0 || e.column >= len(line) {
		return
	}
	removed := line[e.column]
	e.lines[e.row] = append(line[:e.column], line[e.column+1:]...)
	e.byteSize -= utf8.RuneLen(removed)
	e.clampNormalColumn()
	e.preferredCol = e.column
	e.dirty = true
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

func readViPasteKey(input *terminalKeyReader) (viKey, error) {
	first, err := readViByte(input)
	if err != nil {
		return viKey{}, err
	}
	// terminalKeyReader toggles pasteActive only after reading the complete end
	// marker, which lets this literal reader distinguish it from an Escape byte
	// contained in pasted text.
	if first == '\x1b' && !input.pasteActive {
		marker := make([]byte, len(bracketedPasteEnd))
		marker[0] = first
		for index := 1; index < len(marker); index++ {
			marker[index], err = readViByte(input)
			if err != nil {
				return viKey{}, err
			}
		}
		if string(marker) == bracketedPasteEnd {
			return viKey{kind: viPasteEndKey}, nil
		}
		return viKey{kind: viUnknownKey, raw: string(marker)}, nil
	}
	if first == '\r' || first == '\n' {
		return viKey{kind: viEnterKey}, nil
	}
	if first < utf8.RuneSelf {
		return viKey{kind: viRuneKey, rune: rune(first)}, nil
	}
	return readViUTF8Key(input, first)
}

func readViKey(input *terminalKeyReader) (viKey, error) {
	first, err := readViByte(input)
	if err != nil {
		return viKey{}, err
	}
	switch first {
	case '\x1b':
		if input.buffered() == 0 {
			return viKey{kind: viEscapeKey}, nil
		}
		second, err := readViByte(input)
		if err != nil {
			return viKey{kind: viEscapeKey}, nil
		}
		if second != '[' && second != 'O' {
			input.unread([]byte{second})
			return viKey{kind: viEscapeKey}, nil
		}
		sequence := []byte{'\x1b', second}
		for len(sequence) < 32 {
			next, err := readViByte(input)
			if err != nil {
				return viKey{kind: viUnknownKey, raw: string(sequence)}, nil
			}
			sequence = append(sequence, next)
			if next >= 0x40 && next <= 0x7e {
				break
			}
		}
		return decodeViSequence(string(sequence)), nil
	case '\r', '\n':
		return viKey{kind: viEnterKey}, nil
	case '\x7f', '\b':
		return viKey{kind: viBackspaceKey}, nil
	case '\x03':
		return viKey{kind: viInterruptKey}, nil
	}

	if first < utf8.RuneSelf {
		return viKey{kind: viRuneKey, rune: rune(first)}, nil
	}
	return readViUTF8Key(input, first)
}

func readViUTF8Key(input *terminalKeyReader, first byte) (viKey, error) {
	width := utf8SequenceLength(first)
	if width == 0 {
		return viKey{kind: viRuneKey, rune: utf8.RuneError}, nil
	}
	encoded := make([]byte, width)
	encoded[0] = first
	for index := 1; index < width; index++ {
		next, err := readViByte(input)
		if err != nil {
			return viKey{}, err
		}
		encoded[index] = next
	}
	decoded, decodedWidth := utf8.DecodeRune(encoded)
	if decoded == utf8.RuneError && decodedWidth == 1 {
		return viKey{kind: viRuneKey, rune: utf8.RuneError}, nil
	}
	if decoded == terminalKeyDeleteForward {
		return viKey{kind: viDeleteKey}, nil
	}
	return viKey{kind: viRuneKey, rune: decoded}, nil
}

func readViByte(input io.Reader) (byte, error) {
	var value [1]byte
	_, err := io.ReadFull(input, value[:])
	return value[0], err
}

func utf8SequenceLength(first byte) int {
	switch {
	case first&0xe0 == 0xc0:
		return 2
	case first&0xf0 == 0xe0:
		return 3
	case first&0xf8 == 0xf0:
		return 4
	default:
		return 0
	}
}

func decodeViSequence(sequence string) viKey {
	switch sequence {
	case bracketedPasteStart:
		return viKey{kind: viPasteStartKey}
	case bracketedPasteEnd:
		return viKey{kind: viPasteEndKey}
	case "\x1b[A", "\x1bOA":
		return viKey{kind: viUpKey}
	case "\x1b[B", "\x1bOB":
		return viKey{kind: viDownKey}
	case "\x1b[C", "\x1bOC":
		return viKey{kind: viRightKey}
	case "\x1b[D", "\x1bOD":
		return viKey{kind: viLeftKey}
	case "\x1b[H", "\x1bOH":
		return viKey{kind: viHomeKey}
	case "\x1b[F", "\x1bOF":
		return viKey{kind: viEndKey}
	case "\x1b[3~":
		return viKey{kind: viDeleteKey}
	default:
		return viKey{kind: viUnknownKey, raw: sequence}
	}
}

func renderViEditor(output io.Writer, editor *viEditor, width, height int) error {
	if width < 20 {
		width = 20
	}
	if height < 6 {
		height = 6
	}
	bodyRows := height - 2
	top := 0
	if editor.row >= bodyRows {
		top = editor.row - bodyRows + 1
	}
	cursorDisplayColumn := displayViColumn(editor.lines[editor.row], editor.column)
	horizontalOffset := 0
	if cursorDisplayColumn >= width {
		horizontalOffset = cursorDisplayColumn - width + 1
	}

	var screen strings.Builder
	screen.WriteString("\x1b[?25l\x1b[H")
	for screenRow := 0; screenRow < bodyRows; screenRow++ {
		lineIndex := top + screenRow
		line := "~"
		if lineIndex < len(editor.lines) {
			line = displayViWindow(editor.lines[lineIndex], horizontalOffset, width)
		}
		screen.WriteString(padViLine(line, width))
		screen.WriteString("\r\n")
	}
	status := editor.statusLine()
	screen.WriteString("\x1b[7m")
	screen.WriteString(padViLine(status, width))
	screen.WriteString("\x1b[0m\r\n")
	cheatSheet := "h/j/k/l or arrows move · i insert · Esc normal · x delete · dd line · :w :q :wq"
	screen.WriteString(padViLine(cheatSheet, width))

	cursorRow := editor.row - top + 1
	cursorColumn := cursorDisplayColumn - horizontalOffset + 1
	if editor.mode == viCommandMode {
		cursorRow = bodyRows + 1
		cursorColumn = len([]rune(":"+string(editor.command))) + 1
		if cursorColumn > width {
			cursorColumn = width
		}
	}
	fmt.Fprintf(&screen, "\x1b[%d;%dH\x1b[?25h", cursorRow, cursorColumn)
	_, err := io.WriteString(output, screen.String())
	return err
}

func (e *viEditor) statusLine() string {
	if e.mode == viCommandMode {
		return ":" + string(e.command)
	}
	if e.message != "" {
		return sanitizeViText(e.message)
	}
	modified := ""
	if e.dirty {
		modified = " [+]"
	}
	mode := "NORMAL"
	if e.mode == viInsertMode {
		mode = "-- INSERT --"
	}
	return fmt.Sprintf("%s%s  %s  %d,%d", sanitizeViText(e.request.DisplayPath), modified, mode, e.row+1, e.column+1)
}

func displayViColumn(line []rune, column int) int {
	if column > len(line) {
		column = len(line)
	}
	columns := 0
	for _, char := range line[:column] {
		columns += displayViRuneWidth(char, columns)
	}
	return columns
}

func displayViWindow(line []rune, offset, width int) string {
	if offset < 0 {
		offset = 0
	}
	if width <= 0 {
		return ""
	}
	var displayed strings.Builder
	displayed.Grow(width)
	columns := 0
	written := 0
	for _, char := range line {
		segmentWidth := displayViRuneWidth(char, columns)
		if columns+segmentWidth <= offset {
			columns += segmentWidth
			continue
		}
		if written >= width {
			break
		}
		start := 0
		if offset > columns {
			start = offset - columns
		}
		available := segmentWidth - start
		if available > width-written {
			available = width - written
		}
		if char != '\t' && !unicode.IsControl(char) {
			if start == 0 && available == 1 {
				displayed.WriteRune(char)
			}
		} else {
			text := displayViRune(char, columns)
			segment := []rune(text)
			displayed.WriteString(string(segment[start : start+available]))
		}
		written += available
		columns += segmentWidth
	}
	return displayed.String()
}

func displayViRuneWidth(char rune, column int) int {
	if char == '\t' {
		return 8 - column%8
	}
	if unicode.IsControl(char) && char < 0x20 {
		return 2
	}
	return 1
}

func displayViRune(char rune, column int) string {
	if char == '\t' {
		spaces := 8 - column%8
		return strings.Repeat(" ", spaces)
	}
	if unicode.IsControl(char) {
		if char < 0x20 {
			return "^" + string(char+'@')
		}
		return "?"
	}
	return string(char)
}

func sanitizeViText(value string) string {
	var sanitized strings.Builder
	for _, char := range value {
		if unicode.IsControl(char) {
			sanitized.WriteString(displayViRune(char, 0))
			continue
		}
		sanitized.WriteRune(char)
	}
	return sanitized.String()
}

func padViLine(value string, width int) string {
	runes := []rune(value)
	if len(runes) > width {
		runes = runes[:width]
	}
	return string(runes) + strings.Repeat(" ", width-len(runes))
}
