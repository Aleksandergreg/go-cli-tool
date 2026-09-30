package game

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/aleksandergregersen/opsquest/internal/sandbox"
	"golang.org/x/term"
)

const maxScriptedCommandBytes = 64 * 1024

const (
	bracketedPasteStart = "\x1b[200~"
	bracketedPasteEnd   = "\x1b[201~"
)

// CommandLineReader owns line-editing state across mission sessions. Reusing
// one reader preserves buffered scripted input and interactive command history
// when campaign play advances to the next mission.
type CommandLineReader interface {
	ReadLine(prompt string, completions CompletionSource) (string, error)
	Edit(request sandbox.EditorRequest, save viSaveFunc) error
}

type scannerLineReader struct {
	scanner *bufio.Scanner
	out     io.Writer
}

func newScannerLineReader(in io.Reader, out io.Writer) *scannerLineReader {
	scanner := bufio.NewScanner(in)
	// Scripted commands remain bounded so redirected input cannot grow memory
	// without limit. The interactive editor applies its own smaller bound.
	scanner.Buffer(make([]byte, 1024), maxScriptedCommandBytes)
	return &scannerLineReader{scanner: scanner, out: out}
}

func (r *scannerLineReader) ReadLine(prompt string, _ CompletionSource) (string, error) {
	fmt.Fprint(r.out, prompt)
	if r.scanner.Scan() {
		return r.scanner.Text(), nil
	}
	if err := r.scanner.Err(); err != nil {
		return "", err
	}
	return "", io.EOF
}

func (r *scannerLineReader) Edit(_ sandbox.EditorRequest, _ viSaveFunc) error {
	return ErrInteractiveEditor
}

type terminalLineReader struct {
	editor   *term.Terminal
	keys     *terminalKeyReader
	out      io.Writer
	inputFD  int
	outputFD int
}

func NewCommandLineReader(in io.Reader, out io.Writer) CommandLineReader {
	input, inputIsFile := in.(*os.File)
	output, outputIsFile := out.(*os.File)
	if !inputIsFile || !outputIsFile || !term.IsTerminal(int(input.Fd())) || !term.IsTerminal(int(output.Fd())) {
		return newScannerLineReader(in, out)
	}

	keys := newTerminalKeyReader(input)
	readWriter := terminalReadWriter{reader: keys, writer: output}
	return &terminalLineReader{
		editor:   term.NewTerminal(readWriter, ""),
		keys:     keys,
		out:      output,
		inputFD:  int(input.Fd()),
		outputFD: int(output.Fd()),
	}
}

func (r *terminalLineReader) ReadLine(prompt string, completions CompletionSource) (string, error) {
	r.editor.SetPrompt(prompt)
	r.editor.AutoCompleteCallback = terminalCompleter(completions)
	if width, height, err := term.GetSize(r.outputFD); err == nil {
		_ = r.editor.SetSize(width, height)
	}

	previousState, err := term.MakeRaw(r.inputFD)
	if err != nil {
		return "", fmt.Errorf("enable terminal line editing: %w", err)
	}

	r.editor.SetBracketedPasteMode(true)
	line, readErr := r.editor.ReadLine()
	r.editor.SetBracketedPasteMode(false)
	restoreErr := term.Restore(r.inputFD, previousState)

	if errors.Is(readErr, term.ErrPasteIndicator) {
		readErr = nil
	}
	if readErr != nil && restoreErr != nil {
		return line, errors.Join(readErr, fmt.Errorf("restore terminal: %w", restoreErr))
	}
	if readErr != nil {
		return line, readErr
	}
	if restoreErr != nil {
		return line, fmt.Errorf("restore terminal: %w", restoreErr)
	}
	return line, nil
}

func (r *terminalLineReader) Edit(request sandbox.EditorRequest, save viSaveFunc) (returnErr error) {
	previousState, err := term.MakeRaw(r.inputFD)
	if err != nil {
		return fmt.Errorf("enable vi terminal: %w", err)
	}

	enteredAlternateScreen := false
	r.keys.modalEscape = true
	defer func() {
		r.keys.modalEscape = false
		r.keys.pasteActive = false
		if enteredAlternateScreen {
			if _, err := io.WriteString(r.out, "\x1b[?2004l\x1b[?25h\x1b[?1049l"); err != nil {
				returnErr = errors.Join(returnErr, fmt.Errorf("restore vi screen: %w", err))
			}
		}
		if err := term.Restore(r.inputFD, previousState); err != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("restore terminal after vi: %w", err))
		}
	}()

	enteredAlternateScreen = true
	if _, err := io.WriteString(r.out, "\x1b[?1049h\x1b[?2004h\x1b[?25l"); err != nil {
		return fmt.Errorf("open vi screen: %w", err)
	}

	size := func() (int, int) {
		width, height, err := term.GetSize(r.outputFD)
		if err != nil || width < 20 || height < 6 {
			return 80, 24
		}
		return width, height
	}
	return runViEditor(r.keys, r.out, request, save, size)
}
