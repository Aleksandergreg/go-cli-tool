package sandbox

import "testing"

func TestTextCommandsPreserveOneBlankLine(t *testing.T) {
	t.Run("line splitter", func(t *testing.T) {
		lines := textLines("\n")
		if len(lines) != 1 || lines[0] != "" {
			t.Fatalf("textLines(single blank line) = %#v", lines)
		}
	})

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
