package ui

import (
	"bytes"
	"testing"
)

type fakeTerminalWriter struct {
	bytes.Buffer
	fd uintptr
}

func (w *fakeTerminalWriter) Fd() uintptr {
	return w.fd
}

func TestZeroValueStyleIsPlainText(t *testing.T) {
	var style Style
	if style.Enabled() {
		t.Fatal("zero-value Style is enabled")
	}

	tests := map[string]struct {
		got  string
		want string
	}{
		"header":        {got: style.Header("MISSION 01"), want: "MISSION 01"},
		"accent":        {got: style.Accent("next"), want: "next"},
		"success":       {got: style.Success("complete"), want: "complete"},
		"warning":       {got: style.Warning("incomplete"), want: "incomplete"},
		"failure":       {got: style.Failure("failed"), want: "failed"},
		"reward":        {got: style.Reward("+40 XP"), want: "+40 XP"},
		"achievement":   {got: style.Achievement("Pipe Dream"), want: "Pipe Dream"},
		"muted":         {got: style.Muted("locked"), want: "locked"},
		"difficulty":    {got: style.Difficulty("beginner"), want: "beginner"},
		"prompt":        {got: style.Prompt("/workspace"), want: "opsquest:/workspace$ "},
		"progress":      {got: style.Progress("██", "░░"), want: "██░░"},
		"empty colored": {got: style.Header(""), want: ""},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("got %q, want %q", test.got, test.want)
			}
		})
	}
}

func TestEnabledStyleUsesSemanticColors(t *testing.T) {
	style := New(true)
	if !style.Enabled() {
		t.Fatal("New(true) Style is disabled")
	}

	tests := map[string]struct {
		got  string
		want string
	}{
		"header":      {got: style.Header("mission"), want: "\x1b[1;36mmission\x1b[0m"},
		"accent":      {got: style.Accent("next"), want: "\x1b[36mnext\x1b[0m"},
		"success":     {got: style.Success("done"), want: "\x1b[32mdone\x1b[0m"},
		"warning":     {got: style.Warning("hint"), want: "\x1b[33mhint\x1b[0m"},
		"failure":     {got: style.Failure("error"), want: "\x1b[31merror\x1b[0m"},
		"reward":      {got: style.Reward("+40 XP"), want: "\x1b[1;33m+40 XP\x1b[0m"},
		"achievement": {got: style.Achievement("star"), want: "\x1b[1;35mstar\x1b[0m"},
		"muted":       {got: style.Muted("locked"), want: "\x1b[2mlocked\x1b[0m"},
		"prompt":      {got: style.Prompt("/tmp"), want: "\x1b[36mopsquest:/tmp$ \x1b[0m"},
		"progress":    {got: style.Progress("██", "░░"), want: "\x1b[32m██\x1b[0m\x1b[2m░░\x1b[0m"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("got %q, want %q", test.got, test.want)
			}
		})
	}

	if got := style.Header(""); got != "" {
		t.Fatalf("empty styled text = %q, want empty", got)
	}
}

func TestDifficultyUsesKnownSemanticColors(t *testing.T) {
	style := New(true)
	tests := []struct {
		value string
		want  string
	}{
		{value: "beginner", want: "\x1b[32mbeginner\x1b[0m"},
		{value: "Intermediate", want: "\x1b[33mIntermediate\x1b[0m"},
		{value: " advanced ", want: "\x1b[31m advanced \x1b[0m"},
		{value: "expert", want: "expert"},
		{value: "", want: ""},
	}
	for _, test := range tests {
		if got := style.Difficulty(test.value); got != test.want {
			t.Errorf("Difficulty(%q) = %q, want %q", test.value, got, test.want)
		}
	}
}

func TestAutoEnabledPolicy(t *testing.T) {
	tests := []struct {
		name                string
		out                 interface{ Write([]byte) (int, error) }
		environment         map[string]string
		terminal            bool
		want                bool
		wantTerminalChecked bool
	}{
		{name: "terminal", out: &fakeTerminalWriter{fd: 9}, terminal: true, want: true, wantTerminalChecked: true},
		{name: "non-terminal", out: &fakeTerminalWriter{fd: 9}, terminal: false, want: false, wantTerminalChecked: true},
		{name: "writer without descriptor", out: &bytes.Buffer{}, terminal: true, want: false},
		{name: "NO_COLOR present", out: &fakeTerminalWriter{fd: 9}, environment: map[string]string{"NO_COLOR": ""}, terminal: true, want: false},
		{name: "TERM dumb", out: &fakeTerminalWriter{fd: 9}, environment: map[string]string{"TERM": " dumb "}, terminal: true, want: false},
		{name: "ordinary TERM", out: &fakeTerminalWriter{fd: 9}, environment: map[string]string{"TERM": "xterm-256color"}, terminal: true, want: true, wantTerminalChecked: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lookup := func(key string) (string, bool) {
				value, exists := test.environment[key]
				return value, exists
			}
			checked := false
			isTerminal := func(fd int) bool {
				checked = true
				if fd != 9 {
					t.Fatalf("terminal check fd = %d, want 9", fd)
				}
				return test.terminal
			}
			if got := autoEnabled(test.out, lookup, isTerminal); got != test.want {
				t.Fatalf("autoEnabled() = %v, want %v", got, test.want)
			}
			if checked != test.wantTerminalChecked {
				t.Fatalf("terminal check called = %v, want %v", checked, test.wantTerminalChecked)
			}
		})
	}
}

func TestAutoDisablesNonTerminalWriter(t *testing.T) {
	if style := Auto(&bytes.Buffer{}); style.Enabled() {
		t.Fatal("Auto enabled color for a writer without a file descriptor")
	}
}
