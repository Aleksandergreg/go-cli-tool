package sandbox

import (
	"strings"
	"testing"
)

// gnuFidelityCase runs setup lines, then line, and compares the result with
// what GNU coreutils and bash print for the same input. A non-empty wantErr
// expects an error containing that substring instead.
type gnuFidelityCase struct {
	name    string
	setup   []string
	line    string
	want    string
	wantErr string
}

func runGNUFidelityCases(t *testing.T, tests []gnuFidelityCase) {
	t.Helper()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			box := testSandbox(t)
			for _, line := range test.setup {
				if _, err := box.Execute(line); err != nil {
					t.Fatalf("setup %q error = %v", line, err)
				}
			}
			result, err := box.Execute(test.line)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("Execute(%q) error = %v, want substring %q", test.line, err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Execute(%q) error = %v", test.line, err)
			}
			if result.Output != test.want {
				t.Errorf("Execute(%q) output = %q, want %q", test.line, result.Output, test.want)
			}
		})
	}
}

const fourLines = `printf 'h1\nl2\nl3\nl4\n' > /out/four.txt`

func TestDoubleQuotesKeepBackslashesBeforeOrdinaryCharacters(t *testing.T) {
	runGNUFidelityCases(t, []gnuFidelityCase{
		{name: "printf escapes survive", line: `printf "a\nb\n"`, want: "a\nb\n"},
		{name: "echo keeps backslash", line: `echo "x\ty"`, want: `x\ty` + "\n"},
		{name: "special characters still escape", line: "echo \"\\$HOME \\\\ \\\" \\`\"", want: "$HOME \\ \" `\n"},
		{name: "grep sees escaped dot", setup: []string{`printf 'a.log\naxlog\n' > /out/names.txt`}, line: `grep "a\.log" /out/names.txt`, want: "a.log\n"},
		{name: "tr deletes newlines", line: `printf 'a\nb\n' | tr -d "\n"`, want: "ab"},
		{name: "unterminated escape", line: `echo "abc\`, wantErr: "unfinished escape"},
	})
}

func TestHeadTailSignedCounts(t *testing.T) {
	runGNUFidelityCases(t, []gnuFidelityCase{
		{name: "tail from line", setup: []string{fourLines}, line: "tail -n +2 /out/four.txt", want: "l2\nl3\nl4\n"},
		{name: "tail attached from line", setup: []string{fourLines}, line: "tail -n+4 /out/four.txt", want: "l4\n"},
		{name: "tail from zero is everything", setup: []string{fourLines}, line: "tail -n +0 /out/four.txt", want: "h1\nl2\nl3\nl4\n"},
		{name: "tail from past end", setup: []string{fourLines}, line: "tail -n +9 /out/four.txt", want: ""},
		{name: "tail minus counts from end", setup: []string{fourLines}, line: "tail -n -1 /out/four.txt", want: "l4\n"},
		{name: "head all but last", setup: []string{fourLines}, line: "head -n -1 /out/four.txt", want: "h1\nl2\nl3\n"},
		{name: "head all but more than all", setup: []string{fourLines}, line: "head -n -9 /out/four.txt", want: ""},
		{name: "head attached count", setup: []string{fourLines}, line: "head -n2 /out/four.txt", want: "h1\nl2\n"},
		{name: "double sign", setup: []string{fourLines}, line: "tail -n ++2 /out/four.txt", wantErr: "invalid line count"},
		{name: "not a number", setup: []string{fourLines}, line: "head -n two /out/four.txt", wantErr: "invalid line count"},
	})
}

func TestSortNumericUsesLeadingNumber(t *testing.T) {
	runGNUFidelityCases(t, []gnuFidelityCase{
		{name: "units", line: `printf '10ms\n9ms\n100ms\n2ms\n' | sort -n`, want: "2ms\n9ms\n10ms\n100ms\n"},
		{name: "signs decimals and blanks", line: `printf '  3.5 x\n-1.25\n3\n.5\n' | sort -n`, want: "-1.25\n.5\n3\n  3.5 x\n"},
		{name: "non-numeric lines are zero", line: `printf '5\nalpha\n-2\n' | sort -n`, want: "-2\nalpha\n5\n"},
		{name: "exponent is not numeric", line: `printf '1e3\n20\n' | sort -n`, want: "1e3\n20\n"},
		{name: "reverse", line: `printf '10ms\n9ms\n100ms\n' | sort -rn`, want: "100ms\n10ms\n9ms\n"},
		{name: "unique by numeric key", line: `printf '2 b\n1 x\n2 a\n' | sort -nu`, want: "1 x\n2 a\n"},
	})
}

func TestWCMatchesGNUCountsAndLayout(t *testing.T) {
	runGNUFidelityCases(t, []gnuFidelityCase{
		{name: "all counts by default", setup: []string{fourLines}, line: "wc /out/four.txt", want: " 4  4 12 /out/four.txt\n"},
		{name: "combined flags", setup: []string{fourLines}, line: "wc -lw /out/four.txt", want: " 4  4 /out/four.txt\n"},
		{name: "separate flags keep fixed order", setup: []string{fourLines}, line: "wc -c -l /out/four.txt", want: " 4 12 /out/four.txt\n"},
		{name: "single count is unpadded", setup: []string{fourLines}, line: "wc -l /out/four.txt", want: "4 /out/four.txt\n"},
		{name: "multiple files add total", setup: []string{fourLines}, line: "wc -l /out/four.txt events.log", want: " 4 /out/four.txt\n 3 events.log\n 7 total\n"},
		{name: "standard input default width", line: `printf 'a b\n' | wc`, want: "      1       2       4\n"},
		{name: "standard input single count", line: "grep ERROR events.log | wc -l", want: "2\n"},
		{name: "unknown option", line: "wc -m events.log", wantErr: "unknown option -m"},
	})
}

func TestPrintfDirectivesAndFormatReuse(t *testing.T) {
	runGNUFidelityCases(t, []gnuFidelityCase{
		{name: "format reuse", line: `printf '%s\n' a b c`, want: "a\nb\nc\n"},
		{name: "partial final pass", line: `printf '%s=%s\n' a 1 b`, want: "a=1\nb=\n"},
		{name: "no directives ignores values", line: `printf 'ready\n' extra`, want: "ready\n"},
		{name: "integers", line: `printf '%d items, %i left\n' 5 -2`, want: "5 items, -2 left\n"},
		{name: "missing integer is zero", line: `printf '[%d]\n'`, want: "[0]\n"},
		{name: "width precision and flags", line: `printf '%-6s|%3d|%05d|%.2s\n' ab 7 42 xyz`, want: "ab    |  7|00042|xy\n"},
		{name: "percent literal", line: `printf '100%%\n'`, want: "100%\n"},
		{name: "unsupported conversion", line: `printf '%f\n' 1`, wantErr: `unsupported directive "%f"`},
		{name: "malformed flags", line: `printf '%1.2.3s\n' a`, wantErr: "unsupported directive"},
		{name: "invalid integer", line: `printf '%d\n' five`, wantErr: `"five": invalid number`},
		{name: "trailing percent", line: `printf '100%'`, wantErr: "missing conversion character"},
		{name: "oversized width", line: `printf '%999999999s'`, wantErr: "field limit"},
		{name: "oversized precision", line: `printf '%.999999999d' 1`, wantErr: "field limit"},
	})
}

func TestTrCharacterClassesAndOperands(t *testing.T) {
	runGNUFidelityCases(t, []gnuFidelityCase{
		{name: "upper case", setup: []string{fourLines}, line: "tr '[:lower:]' '[:upper:]' < /out/four.txt", want: "H1\nL2\nL3\nL4\n"},
		{name: "delete digits", setup: []string{fourLines}, line: "tr -d '[:digit:]' < /out/four.txt", want: "h\nl\nl\nl\n"},
		{name: "squeeze spaces", line: `printf 'a  \t b\n' | tr -s '[:blank:]'`, want: "a \t b\n"},
		{name: "classes mix with ranges", line: `printf 'Ab9-\n' | tr -d '[:upper:]0-9'`, want: "b-\n"},
		{name: "delete then squeeze second set", line: `printf 'axx  b   c\n' | tr -ds x ' '`, want: "a b c\n"},
		{name: "translate then squeeze", line: `printf 'aabbcc\n' | tr -s ab xx`, want: "xcc\n"},
		{name: "unknown class", line: `printf 'a\n' | tr -d '[:vowel:]'`, wantErr: "unknown character class [:vowel:]"},
		{name: "delete extra operand", line: `printf 'a\n' | tr -d a b`, wantErr: "usage: tr"},
		{name: "delete squeeze needs two sets", line: `printf 'a\n' | tr -ds a`, wantErr: "usage: tr"},
		{name: "translate needs two sets", line: `printf 'a\n' | tr a`, wantErr: "usage: tr"},
	})
}

func TestGrepUsesBasicRegexUnlessExtended(t *testing.T) {
	logs := `printf 'ERROR x\nWARN y\nINFO (z)\na|b\n2^3 $5\n*star\n' > /out/mixed.log`
	runGNUFidelityCases(t, []gnuFidelityCase{
		{name: "escaped alternation", setup: []string{logs}, line: `grep 'ERROR\|WARN' /out/mixed.log`, want: "ERROR x\nWARN y\n"},
		{name: "bare bar is literal", setup: []string{logs}, line: `grep 'a|b' /out/mixed.log`, want: "a|b\n"},
		{name: "extended alternation", setup: []string{logs}, line: `grep -E 'ERROR|WARN' /out/mixed.log`, want: "ERROR x\nWARN y\n"},
		{name: "escaped plus", setup: []string{logs}, line: `grep -c '^[A-Z]\+ ' /out/mixed.log`, want: "3\n"},
		{name: "bare plus is literal", setup: []string{logs}, line: `grep -c 'R+' /out/mixed.log`, want: "0\n"},
		{name: "bare parentheses are literal", setup: []string{logs}, line: `grep '(z)' /out/mixed.log`, want: "INFO (z)\n"},
		{name: "escaped group with interval", setup: []string{logs}, line: `grep '\(R\)\{2\}' /out/mixed.log`, want: "ERROR x\n"},
		{name: "leading star is literal", setup: []string{logs}, line: `grep '*star' /out/mixed.log`, want: "*star\n"},
		{name: "star after anchor is literal", setup: []string{logs}, line: `grep '^*' /out/mixed.log`, want: "*star\n"},
		{name: "inner caret and dollar are literal", setup: []string{logs}, line: `grep '2^3 $5' /out/mixed.log`, want: "2^3 $5\n"},
		{name: "bracket classes", setup: []string{logs}, line: `grep '^[[:upper:]]\{4\} [a-z]$' /out/mixed.log`, want: "WARN y\n"},
		{name: "bracket with leading close", setup: []string{logs}, line: `grep -c '[]|]' /out/mixed.log`, want: "1\n"},
		{name: "word boundary", setup: []string{logs}, line: `grep '\<y\>' /out/mixed.log`, want: "WARN y\n"},
		{name: "backreference", setup: []string{logs}, line: `grep '\(a\)\1' /out/mixed.log`, wantErr: "backreferences"},
		{name: "unterminated bracket", setup: []string{logs}, line: `grep '[abc' /out/mixed.log`, wantErr: "unterminated bracket"},
	})
}

func TestSedReplacementSyntax(t *testing.T) {
	config := `printf 'LOG_LEVEL=debug\npath=/var/log\n' > /out/app.env`
	runGNUFidelityCases(t, []gnuFidelityCase{
		{name: "ampersand is the match", setup: []string{config}, line: `sed 's/debug/[&]/' /out/app.env`, want: "LOG_LEVEL=[debug]\npath=/var/log\n"},
		{name: "escaped ampersand is literal", setup: []string{config}, line: `sed 's/debug/\&/' /out/app.env`, want: "LOG_LEVEL=&\npath=/var/log\n"},
		{name: "basic group reference", setup: []string{config}, line: `sed 's/\(LOG_LEVEL\)=debug/\1=info/' /out/app.env`, want: "LOG_LEVEL=info\npath=/var/log\n"},
		{name: "extended group reference", setup: []string{config}, line: `sed -E 's/(LOG)_(LEVEL)/\2_\1/' /out/app.env`, want: "LEVEL_LOG=debug\npath=/var/log\n"},
		{name: "dollar is literal", setup: []string{config}, line: `sed 's/debug/$1 cost/' /out/app.env`, want: "LOG_LEVEL=$1 cost\npath=/var/log\n"},
		{name: "escaped backslash", setup: []string{config}, line: `sed 's/debug/a\\b/' /out/app.env`, want: "LOG_LEVEL=a\\b\npath=/var/log\n"},
		{name: "backslash pattern", line: `printf 'a\\b\n' | sed 's/\\/X/'`, want: "aXb\n"},
		{name: "escaped delimiter", setup: []string{config}, line: `sed 's/\//_/g' /out/app.env`, want: "LOG_LEVEL=debug\npath=_var_log\n"},
		{name: "basic plus is literal", setup: []string{config}, line: `sed 's/[a-z]+/X/' /out/app.env`, want: "LOG_LEVEL=debug\npath=/var/log\n"},
		{name: "global ampersand", line: `printf 'ab\n' | sed 's/[ab]/&&/g'`, want: "aabb\n"},
		{name: "invalid group reference", setup: []string{config}, line: `sed 's/debug/\1/' /out/app.env`, wantErr: `invalid reference \1`},
	})
}

func TestShellGlobNegatedBrackets(t *testing.T) {
	runGNUFidelityCases(t, []gnuFidelityCase{
		{name: "argument glob", line: "echo [!e]*.log", want: "quiet.log\n"},
		{name: "find name", line: "find . -type f -name '[!q]*'", want: "./events.log\n"},
	})

	for _, test := range []struct {
		pattern, name string
		want          bool
	}{
		{pattern: "[!e]*", name: "quiet.log", want: true},
		{pattern: "[!e]*", name: "events.log", want: false},
		{pattern: `\[!x]`, name: "[!x]", want: true},
		{pattern: `\[!x]`, name: "a", want: false},
	} {
		if got, err := matchShellPattern(test.pattern, test.name); err != nil || got != test.want {
			t.Errorf("matchShellPattern(%q, %q) = %v, %v; want %v", test.pattern, test.name, got, err, test.want)
		}
	}
}

func TestRemoveRefusesDirectoriesWithoutRecursion(t *testing.T) {
	runGNUFidelityCases(t, []gnuFidelityCase{
		{name: "empty directory", setup: []string{"mkdir /out/empty"}, line: "rm /out/empty", wantErr: "is a directory"},
		{name: "force does not imply recursion", setup: []string{"mkdir /out/empty"}, line: "rm -f /out/empty", wantErr: "is a directory"},
		{name: "recursive removes", setup: []string{"mkdir /out/empty"}, line: "rm -r /out/empty", want: ""},
		{name: "rmdir removes", setup: []string{"mkdir /out/empty"}, line: "rmdir /out/empty", want: ""},
	})

	box := testSandbox(t)
	if _, err := box.Execute("mkdir /out/empty"); err != nil {
		t.Fatal(err)
	}
	if _, err := box.Execute("rm /out/empty"); err == nil || !box.FS.IsDir("/out/empty") {
		t.Fatalf("rm without -r removed a directory: err %v", err)
	}
}

func TestOptionlessCommandsRejectUnknownOptions(t *testing.T) {
	runGNUFidelityCases(t, []gnuFidelityCase{
		{name: "cat", line: "cat -n events.log", wantErr: "unknown option -n"},
		{name: "less", line: "less -S events.log", wantErr: "unknown option -S"},
		{name: "touch", line: "touch -a /out/new.txt", wantErr: "unknown option -a"},
		{name: "mv", line: "mv -f events.log /out/", wantErr: "unknown option -f"},
		{name: "stat", line: "stat -c '%a' events.log", wantErr: "unknown option -c"},
		{name: "gzip", line: "gzip -d events.log", wantErr: "unknown option -d"},
		{name: "gunzip", line: "gunzip -k events.log.gz", wantErr: "unknown option -k"},
		{name: "gzip directory", line: "gzip /out", wantErr: "is a directory"},
		{name: "gzip compressed", setup: []string{"gzip quiet.log"}, line: "gzip quiet.log.gz", wantErr: "already has .gz suffix"},
		{name: "end of options", setup: []string{"touch -- -dash.txt"}, line: "cat -- -dash.txt", want: ""},
	})

	box := testSandbox(t)
	for _, line := range []string{"touch -a /out/new.txt", "mv -f events.log /out/"} {
		if _, err := box.Execute(line); err == nil {
			t.Fatalf("Execute(%q) unexpectedly succeeded", line)
		}
	}
	if box.FS.Exists("/out/new.txt") || box.FS.Exists("/work/-a") || !box.FS.Exists("/work/events.log") {
		t.Fatalf("rejected options changed the filesystem: %v", box.FS.Paths())
	}
}

func TestTranslateBasicRegex(t *testing.T) {
	tests := []struct {
		basic string
		want  string
	}{
		{basic: `ERROR\|WARN`, want: `ERROR|WARN`},
		{basic: `a|b+c?`, want: `a\|b\+c\?`},
		{basic: `\(ab\)\{2,3\}`, want: `(ab){2,3}`},
		{basic: `(x){y}`, want: `\(x\)\{y\}`},
		{basic: `*a`, want: `\*a`},
		{basic: `^*a`, want: `^\*a`},
		{basic: `\(*a\)`, want: `(\*a)`},
		{basic: `a^b`, want: `a\^b`},
		{basic: `^^`, want: `^\^`},
		{basic: `a$b$`, want: `a\$b$`},
		{basic: `\(a$\)`, want: `(a$)`},
		{basic: `[]a\]x`, want: `[]a\\]x`},
		{basic: `[^[:space:]]`, want: `[^[:space:]]`},
		{basic: `\.\*\/`, want: `\.\*\/`},
		{basic: `\<w\>`, want: `\bw\b`},
	}
	for _, test := range tests {
		got, err := translateBasicRegex(test.basic)
		if err != nil || got != test.want {
			t.Errorf("translateBasicRegex(%q) = %q, %v; want %q", test.basic, got, err, test.want)
		}
	}
	for _, invalid := range []string{`a\`, `[abc`, `\(a\)\1`} {
		if _, err := translateBasicRegex(invalid); err == nil {
			t.Errorf("translateBasicRegex(%q) unexpectedly succeeded", invalid)
		}
	}
}

func TestOutputRedirectionEmptiesItsTargetBeforeTheCommandRuns(t *testing.T) {
	runGNUFidelityCases(t, []gnuFidelityCase{
		{name: "sort onto its input", setup: []string{fourLines, "sort /out/four.txt > /out/four.txt"}, line: "wc -c /out/four.txt", want: "0 /out/four.txt\n"},
		{name: "input redirection from the target", setup: []string{fourLines, "cat < /out/four.txt > /out/four.txt"}, line: "wc -c /out/four.txt", want: "0 /out/four.txt\n"},
		{name: "append keeps its input", setup: []string{fourLines, "head -n 1 /out/four.txt >> /out/four.txt"}, line: "tail -n 2 /out/four.txt", want: "l4\nh1\n"},
		{name: "directory target fails before running", line: "touch /out/new.txt > /out", wantErr: "redirect"},
	})

	box := testSandbox(t)
	if _, err := box.Execute("touch /out/new.txt > /out"); err == nil || box.FS.Exists("/out/new.txt") {
		t.Fatalf("a rejected redirection ran its command: err %v", err)
	}
}

func TestFailedStageRestoresItsRedirectTarget(t *testing.T) {
	box := testSandbox(t)
	if err := box.FS.WriteFile("/out/report", "keep\n", 0o640); err != nil {
		t.Fatal(err)
	}
	if err := box.FS.Chown("/out/report", "reviewer"); err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		"cat missing.log > /out/report",
		"cat missing.log > /out/created",
		"grep ERROR < missing.log > /out/report",
	} {
		if _, err := box.Execute(line); err == nil {
			t.Fatalf("Execute(%q) unexpectedly succeeded", line)
		}
	}
	entry, exists := box.FS.Entry("/out/report")
	if !exists || entry.Content != "keep\n" || entry.Mode != 0o640 || entry.Owner != "reviewer" {
		t.Errorf("failed command changed its destination: %#v", entry)
	}
	if box.FS.Exists("/out/created") {
		t.Error("failed command left a destination it created")
	}

	// Earlier stages that finished keep their effects, as before.
	if _, err := box.Execute("echo kept > /out/first.txt | cat missing.log"); err == nil {
		t.Fatal("pipeline unexpectedly succeeded")
	}
	if content, _ := box.FS.ReadFile("/out/first.txt"); content != "kept\n" {
		t.Errorf("completed stage output = %q, want kept", content)
	}

	// Archive metadata comes back with a restored archive.
	if _, err := box.Execute("tar -cf /out/logs.tar events.log"); err != nil {
		t.Fatal(err)
	}
	if _, err := box.Execute("cat missing.log > /out/logs.tar"); err == nil {
		t.Fatal("failing redirect unexpectedly succeeded")
	}
	if result, err := box.Execute("tar -tf /out/logs.tar"); err != nil || !strings.Contains(result.Output, "events.log") {
		t.Errorf("restored archive listing = %q, %v", result.Output, err)
	}
}

func TestListingFollowsGNULayout(t *testing.T) {
	tree := []string{"mkdir -p /out/d1/d2", "touch /out/d1/f /out/d1/g /out/d1/.env /out/top /out/.hidden"}
	runGNUFidelityCases(t, []gnuFidelityCase{
		{name: "file operand keeps its path", setup: tree, line: "ls /out/d1/f", want: "/out/d1/f\n"},
		{name: "files before directories", setup: tree, line: "ls /out/d1 /out/top", want: "/out/top\n\n/out/d1:\nd2\nf\ng\n"},
		{name: "directories sorted with headers", setup: tree, line: "ls /out/d1/d2 /out/d1", want: "/out/d1:\nd2\nf\ng\n\n/out/d1/d2:\n"},
		{name: "single directory has no header", setup: tree, line: "ls /out/d1", want: "d2\nf\ng\n"},
		{name: "all shows dot entries", setup: tree, line: "ls -a /out/d1", want: ".\n..\n.env\nd2\nf\ng\n"},
		{name: "named hidden file is shown", setup: tree, line: "ls /out/.hidden", want: "/out/.hidden\n"},
		{name: "long file operand", setup: tree, line: "ls -l /out/top", want: "-rw-r--r-- operator      0 /out/top\n"},
	})
}

func TestGlobsSkipHiddenNamesUnlessTheDotIsExplicit(t *testing.T) {
	tree := []string{"mkdir -p /out/d", "touch /out/a /out/.env /out/.profile /out/d/b /out/d/.secret"}
	runGNUFidelityCases(t, []gnuFidelityCase{
		{name: "star", setup: tree, line: "echo /out/*", want: "/out/a /out/d\n"},
		{name: "dot star", setup: tree, line: "echo /out/.*", want: "/out/.env /out/.profile\n"},
		{name: "dot prefix", setup: tree, line: "echo /out/.e*", want: "/out/.env\n"},
		{name: "nested star", setup: tree, line: "echo /out/*/*", want: "/out/d/b\n"},
		{name: "question mark", setup: tree, line: "echo /out/?", want: "/out/a /out/d\n"},
		{name: "rm star keeps hidden files", setup: append(tree, "rm /out/a"), line: "ls -a /out", want: ".\n..\n.env\n.profile\nd\n"},
		{name: "find still matches hidden names", setup: tree, line: "find /out/d -name '*' -type f", want: "/out/d/.secret\n/out/d/b\n"},
	})
}

func TestCutFieldListsAndUndelimitedLines(t *testing.T) {
	rows := `printf 'a:b:c:d\nplain\n' > /out/rows.txt`
	runGNUFidelityCases(t, []gnuFidelityCase{
		{name: "list", setup: []string{rows}, line: "cut -d: -f1,3 /out/rows.txt", want: "a:c\nplain\n"},
		{name: "open range", setup: []string{rows}, line: "cut -d : -f 3- /out/rows.txt", want: "c:d\nplain\n"},
		{name: "leading range", setup: []string{rows}, line: "cut -d: -f-2 /out/rows.txt", want: "a:b\nplain\n"},
		{name: "overlapping ranges keep input order", setup: []string{rows}, line: "cut -d: -f4,1-2,2 /out/rows.txt", want: "a:b:d\nplain\n"},
		{name: "fields past the end", setup: []string{rows}, line: "cut -d: -f9 /out/rows.txt", want: "\nplain\n"},
		{name: "huge range", setup: []string{rows}, line: "cut -d: -f2-999999999 /out/rows.txt", want: "b:c:d\nplain\n"},
		{name: "only delimited", setup: []string{rows}, line: "cut -s -d: -f2 /out/rows.txt", want: "b\n"},
		{name: "tab default", line: `printf 'x\ty\n' | cut -f2`, want: "y\n"},
		{name: "multi-character delimiter", setup: []string{rows}, line: "cut -d ', ' -f1 /out/rows.txt", wantErr: "single character"},
		{name: "zero field", setup: []string{rows}, line: "cut -d: -f0 /out/rows.txt", wantErr: "numbered from 1"},
		{name: "decreasing range", setup: []string{rows}, line: "cut -d: -f3-1 /out/rows.txt", wantErr: "decreasing range"},
		{name: "malformed list", setup: []string{rows}, line: "cut -d: -f1,x /out/rows.txt", wantErr: "invalid field list"},
		{name: "missing list", setup: []string{rows}, line: "cut -d: /out/rows.txt", wantErr: "select fields with -f"},
	})
}

func TestFindCombinesTestsWithAnd(t *testing.T) {
	tree := []string{"mkdir -p /out/logs", "touch /out/logs/app.log /out/logs/api.txt /out/access.log"}
	runGNUFidelityCases(t, []gnuFidelityCase{
		{name: "two names", setup: tree, line: "find /out -name 'a*' -name '*.log'", want: "/out/access.log\n/out/logs/app.log\n"},
		{name: "contradictory types", setup: tree, line: "find /out -type f -type d", want: ""},
		{name: "explicit and", setup: tree, line: "find /out -type f -a -name '*.txt'", want: "/out/logs/api.txt\n"},
		{name: "name and iname", setup: tree, line: "find /out -iname 'API*' -name '*.txt'", want: "/out/logs/api.txt\n"},
		{name: "exec only on matches", setup: tree, line: `find /out -name '*.log' -name 'app*' -exec basename {} \;`, want: "app.log\n"},
		{name: "exec status does not stop find", line: `find . -type f -exec grep -l ERROR {} \;`, want: "./events.log\n"},
		{name: "or is rejected", setup: tree, line: "find /out -name a -o -name b", wantErr: "implicit AND"},
		{name: "second exec is rejected", setup: tree, line: `find /out -exec basename {} \; -exec basename {} \;`, wantErr: "one -exec"},
	})
}
