package sandbox

import (
	"slices"
	"strings"
	"testing"
)

func out(text string) OutputChunk { return OutputChunk{Text: text} }

func errText(text string) OutputChunk { return OutputChunk{Text: text, Error: true} }

func TestCommandListsFollowShellControlFlow(t *testing.T) {
	missing := "ls: /missing: no such file or directory"
	tests := []struct {
		name         string
		line         string
		wantOutput   string
		wantChunks   []OutputChunk
		wantCommands []string
	}{
		{name: "sequence", line: "echo a; echo b", wantOutput: "a\nb\n", wantChunks: []OutputChunk{out("a\n"), out("b\n")}, wantCommands: []string{"echo", "echo"}},
		{name: "sequence continues after failure", line: "ls /missing; echo after", wantOutput: "after\n", wantChunks: []OutputChunk{errText(missing), out("after\n")}, wantCommands: []string{"echo"}},
		{name: "and runs after success", line: "echo first && echo second", wantOutput: "first\nsecond\n", wantChunks: []OutputChunk{out("first\n"), out("second\n")}, wantCommands: []string{"echo", "echo"}},
		{name: "and skips after failure", line: "ls /missing && echo never", wantChunks: []OutputChunk{errText(missing)}, wantCommands: nil},
		{name: "or runs after failure", line: "ls /missing || echo fallback", wantOutput: "fallback\n", wantChunks: []OutputChunk{errText(missing), out("fallback\n")}, wantCommands: []string{"echo"}},
		{name: "or skips after success", line: "echo ok || echo never", wantOutput: "ok\n", wantChunks: []OutputChunk{out("ok\n")}, wantCommands: []string{"echo"}},
		{name: "equal precedence left to right", line: "false && echo x || echo y", wantOutput: "y\n", wantChunks: []OutputChunk{out("y\n")}, wantCommands: []string{"false", "echo"}},
		{name: "skipped and keeps the earlier status", line: "ls /missing && echo x || echo recovered", wantOutput: "recovered\n", wantChunks: []OutputChunk{errText(missing), out("recovered\n")}, wantCommands: []string{"echo"}},
		{name: "silent false drives or", line: "false || echo fallback", wantOutput: "fallback\n", wantChunks: []OutputChunk{out("fallback\n")}, wantCommands: []string{"false", "echo"}},
		{name: "true drives and", line: "true && echo yes", wantOutput: "yes\n", wantChunks: []OutputChunk{out("yes\n")}, wantCommands: []string{"true", "echo"}},
		{name: "grep match status", line: "grep -q ERROR events.log && echo found", wantOutput: "found\n", wantChunks: []OutputChunk{out("found\n")}, wantCommands: []string{"grep", "echo"}},
		{name: "grep no match status", line: "grep -q PANIC events.log || echo clean", wantOutput: "clean\n", wantChunks: []OutputChunk{out("clean\n")}, wantCommands: []string{"grep", "echo"}},
		{name: "grep count still prints zero", line: "grep -c PANIC events.log || echo none", wantOutput: "0\nnone\n", wantChunks: []OutputChunk{out("0\n"), out("none\n")}, wantCommands: []string{"grep", "echo"}},
		{name: "pipeline status is its last stage", line: "grep PANIC events.log | wc -l && echo counted", wantOutput: "0\ncounted\n", wantChunks: []OutputChunk{out("0\n"), out("counted\n")}, wantCommands: []string{"grep", "wc", "echo"}},
		{name: "trailing semicolon", line: "echo done;", wantOutput: "done\n", wantCommands: []string{"echo"}},
		{name: "trailing comment after semicolon", line: "echo a; echo b; # ; && ||", wantOutput: "a\nb\n", wantChunks: []OutputChunk{out("a\n"), out("b\n")}, wantCommands: []string{"echo", "echo"}},
		{name: "quoted and escaped operators are data", line: `echo 'a; b' "c && d" e\;f g\|\|h`, wantOutput: "a; b c && d e;f g||h\n", wantCommands: []string{"echo"}},
		{name: "operators inside a comment", line: "echo a # ; echo b", wantOutput: "a\n", wantCommands: []string{"echo"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			box := testSandbox(t)
			result, err := box.Execute(test.line)
			if err != nil {
				t.Fatalf("Execute(%q) error = %v", test.line, err)
			}
			if result.Output != test.wantOutput {
				t.Errorf("Output = %q, want %q", result.Output, test.wantOutput)
			}
			if !slices.Equal(result.Transcript, test.wantChunks) {
				t.Errorf("Transcript = %+v, want %+v", result.Transcript, test.wantChunks)
			}
			if !slices.Equal(result.Commands, test.wantCommands) {
				t.Errorf("Commands = %q, want %q", result.Commands, test.wantCommands)
			}
		})
	}
}

func TestCommandListsExpandEachPipelineWhenItRuns(t *testing.T) {
	box := testSandbox(t)
	steps := []struct{ line, want string }{
		{line: `export STAGE=deploy; echo "$STAGE"`, want: "deploy\n"},
		{line: "cd /out && pwd", want: "/out\n"},
		{line: "touch a.txt b.txt; echo *.txt", want: "a.txt b.txt\n"},
		{line: "cd /missing && touch never.txt; pwd", want: "/out\n"},
	}
	for _, step := range steps {
		result, err := box.Execute(step.line)
		if err != nil {
			t.Fatalf("Execute(%q) error = %v", step.line, err)
		}
		if result.Output != step.want {
			t.Errorf("Execute(%q) output = %q, want %q", step.line, result.Output, step.want)
		}
	}
	if box.FS.Exists("/out/never.txt") {
		t.Error("&& ran a command after cd failed")
	}
	if len(box.History) != len(steps) {
		t.Errorf("history = %q, want one entry per line", box.History)
	}
}

