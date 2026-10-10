package sandbox

import (
	"strings"
	"testing"
)

func TestInteractiveLinesRejectUnsupportedShellSyntaxBeforeRunning(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "empty list item", line: "touch /out/first.txt ;; touch /out/second.txt", want: `syntax error near unexpected ";"`},
		{name: "leading operator", line: "&& touch /out/first.txt", want: `syntax error near unexpected "&&"`},
		{name: "dangling or", line: "touch /out/first.txt ||", want: `cannot end with "||"`},
		{name: "unsupported syntax later in a list", line: "touch /out/first.txt; echo $(pwd)", want: "command substitution"},
		{name: "parse error later in a list", line: "touch /out/first.txt && | touch /out/second.txt", want: "unexpected pipe"},
		{name: "editor in a list", line: "touch /out/first.txt; vi events.log", want: "cannot be combined"},
		{name: "background job", line: "touch /out/first.txt &", want: "background jobs"},
		{name: "subshell", line: "(touch /out/first.txt)", want: `shell control syntax "("`},
		{name: "stderr redirection", line: "touch /out/first.txt 2>/out/errors.txt", want: `redirection such as "2>"`},
		{name: "stderr append", line: "touch /out/first.txt 2>>/out/errors.txt", want: `redirection such as "2>"`},
		{name: "stderr to stdout", line: "touch /out/first.txt 2>&1", want: `redirection such as "2>"`},
		{name: "stdout to stderr", line: "touch /out/first.txt >&2", want: `redirection such as ">&"`},
		{name: "both streams", line: "touch /out/first.txt &>/out/all.txt", want: `redirection such as "&>"`},
		{name: "numbered input", line: "touch /out/first.txt 0</work/events.log", want: `redirection such as "0<"`},
		{name: "backticks", line: "touch /out/`pwd`", want: "command substitution"},
		{name: "dollar substitution", line: "touch /out/$(pwd)", want: "command substitution"},
		{name: "exit status", line: "touch /out/$?", want: `"$?"`},
		{name: "braced positional", line: "touch /out/${1}", want: `"${1}"`},
		{name: "quoted positional", line: `touch "/out/$1"`, want: `"$1"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			box := testSandbox(t)
			_, err := box.Execute(test.line)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Execute(%q) error = %v, want substring %q", test.line, err, test.want)
			}
			for _, path := range []string{"/out/first.txt", "/out/second.txt", "/out/errors.txt", "/out/all.txt"} {
				if box.FS.Exists(path) {
					t.Errorf("rejected line %q created %s", test.line, path)
				}
			}
			if len(box.History) != 1 || box.History[0] != test.line {
				t.Errorf("history = %q, want the rejected line recorded once", box.History)
			}
		})
	}
}

func TestInteractiveLinesKeepSupportedShellSyntax(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "quoted punctuation", line: `echo '; && || & () ` + "`pwd`" + ` $(pwd) $? 2>x'`, want: "; && || & () `pwd` $(pwd) $? 2>x\n"},
		{name: "escaped punctuation", line: `echo \; \& \$? 2\>x`, want: "; & $? 2>x\n"},
		{name: "spaced digit argument", line: "echo 2 > /out/digit.txt", want: ""},
		{name: "quoted digit before redirection", line: `echo hi '2'>/out/quoted.txt`, want: ""},
		{name: "option before redirection", line: "head -2>/out/head.txt events.log", want: ""},
		{name: "find exec terminator", line: `find . -name '*.log' -exec grep -l ERROR {} \;`, want: "./events.log\n"},
		{name: "named variables", line: `echo "$TARGET" ${TARGET}`, want: "staging staging\n"},
		{name: "trailing comment", line: "echo ok # ; && $(pwd)", want: "ok\n"},
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

	t.Run("comment after an operator matches the lexer", func(t *testing.T) {
		for _, line := range []string{"cat events.log |# ; && $?", "echo hi >#; $(pwd)"} {
			if err := validateShellSyntax(line); err != nil {
				t.Errorf("validateShellSyntax(%q) treated a comment as code: %v", line, err)
			}
		}
	})

	t.Run("digit arguments still redirect stdout", func(t *testing.T) {
		box := testSandbox(t)
		for _, line := range []string{"echo 2 > /out/digit.txt", `echo hi '2'>/out/quoted.txt`} {
			if _, err := box.Execute(line); err != nil {
				t.Fatalf("Execute(%q) error = %v", line, err)
			}
		}
		for path, want := range map[string]string{"/out/digit.txt": "2\n", "/out/quoted.txt": "hi 2\n"} {
			content, err := box.FS.ReadFile(path)
			if err != nil || content != want {
				t.Errorf("%s = %q, %v; want %q", path, content, err, want)
			}
		}
	})
}
