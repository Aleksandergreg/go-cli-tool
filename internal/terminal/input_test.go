package terminal

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"golang.org/x/term"
)

func TestTerminalEditorCompletesAndRecallsHistory(t *testing.T) {

	input := strings.NewReader("cat W\t\r\x1b[A\r")
	output := &bytes.Buffer{}
	editor := term.NewTerminal(terminalReadWriter{reader: newTerminalKeyReader(input), writer: output}, "opsquest$ ")
	editor.AutoCompleteCallback = Completer(historyCompletions{}, nil)

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

func TestTerminalEditorSupportsCommonCursorMotions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "left and right arrows", input: "pd\x1b[D\x1b[C\x1b[Dw\r", want: "pwd"},
		{name: "application home and end", input: "w\x1bOHp\x1bOFd\r", want: "pwd"},
		{name: "alternate home and end", input: "w\x1b[1~p\x1b[4~d\r", want: "pwd"},
		{name: "option word left", input: "cat txt\x1bbWELCOME.\r", want: "cat WELCOME.txt"},
		{name: "control word left", input: "cat txt\x1b[1;5DWELCOME.\r", want: "cat WELCOME.txt"},
		{name: "command line boundaries", input: "w\x1b[1;9Dp\x1b[1;9Cd\r", want: "pwd"},
		{name: "forward delete", input: "pwxd\x1b[H\x1b[C\x1b[C\x1b[3~\r", want: "pwd"},
		{name: "option backspace", input: "cat wrong\x1b\x7fWELCOME.txt\r", want: "cat WELCOME.txt"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := &bytes.Buffer{}
			editor := term.NewTerminal(terminalReadWriter{
				reader: newTerminalKeyReader(strings.NewReader(test.input)),
				writer: output,
			}, "opsquest$ ")
			editor.AutoCompleteCallback = Completer(nil, nil)

			line, err := editor.ReadLine()
			if err != nil {
				t.Fatalf("ReadLine() error = %v", err)
			}
			if line != test.want {
				t.Fatalf("line = %q, want %q", line, test.want)
			}
		})
	}
}

func TestTerminalKeyReaderDoesNotNormalizeBracketedPaste(t *testing.T) {
	input := bracketedPasteStart + "before\x1b[3~after" + bracketedPasteEnd + "\x1b[3~"
	reader := newTerminalKeyReader(strings.NewReader(input))
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	want := bracketedPasteStart + "before\x1b[3~after" + bracketedPasteEnd + string(terminalKeyDeleteForward)
	if string(got) != want {
		t.Fatalf("normalized input = %q, want %q", got, want)
	}
}

func TestForwardDeleteHandlesUnicodeAndLineEnd(t *testing.T) {
	line, position, handled := Completer(nil, nil)("a🙂b", 1, terminalKeyDeleteForward)
	if !handled || line != "ab" || position != 1 {
		t.Fatalf("Unicode delete = %q, %d, %v", line, position, handled)
	}

	line, position, handled = Completer(nil, nil)("pwd", 3, terminalKeyDeleteForward)
	if !handled || line != "pwd" || position != 3 {
		t.Fatalf("end-of-line delete = %q, %d, %v", line, position, handled)
	}
}

func TestNonTerminalReaderKeepsScriptedInputPath(t *testing.T) {
	output := &bytes.Buffer{}
	reader := NewReader(context.Background(), strings.NewReader("pwd\n"), output, nil)
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
	line, position, handled := Completer(nil, nil)("pw", 2, 'd')
	if handled || line != "pw" || position != 2 {
		t.Fatalf("non-Tab completion = %q, %d, %v", line, position, handled)
	}
}

type historyCompletions struct{}

func (historyCompletions) CommandNames() []string { return nil }
func (historyCompletions) PathCandidates(string) []CompletionCandidate {
	return []CompletionCandidate{{Value: "WELCOME.txt"}}
}