func TestSingleCommandsKeepTheirFailureContract(t *testing.T) {
	box := testSandbox(t)
	for _, line := range []string{"grep PANIC events.log", "false", "true"} {
		result, err := box.Execute(line)
		if err != nil || result.Output != "" || result.Transcript != nil {
			t.Errorf("Execute(%q) = %#v, %v; want silent success without a transcript", line, result, err)
		}
	}
	result, err := box.Execute("ls /missing")
	if err == nil || result.Transcript != nil {
		t.Fatalf("single failure = %#v, %v; want a plain error", result, err)
	}
}

func TestCommandListsBoundOutputAndDispatch(t *testing.T) {
	box := testSandbox(t)
	if err := box.FS.WriteFile("/work/big.txt", strings.Repeat("x", maxCommandOutputBytes/2+1), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := box.Execute("cat big.txt; cat big.txt"); err == nil || !strings.Contains(err.Error(), "output") {
		t.Fatalf("combined list output error = %v, want the output limit", err)
	}

	result, err := box.Execute(strings.Repeat("true; ", maxExecutionDispatchSteps+10))
	if err != nil {
		t.Fatalf("long list error = %v", err)
	}
	if len(result.Commands) != maxExecutionDispatchSteps {
		t.Errorf("dispatched commands = %d, want the %d-step budget", len(result.Commands), maxExecutionDispatchSteps)
	}
	last := result.Transcript[len(result.Transcript)-1]
	if !last.Error || !strings.Contains(last.Text, "dispatch limit") {
		t.Errorf("last transcript entry = %#v, want the dispatch limit", last)
	}
}

func TestScriptsSupportCommandLists(t *testing.T) {
	t.Run("handled failures are reported with their location", func(t *testing.T) {
		box := testSandbox(t)
		writeTestScript(t, box, "report.sh", "echo start\ncat missing.log || echo fallback\necho done\n", 0o644)
		result, err := box.Execute("sh report.sh")
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		want := []OutputChunk{
			errText("/work/report.sh:2: cat: /work/missing.log: no such file"),
			out("start\nfallback\ndone\n"),
		}
		if !slices.Equal(result.Transcript, want) {
			t.Fatalf("Transcript = %+v, want %+v", result.Transcript, want)
		}
		if result.Output != "start\nfallback\ndone\n" {
			t.Errorf("Output = %q", result.Output)
		}
	})

	t.Run("an unhandled failure still stops the script", func(t *testing.T) {
		box := testSandbox(t)
		writeTestScript(t, box, "stop.sh", "echo start && cat missing.log\necho never\n", 0o644)
		_, err := box.Execute("sh stop.sh")
		if err == nil || !strings.Contains(err.Error(), "/work/stop.sh:1: cat:") {
			t.Fatalf("Execute() error = %v, want the located cat failure", err)
		}
	})

	t.Run("exit status is the last command's", func(t *testing.T) {
		box := testSandbox(t)
		writeTestScript(t, box, "check.sh", "echo checking\ngrep -q PANIC events.log\n", 0o644)
		result, err := box.Execute("sh check.sh || echo clean")
		if err != nil || result.Output != "checking\nclean\n" {
			t.Fatalf("Execute() = %q, %v; want the script's failing status to trigger ||", result.Output, err)
		}
	})

	t.Run("nested scripts chain locations", func(t *testing.T) {
		box := testSandbox(t)
		writeTestScript(t, box, "inner.sh", "cat missing.log || echo inner\n", 0o644)
		writeTestScript(t, box, "outer.sh", "sh inner.sh\necho outer\n", 0o644)
		result, err := box.Execute("sh outer.sh")
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if len(result.Transcript) == 0 || result.Transcript[0].Text != "/work/outer.sh:1: /work/inner.sh:1: cat: /work/missing.log: no such file" {
			t.Fatalf("Transcript = %+v, want the outer and inner locations", result.Transcript)
		}
	})

	t.Run("a single failing command with diagnostics earns no practice", func(t *testing.T) {
		box := testSandbox(t)
		writeTestScript(t, box, "partial.sh", "cat missing.log || echo fallback\ncat missing.log\n", 0o644)
		result, err := box.Execute("sh partial.sh")
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if result.Commands != nil {
			t.Errorf("Commands = %q, want none for a failed command", result.Commands)
		}
		last := result.Transcript[len(result.Transcript)-1]
		if !last.Error || !strings.Contains(last.Text, "partial.sh:2:") {
			t.Errorf("last transcript entry = %#v, want the script failure", last)
		}
	})
}

func TestSplitCommandList(t *testing.T) {
	items, err := splitCommandList(`a 'b;c' && d "e||f" || g\;h; i # j && k`)
	if err != nil {
		t.Fatal(err)
	}
	want := []commandListItem{
		{operator: "", line: `a 'b;c' `},
		{operator: "&&", line: ` d "e||f" `},
		{operator: "||", line: ` g\;h`},
		{operator: ";", line: ` i # j && k`},
	}
	if !slices.Equal(items, want) {
		t.Fatalf("splitCommandList() = %q, want %q", items, want)
	}
	for line, wantErr := range map[string]string{
		"; echo":         `unexpected ";"`,
		"echo a ;; b":    `unexpected ";"`,
		"echo a && || b": `unexpected "||"`,
		"echo a &&":      `cannot end with "&&"`,
		"echo a || # x":  `cannot end with "||"`,
	} {
		if _, err := splitCommandList(line); err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("splitCommandList(%q) error = %v, want %q", line, err, wantErr)
		}
	}
}
