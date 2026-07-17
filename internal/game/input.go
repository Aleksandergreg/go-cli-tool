package game

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/term"

	"github.com/aleksandergregersen/opsquest/internal/sandbox"
)

const maxScriptedCommandBytes = 64 * 1024

const terminalKeyDeleteForward rune = '\ue000'

var terminalSequenceReplacements = map[string]string{
	"\x1bOH":     "\x1b[H",    // application-mode Home
	"\x1bOF":     "\x1b[F",    // application-mode End
	"\x1b[1~":    "\x1b[H",    // alternate Home
	"\x1b[4~":    "\x1b[F",    // alternate End
	"\x1b[7~":    "\x1b[H",    // rxvt Home
	"\x1b[8~":    "\x1b[F",    // rxvt End
	"\x1bb":      "\x1b[1;3D", // Meta/Option-B
	"\x1bf":      "\x1b[1;3C", // Meta/Option-F
	"\x1b[1;5D":  "\x1b[1;3D", // Ctrl-Left
	"\x1b[1;5C":  "\x1b[1;3C", // Ctrl-Right
	"\x1b[1;9D":  "\x1b[H",    // Command/Meta-Left
	"\x1b[1;9C":  "\x1b[F",    // Command/Meta-Right
	"\x1b[1;10D": "\x1b[H",    // Shift-Command/Meta-Left
	"\x1b[1;10C": "\x1b[F",    // Shift-Command/Meta-Right
	"\x1b[3~":    string(terminalKeyDeleteForward),
	"\x1b\x7f":   "\x17", // Option-Backspace to Ctrl-W
	"\x1b\x08":   "\x17", // alternate Option-Backspace
}

const (
	bracketedPasteStart = "\x1b[200~"
	bracketedPasteEnd   = "\x1b[201~"
)

