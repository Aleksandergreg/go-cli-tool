package sandbox

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// commandOutputBuffer bounds incrementally produced output before a command
// can allocate an arbitrarily large result. The dispatcher retains a final
// size check for handlers whose result is assembled without a builder.
type commandOutputBuffer struct {
	builder strings.Builder
	err     error
}

func (b *commandOutputBuffer) WriteString(value string) (int, error) {
	if err := b.reserve(len(value)); err != nil {
		return 0, err
	}
	return b.builder.WriteString(value)
}

func (b *commandOutputBuffer) WriteByte(value byte) error {
	if err := b.reserve(1); err != nil {
		return err
	}
	return b.builder.WriteByte(value)
}

func (b *commandOutputBuffer) WriteRune(value rune) (int, error) {
	size := utf8.RuneLen(value)
	if size < 0 {
		size = utf8.RuneLen(utf8.RuneError)
	}
	if err := b.reserve(size); err != nil {
		return 0, err
	}
	return b.builder.WriteRune(value)
}

func (b *commandOutputBuffer) reserve(size int) error {
	if b.err == nil && size > maxCommandOutputBytes-b.builder.Len() {
		b.err = commandOutputLimitError()
	}
	return b.err
}

func (b *commandOutputBuffer) Result() (string, error) {
	if b.err != nil {
		return "", b.err
	}
	return b.builder.String(), nil
}

func (b *commandOutputBuffer) Err() error {
	return b.err
}

func commandOutputLimitError() error {
	return fmt.Errorf("output exceeds the %d KiB command limit", maxCommandOutputBytes/1024)
}
