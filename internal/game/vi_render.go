package game

import (
	"fmt"
	"io"
	"strings"
	"unicode"
)

func renderViEditor(output io.Writer, editor *viEditor, width, height int) error {
	width = max(width, 20)
	height = max(height, 6)
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
		cursorColumn = min(cursorColumn, width)
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
	column = min(column, len(line))
	columns := 0
	for _, char := range line[:column] {
		columns += displayViRuneWidth(char, columns)
	}
	return columns
}

func displayViWindow(line []rune, offset, width int) string {
	offset = max(offset, 0)
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
	runes = runes[:min(len(runes), width)]
	return string(runes) + strings.Repeat(" ", width-len(runes))
}
