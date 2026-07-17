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

type commandLineReader interface {
	ReadLine(prompt string, box *sandbox.Sandbox) (string, error)
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

type terminalReadWriter struct {
	reader io.Reader
	writer io.Writer
}

func (rw terminalReadWriter) Read(buffer []byte) (int, error) {
	return rw.reader.Read(buffer)
}

func (rw terminalReadWriter) Write(buffer []byte) (int, error) {
	return rw.writer.Write(buffer)
}

type terminalLineReader struct {
	editor   *term.Terminal
	inputFD  int
	outputFD int
}

func newCommandLineReader(in io.Reader, out io.Writer) commandLineReader {
	input, inputIsFile := in.(*os.File)
	output, outputIsFile := out.(*os.File)
	if !inputIsFile || !outputIsFile || !term.IsTerminal(int(input.Fd())) || !term.IsTerminal(int(output.Fd())) {
		return newScannerLineReader(in, out)
	}

	readWriter := terminalReadWriter{reader: input, writer: output}
	return &terminalLineReader{
		editor:   term.NewTerminal(readWriter, ""),
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

func terminalCompleter(box *sandbox.Sandbox) func(string, int, rune) (string, int, bool) {
	return func(line string, position int, key rune) (string, int, bool) {
		if key != '\t' {
			return line, position, false
		}
		return completeLine(line, position, box)
	}
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
	commands := append(sandbox.CommandNames(), "?", ":q", "exit", "hint", "objective", "quit", "restart", "status")
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