// CommandLineReader owns line-editing state across mission sessions. Reusing
// one reader preserves buffered scripted input and interactive command history
// when campaign play advances to the next mission.
type CommandLineReader interface {
	ReadLine(prompt string, box *sandbox.Sandbox) (string, error)
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

func (r *scannerLineReader) ReadLine(prompt string, _ *sandbox.Sandbox) (string, error) {
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

type terminalReadWriter struct {
	reader io.Reader
	writer io.Writer
}

// terminalKeyReader normalizes common terminal-specific key encodings into
// the VT100/readline subset understood by x/term. It never interprets bytes
// inside bracketed paste markers.
type terminalKeyReader struct {
	source      *bufio.Reader
	pending     []byte
	deferredErr error
	pasteActive bool
	modalEscape bool
}

func newTerminalKeyReader(reader io.Reader) *terminalKeyReader {
	return &terminalKeyReader{source: bufio.NewReader(reader)}
}

func (r *terminalKeyReader) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	if len(r.pending) == 0 {
		if r.deferredErr != nil {
			err := r.deferredErr
			r.deferredErr = nil
			return 0, err
		}
		sequence, err := r.readSequence()
		if len(sequence) == 0 {
			return 0, err
		}
		if err != nil {
			r.deferredErr = err
		}
		r.pending = r.normalize(sequence)
	}

	count := copy(buffer, r.pending)
	r.pending = r.pending[count:]
	return count, nil
}

func (r *terminalKeyReader) readSequence() ([]byte, error) {
	first, err := r.source.ReadByte()
	if err != nil {
		return nil, err
	}
	sequence := []byte{first}
	if first != '\x1b' {
		return sequence, nil
	}
	// A standalone Escape key is meaningful to modal editors. ReadByte fills
	// the source buffer with bytes already delivered by the terminal, so a
	// complete arrow/CSI sequence remains available here while a lone Escape
	// can return immediately instead of waiting for the next keypress.
	if r.modalEscape && r.source.Buffered() == 0 {
		return sequence, nil
	}

	second, err := r.source.ReadByte()
	if err != nil {
		return sequence, err
	}
	sequence = append(sequence, second)
	if second != '[' && second != 'O' {
		return sequence, nil
	}

	for len(sequence) < 32 {
		next, readErr := r.source.ReadByte()
		if readErr != nil {
			return sequence, readErr
		}
		sequence = append(sequence, next)
		if next >= 0x40 && next <= 0x7e {
			break
		}
	}
	return sequence, nil
}

func (r *terminalKeyReader) buffered() int {
	return len(r.pending) + r.source.Buffered()
}

func (r *terminalKeyReader) unread(data []byte) {
	pending := make([]byte, 0, len(data)+len(r.pending))
	pending = append(pending, data...)
	pending = append(pending, r.pending...)
	r.pending = pending
}

func (r *terminalKeyReader) normalize(sequence []byte) []byte {
	value := string(sequence)
	if value == bracketedPasteStart {
		r.pasteActive = true
		return sequence
	}
	if value == bracketedPasteEnd && r.pasteActive {
		r.pasteActive = false
		return sequence
	}
	if r.pasteActive {
		return sequence
	}
	if replacement, exists := terminalSequenceReplacements[value]; exists {
		return []byte(replacement)
	}
	return sequence
}

func (rw terminalReadWriter) Read(buffer []byte) (int, error) {
	return rw.reader.Read(buffer)
}

func (rw terminalReadWriter) Write(buffer []byte) (int, error) {
	return rw.writer.Write(buffer)
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

func (r *terminalLineReader) ReadLine(prompt string, box *sandbox.Sandbox) (string, error) {
	r.editor.SetPrompt(prompt)
	r.editor.AutoCompleteCallback = terminalCompleter(box)
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

func terminalCompleter(box *sandbox.Sandbox) func(string, int, rune) (string, int, bool) {
	return func(line string, position int, key rune) (string, int, bool) {
		switch key {
		case terminalKeyDeleteForward:
			return deleteRuneAtCursor(line, position)
		case '\t':
			return completeLine(line, position, box)
		default:
			return line, position, false
		}
	}
}

func deleteRuneAtCursor(line string, position int) (string, int, bool) {
	if position < 0 || position > len(line) || !utf8.ValidString(line) || position < len(line) && !utf8.RuneStart(line[position]) {
		return line, position, true
	}
	if position == len(line) {
		return line, position, true
	}
	_, size := utf8.DecodeRuneInString(line[position:])
	return line[:position] + line[position+size:], position, true
}

type completionCandidate struct {
	value     string
	directory bool
}

func completeLine(line string, position int, box *sandbox.Sandbox) (string, int, bool) {
	if position < 0 || position > len(line) || !utf8.ValidString(line) || position < len(line) && !utf8.RuneStart(line[position]) {
		return line, position, true
	}

	start, end := completionTokenRange(line, position)
	typed, quote, valid := completionToken(line[start:position])
	if !valid {
		return line, position, true
	}

	var candidates []completionCandidate
	if commandPosition(line[:start]) {
		candidates = commandCandidates(typed)
	} else {
		candidates = pathCandidates(box, typed)
	}
	if len(candidates) == 0 {
		return line, position, true
	}

	replacement := ""
	if len(candidates) == 1 {
		candidate := candidates[0]
		replacement = formatCompletion(candidate.value, quote, !candidate.directory)
		if !candidate.directory && completionNeedsSpace(line[end:]) {
			replacement += " "
		}
	} else {
		common := longestCommonPrefix(candidates)
		if common == typed {
			return line, position, true
		}
		replacement = formatCompletion(common, quote, false)
	}

	completed := line[:start] + replacement + line[end:]
	return completed, start + len(replacement), true
}

func completionTokenRange(line string, position int) (int, int) {
	start := 0
	var quote byte
	escaped := false
	for index := 0; index < position; index++ {
		char := line[index]
		if escaped {
			escaped = false
			continue
		}
		if char == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			continue
		}
		if completionBoundary(char) {
			start = index + 1
		}
	}

	end := position
	for end < len(line) {
		char := line[end]
		if escaped {
			escaped = false
			end++
			continue
		}
		if char == '\\' && quote != '\'' {
			escaped = true
			end++
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			}
			end++
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			end++
			continue
		}
		if completionBoundary(char) {
			break
		}
		end++
	}
	return start, end
}

func completionBoundary(char byte) bool {
	return char == ' ' || char == '\t' || char == '\r' || char == '\n' || char == '|' || char == '<' || char == '>'
}

func completionToken(raw string) (string, byte, bool) {
	var quote byte
	if raw != "" && (raw[0] == '\'' || raw[0] == '"') {
		quote = raw[0]
		raw = raw[1:]
		if strings.ContainsRune(raw, rune(quote)) {
			return "", 0, false
		}
	}
	if quote == '\'' {
		return raw, quote, true
	}

	var value strings.Builder
	escaped := false
	for _, char := range raw {
		if escaped {
			value.WriteRune(char)
			escaped = false
			continue
		}
		if char == '\\' {
			escaped = true
			continue
		}
		value.WriteRune(char)
	}
	if escaped {
		return "", 0, false
	}
	return value.String(), quote, true
}

func commandPosition(beforeToken string) bool {
	trimmed := strings.TrimSpace(beforeToken)
	return trimmed == "" || strings.HasSuffix(trimmed, "|")
}

func commandCandidates(prefix string) []completionCandidate {
	commands := append(sandbox.CommandNames(), "?", ":q", "exit", "hint", "list", "missions", "next", "objective", "opsquest", "play", "prev", "previous", "quit", "restart", "status")
	sort.Strings(commands)
	candidates := make([]completionCandidate, 0)
	last := ""
	for _, command := range commands {
		if command == last || !strings.HasPrefix(command, prefix) {
			continue
		}
		candidates = append(candidates, completionCandidate{value: command})
		last = command
	}
	return candidates
}

func pathCandidates(box *sandbox.Sandbox, prefix string) []completionCandidate {
	if prefix == "~" && box.FS.IsDir(box.Resolve("~")) {
		return []completionCandidate{{value: "~/", directory: true}}
	}

	directoryPart, namePrefix := path.Split(prefix)
	lookupDirectory := directoryPart
	if lookupDirectory == "" {
		lookupDirectory = "."
	}
	children, err := box.FS.Children(box.Resolve(lookupDirectory))
	if err != nil {
		return nil
	}

	candidates := make([]completionCandidate, 0)
	for _, child := range children {
		name := path.Base(child)
		if !strings.HasPrefix(name, namePrefix) {
			continue
		}
		directory := box.FS.IsDir(child)
		value := directoryPart + name
		if directory {
			value += "/"
		}
		candidates = append(candidates, completionCandidate{value: value, directory: directory})
	}
	return candidates
}

func longestCommonPrefix(candidates []completionCandidate) string {
	prefix := []rune(candidates[0].value)
	for _, candidate := range candidates[1:] {
		value := []rune(candidate.value)
		limit := len(prefix)
		if len(value) < limit {
			limit = len(value)
		}
		index := 0
		for index < limit && prefix[index] == value[index] {
			index++
		}
		prefix = prefix[:index]
	}
	return string(prefix)
}

func formatCompletion(value string, quote byte, closeQuote bool) string {
	if quote == '\'' {
		result := "'" + strings.ReplaceAll(value, "'", `'\''`)
		if closeQuote {
			result += "'"
		}
		return result
	}
	if quote == '"' {
		replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`)
		result := `"` + replacer.Replace(value)
		if closeQuote {
			result += `"`
		}
		return result
	}

	var escaped strings.Builder
	for _, char := range value {
		if unicode.IsSpace(char) || strings.ContainsRune("\\'\"|<>#$*?[", char) {
			escaped.WriteRune('\\')
		}
		escaped.WriteRune(char)
	}
	return escaped.String()
}

func completionNeedsSpace(afterToken string) bool {
	return afterToken == "" || afterToken[0] == '|' || afterToken[0] == '<' || afterToken[0] == '>'
}
