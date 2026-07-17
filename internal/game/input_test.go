package game

import (
	"bytes"
	"strings"
	"testing"

	"golang.org/x/term"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/sandbox"
)

func completionTestSandbox(t *testing.T) *sandbox.Sandbox {
	t.Helper()
	box, err := sandbox.New(mission.Setup{
		Directories: []mission.DirectorySpec{
			{Path: "/home/operator"},
			{Path: "/home/operator/workspace"},
			{Path: "/srv/web/config"},
		},
		Files: []mission.FileSpec{
			{Path: "/home/operator/WELCOME.txt", Content: "Welcome\n"},
			{Path: "/home/operator/cash$money.txt", Content: "Cash\n"},
			{Path: "/home/operator/incident report.txt", Content: "Incident\n"},
			{Path: "/home/operator/it's.txt", Content: "Apostrophe\n"},
			{Path: `/home/operator/say"hi.txt`, Content: "Quote\n"},
			{Path: "/home/operator/worker.log", Content: "worker\n"},
			{Path: "/home/operator/work.txt", Content: "work\n"},
		},
	}, "/home/operator")
	if err != nil {
		t.Fatal(err)
	}
	return box
}

func TestCompleteLineUsesCommandsControlsAndVirtualPaths(t *testing.T) {
	box := completionTestSandbox(t)
	tests := []struct {
		name     string
		line     string
		position int
		want     string
	}{
		{name: "command", line: "pw", position: len("pw"), want: "pwd "},
		{name: "mission control", line: "obj", position: len("obj"), want: "objective "},
		{name: "relative file", line: "cat W", position: len("cat W"), want: "cat WELCOME.txt "},
		{name: "quoted file", line: `cat "W`, position: len(`cat "W`), want: `cat "WELCOME.txt" `},
		{name: "absolute directory", line: "cd /srv/w", position: len("cd /srv/w"), want: "cd /srv/web/"},
		{name: "space escaped", line: "cat inc", position: len("cat inc"), want: `cat incident\ report.txt `},
		{name: "double quoted variable marker", line: `cat "cash`, position: len(`cat "cash`), want: `cat "cash\$money.txt" `},
		{name: "double quoted quote", line: `cat "say`, position: len(`cat "say`), want: `cat "say\"hi.txt" `},
		{name: "single quoted apostrophe", line: "cat 'it", position: len("cat 'it"), want: `cat 'it'\''s.txt' `},
		{name: "pipeline command", line: "cat WELCOME.txt | gr", position: len("cat WELCOME.txt | gr"), want: "cat WELCOME.txt | grep "},
		{name: "replace token around cursor", line: "cat Wzz tail", position: len("cat W"), want: "cat WELCOME.txt tail"},
		{name: "multiple paths extend common prefix", line: "cat w", position: len("cat w"), want: "cat work"},
		{name: "host path unavailable", line: "cat /etc/p", position: len("cat /etc/p"), want: "cat /etc/p"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _, handled := completeLine(test.line, test.position, box)
			if !handled {
				t.Fatal("Tab completion was not handled")
			}
			if got != test.want {
				t.Fatalf("completeLine(%q) = %q, want %q", test.line, got, test.want)
			}
		})
	}
}

func TestTerminalEditorCompletesAndRecallsHistory(t *testing.T) {
	box := completionTestSandbox(t)
	input := strings.NewReader("cat W\t\r\x1b[A\r")
	output := &bytes.Buffer{}
	editor := term.NewTerminal(terminalReadWriter{reader: input, writer: output}, "opsquest$ ")
	editor.AutoCompleteCallback = terminalCompleter(box)

	first, err := editor.ReadLine()
	if err != nil {
		t.Fatalf("first ReadLine() error = %v", err)
	}
	if first != "cat WELCOME.txt " {
		t.Fatalf("first line = %q", first)
	}

	second, err := editor.ReadLine()
	if err != nil {
		t.Fatalf("history ReadLine() error = %v", err)
	}
	if second != first {
		t.Fatalf("up-arrow history = %q, want %q", second, first)
	}
}

func TestCompletedQuotedPathsRemainValidSandboxCommands(t *testing.T) {
	box := completionTestSandbox(t)
	tests := []struct {
		line       string
		wantOutput string
	}{
		{line: `cat "cash`, wantOutput: "Cash\n"},
		{line: `cat "say`, wantOutput: "Quote\n"},
		{line: "cat 'it", wantOutput: "Apostrophe\n"},
		{line: "cat inc", wantOutput: "Incident\n"},
	}

	for _, test := range tests {
		completed, _, _ := completeLine(test.line, len(test.line), box)
		result, err := box.Execute(strings.TrimSpace(completed))
		if err != nil {
			t.Errorf("execute completion for %q: %v", test.line, err)
			continue
		}
		if result.Output != test.wantOutput {
			t.Errorf("completion for %q output = %q, want %q", test.line, result.Output, test.wantOutput)
		}
	}
}

func TestNonTerminalReaderKeepsScriptedInputPath(t *testing.T) {
	output := &bytes.Buffer{}
	reader := NewCommandLineReader(strings.NewReader("pwd\n"), output)
	if _, ok := reader.(*scannerLineReader); !ok {
		t.Fatalf("reader type = %T, want scanner fallback", reader)
	}

	line, err := reader.ReadLine("opsquest:/work$ ", nil)
	if err != nil {
		t.Fatal(err)
	}
	if line != "pwd" || output.String() != "opsquest:/work$ " {
		t.Fatalf("line = %q, output = %q", line, output.String())
	}
}

func TestTerminalCompleterLeavesOtherKeysToTheEditor(t *testing.T) {
	line, position, handled := terminalCompleter(completionTestSandbox(t))("pw", 2, 'd')
	if handled || line != "pw" || position != 2 {
		t.Fatalf("non-Tab completion = %q, %d, %v", line, position, handled)
	}
}
