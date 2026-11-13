package sandbox

import (
	"reflect"
	"strings"
	"testing"
)

func TestTextCommandsPreserveOneBlankLine(t *testing.T) {
	lineTests := []struct {
		name string
		text string
		want []string
	}{
		{name: "empty input", text: "", want: nil},
		{name: "unterminated line", text: "alpha", want: []string{"alpha"}},
		{name: "terminated line", text: "alpha\n", want: []string{"alpha"}},
		{name: "single blank line", text: "\n", want: []string{""}},
		{name: "two blank lines", text: "\n\n", want: []string{"", ""}},
		{name: "text then blank line", text: "alpha\n\n", want: []string{"alpha", ""}},
	}
	for _, test := range lineTests {
		t.Run("line splitter/"+test.name, func(t *testing.T) {
			if got := textLines(test.text); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("textLines(%q) = %#v, want %#v", test.text, got, test.want)
			}
		})
	}

	commands := []string{
		`printf '\n' | head -n 1`,
		`printf '\n' | tail -n 1`,
		`printf '\n' | grep '^$'`,
		`printf '\n' | sort`,
		`printf '\n' | uniq`,
		`printf '\n' | awk '{print $1}'`,
		`printf '\n' | cut -d : -f 1`,
	}
	for _, command := range commands {
		t.Run(command, func(t *testing.T) {
			box := testSandbox(t)
			result, err := box.Execute(command)
			if err != nil {
				t.Fatalf("Execute(%q) error = %v", command, err)
			}
			if result.Output != "\n" {
				t.Errorf("Execute(%q) output = %q, want one blank line", command, result.Output)
			}
		})
	}
}

func TestDoubleQuotesKeepBackslashSequencesForCommands(t *testing.T) {
	box := testSandbox(t)
	for _, test := range []struct {
		command string
		want    string
	}{
		{command: `printf "x\ty\n"`, want: "x\ty\n"},
		{command: `printf 'x\ty\n'`, want: "x\ty\n"},
		{command: `printf "a\nb\n" | tr "\n" " "`, want: "a b "},
		{command: `echo "a\nb"`, want: "a\\nb\n"},
		{command: `echo "\$TARGET \"quoted\" back\\slash"`, want: "$TARGET \"quoted\" back\\slash\n"},
		{command: `echo "$TARGET\n"`, want: "staging\\n\n"},
		{command: `echo a\nb`, want: "anb\n"},
		{command: `grep -c "api\b" events.log`, want: "2\n"},
		{command: `printf "a.b\naxb\n" | grep "a\.b"`, want: "a.b\n"},
	} {
		result, err := box.Execute(test.command)
		if err != nil || result.Output != test.want {
			t.Errorf("%s output = %q, error %v; want %q", test.command, result.Output, err, test.want)
		}
	}
	if _, err := box.Execute(`echo "unfinished\`); err == nil {
		t.Error("unfinished escape inside double quotes unexpectedly succeeded")
	}
}

func TestSedReplacementUsesSedSyntax(t *testing.T) {
	box := testSandbox(t)
	for _, test := range []struct {
		command string
		want    string
	}{
		{command: `printf 'COST=x\n' | sed 's/x/$100/'`, want: "COST=$100\n"},
		{command: `printf 'COST=x\n' | sed "s/x/\$100/"`, want: "COST=$100\n"},
		{command: `printf 'COST=x\n' | sed 's/x/${1}$$/'`, want: "COST=${1}$$\n"},
		{command: `printf 'COST=x\n' | sed 's/COST/[&]/'`, want: "[COST]=x\n"},
		{command: `printf 'COST=x\n' | sed 's/COST/\&/'`, want: "&=x\n"},
		{command: `printf 'COST=x\n' | sed 's/(COST)=(x)/\2=\1/'`, want: "x=COST\n"},
		{command: `printf 'COST=x\n' | sed -E 's/(COST)/\1S/'`, want: "COSTS=x\n"},
		{command: `printf 'COST=x\n' | sed 's/(COST)/\0\0/'`, want: "COSTCOST=x\n"},
		{command: `printf 'a a\n' | sed 's/a/<&>/g'`, want: "<a> <a>\n"},
		{command: `printf 'a/b\n' | sed 's/\//\\/'`, want: "a\\b\n"},
		{command: `printf 'a b\n' | sed 's/ /\t/'`, want: "a\tb\n"},
	} {
		result, err := box.Execute(test.command)
		if err != nil || result.Output != test.want {
			t.Errorf("%s output = %q, error %v; want %q", test.command, result.Output, err, test.want)
		}
	}
}

func TestSedRejectsReferencesToMissingGroups(t *testing.T) {
	box := testSandbox(t)
	before, _ := box.FS.ReadFile("/work/quiet.log")
	for command, want := range map[string]string{
		`sed -i 's/\(INFO\)/\1!/' quiet.log`: "group with ( )",
		`sed -i 's/(INFO)/\2!/' quiet.log`:   "has 1 capture group(s)",
	} {
		_, err := box.Execute(command)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s error = %v, want mention of %q", command, err, want)
		}
	}
	if after, _ := box.FS.ReadFile("/work/quiet.log"); after != before {
		t.Errorf("rejected sed changed the file: %q", after)
	}
	manual, err := box.Execute(`help sed`)
	if err != nil || !strings.Contains(manual.Output, `\1 to \9`) || !strings.Contains(manual.Output, "extended") {
		t.Errorf("help sed = %q, %v", manual.Output, err)
	}
}
