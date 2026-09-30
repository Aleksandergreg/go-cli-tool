package game

import (
	"bufio"
	"io"
)

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

func (r *terminalKeyReader) buffered() int { return len(r.pending) + r.source.Buffered() }

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

func (rw terminalReadWriter) Read(buffer []byte) (int, error) { return rw.reader.Read(buffer) }

func (rw terminalReadWriter) Write(buffer []byte) (int, error) { return rw.writer.Write(buffer) }
