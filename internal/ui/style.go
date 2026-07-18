// Package ui provides the small, semantic ANSI palette used by OpsQuest
// presentation code. Its zero value is deliberately plain text so tests,
// redirected output, and callers that do not opt into styling remain stable.
package ui

import (
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

const (
	ansiReset       = "\x1b[0m"
	ansiBold        = "\x1b[1m"
	ansiBoldCyan    = "\x1b[1;36m"
	ansiCyan        = "\x1b[36m"
	ansiGreen       = "\x1b[32m"
	ansiYellow      = "\x1b[33m"
	ansiRed         = "\x1b[31m"
	ansiMagenta     = "\x1b[35m"
	ansiBoldYellow  = "\x1b[1;33m"
	ansiBoldMagenta = "\x1b[1;35m"
	ansiDim         = "\x1b[2m"
)

// Style applies semantic terminal colors when enabled. The zero value is
// disabled and returns every string unchanged.
type Style struct {
	enabled bool
}

// New returns a Style with the requested color behavior.
func New(enabled bool) Style {
	return Style{enabled: enabled}
}

// Auto enables colors only when out is a terminal that has not opted out via
// NO_COLOR or TERM=dumb.
func Auto(out io.Writer) Style {
	return New(autoEnabled(out, os.LookupEnv, term.IsTerminal))
}

// Enabled reports whether the Style emits ANSI escape sequences.
func (s Style) Enabled() bool {
	return s.enabled
}

// Header styles prominent OpsQuest headings.
func (s Style) Header(text string) string {
	return s.paint(ansiBoldCyan, text)
}

// Section styles a heading within one screen without competing with the page
// header's color. It is useful for labels such as Objective and Progress.
func (s Style) Section(text string) string {
	return s.paint(ansiBold, text)
}

// World styles campaign and narrative-world labels. Regular magenta keeps the
// role distinct from bold-magenta achievement announcements.
func (s Style) World(text string) string {
	return s.paint(ansiMagenta, text)
}

// Accent styles navigation and other secondary highlights.
func (s Style) Accent(text string) string {
	return s.paint(ansiCyan, text)
}

// Success styles completed or healthy state.
func (s Style) Success(text string) string {
	return s.paint(ansiGreen, text)
}

// Warning styles hints, incomplete state, and other cautions.
func (s Style) Warning(text string) string {
	return s.paint(ansiYellow, text)
}

// Failure styles errors and failed state.
func (s Style) Failure(text string) string {
	return s.paint(ansiRed, text)
}

// Reward styles XP and other earned rewards.
func (s Style) Reward(text string) string {
	return s.paint(ansiBoldYellow, text)
}

// Achievement styles achievement announcements and unlocked markers.
func (s Style) Achievement(text string) string {
	return s.paint(ansiBoldMagenta, text)
}

// Muted styles supporting instructions and unavailable state.
func (s Style) Muted(text string) string {
	return s.paint(ansiDim, text)
}

// Difficulty applies the teaching difficulty's semantic color. Unknown values
// remain plain so a new schema value is not assigned a misleading meaning.
func (s Style) Difficulty(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "beginner":
		return s.Success(value)
	case "intermediate":
		return s.Warning(value)
	case "advanced":
		// Advanced describes curriculum depth, not a failed or dangerous state.
		// Keep red reserved for actual errors and failures.
		return s.paint(ansiMagenta, value)
	default:
		return value
	}
}

// Prompt returns the complete mission-shell prompt, accented when enabled.
func (s Style) Prompt(cwd string) string {
	return s.Accent("opsquest:" + cwd + "$ ")
}

// Progress combines a completed segment with a muted remaining segment.
func (s Style) Progress(filled, empty string) string {
	return s.Success(filled) + s.Muted(empty)
}

// CommandGuide formats the free, non-prescriptive tool orientation shared by
// mission introductions, previews, and objective recall.
func (s Style) CommandGuide(commands []string) string {
	return s.Section("Commands you may need to solve this level:") + "\n  " + s.Accent(strings.Join(commands, ", "))
}

func (s Style) paint(code, text string) string {
	if !s.enabled || text == "" {
		return text
	}
	return code + text + ansiReset
}

type environmentLookup func(string) (string, bool)
type terminalCheck func(int) bool

type fileDescriptorWriter interface {
	Fd() uintptr
}

func autoEnabled(out io.Writer, lookup environmentLookup, isTerminal terminalCheck) bool {
	if _, exists := lookup("NO_COLOR"); exists {
		return false
	}
	if value, exists := lookup("TERM"); exists && strings.EqualFold(strings.TrimSpace(value), "dumb") {
		return false
	}
	terminal, ok := out.(fileDescriptorWriter)
	if !ok {
		return false
	}
	return isTerminal(int(terminal.Fd()))
}
