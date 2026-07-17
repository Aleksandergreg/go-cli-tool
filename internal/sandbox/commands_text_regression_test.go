package sandbox

import (
	"reflect"
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
