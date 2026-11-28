package game

import (
	"strings"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/sandbox"
	"github.com/aleksandergregersen/opsquest/internal/terminal"
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
		{name: "vi command", line: "v", position: len("v"), want: "vi "},
		{name: "mission control", line: "obj", position: len("obj"), want: "objective "},
		{name: "navigation control", line: "previ", position: len("previ"), want: "previous "},
		{name: "world navigation", line: "worl", position: len("worl"), want: "world"},
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
			got, _, handled := completeLine(test.line, test.position, newSandboxEnvironment(box))
			if !handled {
				t.Fatal("Tab completion was not handled")
			}
			if got != test.want {
				t.Fatalf("completeLine(%q) = %q, want %q", test.line, got, test.want)
			}
		})
	}
}

func TestCompleteLineCompletesExecutableScriptPath(t *testing.T) {
	box := completionTestSandbox(t)
	if err := box.FS.WriteFile("/home/operator/report.sh", "#!/bin/sh\n", 0o755); err != nil {
		t.Fatal(err)
	}

	line := "./rep"
	got, position, handled := completeLine(line, len(line), newSandboxEnvironment(box))
	if !handled {
		t.Fatal("Tab completion was not handled")
	}
	if got != "./report.sh " || position != len(got) {
		t.Fatalf("script completion = %q at %d, want %q at %d", got, position, "./report.sh ", len("./report.sh "))
	}
}

func TestCompleteLineUsesOnlyTheActiveEnvironmentCandidates(t *testing.T) {
	commands := []string{"docker"}
	source := &seamCompletionSource{
		commands: commands,
		paths: map[string][]CompletionCandidate{
			"rep": {{Value: "reports/", Directory: true}},
		},
	}

	completed, _, handled := completeLine("doc", len("doc"), source)
	if !handled || completed != "docker " {
		t.Fatalf("environment command completion = %q, %v", completed, handled)
	}
	completed, _, handled = completeLine("pw", len("pw"), source)
	if !handled || completed != "pw" {
		t.Fatalf("inactive sandbox command completed = %q, %v", completed, handled)
	}
	completed, _, handled = completeLine("cat rep", len("cat rep"), source)
	if !handled || completed != "cat reports/" {
		t.Fatalf("environment path completion = %q, %v", completed, handled)
	}
	completed, _, handled = completeLine("obj", len("obj"), source)
	if !handled || completed != "objective " {
		t.Fatalf("mission control completion = %q, %v", completed, handled)
	}
	if len(commands) != 1 || commands[0] != "docker" {
		t.Fatalf("completion mutated environment commands: %#v", commands)
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
		completed, _, _ := completeLine(test.line, len(test.line), newSandboxEnvironment(box))
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

func completeLine(line string, position int, source CompletionSource) (string, int, bool) {
	return terminal.Completer(source, missionControls)(line, position, '\t')
}
