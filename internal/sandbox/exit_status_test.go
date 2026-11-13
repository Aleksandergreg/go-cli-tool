package sandbox

import "testing"

func TestExitStatusMatchesTheShell(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "success", line: "true; echo $?", want: "0\n"},
		{name: "false", line: "false; echo $?", want: "1\n"},
		{name: "ordinary failure", line: "cat missing.log; echo $?", want: "1\n"},
		{name: "ls trouble", line: "ls /missing; echo $?", want: "2\n"},
		{name: "grep match", line: "grep -q ERROR events.log; echo $?", want: "0\n"},
		{name: "grep no match", line: "grep PANIC events.log; echo $?", want: "1\n"},
		{name: "grep trouble", line: "grep ERROR missing.log; echo $?", want: "2\n"},
		{name: "sort trouble", line: "sort missing.log; echo $?", want: "2\n"},
		{name: "unknown command", line: "deploy-everything; echo $?", want: "127\n"},
		{name: "missing script", line: "sh missing.sh; echo $?", want: "127\n"},
		{name: "missing executable", line: "./missing.sh; echo $?", want: "127\n"},
		{name: "directory as script", line: "sh /work; echo $?", want: "126\n"},
		{name: "pipeline is its last stage", line: "grep PANIC events.log | wc -l; echo $?", want: "0\n0\n"},
		{name: "skipped command keeps the status", line: "false && echo never; echo $?", want: "1\n"},
		{name: "status updates after each command", line: "false; true; echo $?", want: "0\n"},
		{name: "braces", line: "false; echo ${?}", want: "1\n"},
		{name: "double quotes", line: `false; echo "status=$?"`, want: "status=1\n"},
		{name: "single quotes are literal", line: `false; echo '$?'`, want: "$?\n"},
		{name: "escaped is literal", line: `false; echo \$?`, want: "$?\n"},
		{name: "starts at zero", line: "echo $?", want: "0\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			box := testSandbox(t)
			result, err := box.Execute(test.line)
			if err != nil {
				t.Fatalf("Execute(%q) error = %v", test.line, err)
			}
			if result.Output != test.want {
				t.Errorf("Execute(%q) output = %q, want %q", test.line, result.Output, test.want)
			}
		})
	}
}

func TestExitStatusPersistsAcrossLines(t *testing.T) {
	box := testSandbox(t)
	writeTestScript(t, box, "plain.sh", "echo hi\n", 0o644)
	steps := []struct{ line, want string }{
		{line: "ls /missing"},
		{line: "echo $?", want: "2\n"},
		{line: "echo $?", want: "0\n"},
		{line: "false"},
		{line: ""},
		{line: "   # a comment"},
		{line: "echo $?", want: "1\n"},
		{line: "echo 'unterminated"},
		{line: "echo $?", want: "2\n"},
		{line: "echo a &&"},
		{line: "echo $?", want: "2\n"},
		{line: "echo $(pwd)"},
		{line: "echo $?", want: "2\n"},
		{line: "./plain.sh"},
		{line: "echo $?", want: "126\n"},
	}
	for _, step := range steps {
		result, err := box.Execute(step.line)
		if step.want == "" {
			continue
		}
		if err != nil || result.Output != step.want {
			t.Fatalf("Execute(%q) = %q, %v; want %q", step.line, result.Output, err, step.want)
		}
	}
}

func TestScriptsHaveTheirOwnExitStatus(t *testing.T) {
	box := testSandbox(t)
	writeTestScript(t, box, "inner-status.sh", "false\necho \"inside=$?\"\n", 0o644)
	writeTestScript(t, box, "fresh.sh", "echo \"start=$?\"\n", 0o644)
	writeTestScript(t, box, "fails.sh", "echo start\nls /missing\n", 0o644)
	writeTestScript(t, box, "last.sh", "echo checking\ngrep -q PANIC events.log\n", 0o644)
	steps := []struct{ line, want string }{
		{line: "sh inner-status.sh", want: "inside=1\n"},
		{line: "false; sh fresh.sh", want: "start=0\n"},
		{line: "sh fails.sh; echo $?", want: "2\n"},
		{line: "sh last.sh; echo $?", want: "checking\n1\n"},
	}
	for _, step := range steps {
		result, err := box.Execute(step.line)
		if err != nil || result.Output != step.want {
			t.Fatalf("Execute(%q) = %q, %v; want %q", step.line, result.Output, err, step.want)
		}
	}
}
