package game

import (
	"io"
	"unicode/utf8"
)

type viKeyKind uint8

const (
	viRuneKey viKeyKind = iota
	viEscapeKey
	viEnterKey
	viBackspaceKey
	viDeleteKey
	viUpKey
	viDownKey
	viLeftKey
	viRightKey
	viHomeKey
	viEndKey
	viInterruptKey
	viPasteStartKey
	viPasteEndKey
	viUnknownKey
)

type viKey struct {
	kind viKeyKind
	rune rune
	raw  string
}

func readViPasteKey(input *terminalKeyReader) (viKey, error) {
	first, err := readViByte(input)
	if err != nil {
		return viKey{}, err
	}
	// terminalKeyReader toggles pasteActive only after reading the complete end
	// marker, which lets this literal reader distinguish it from an Escape byte
	// contained in pasted text.
	if first == '\x1b' && !input.pasteActive {
		marker := make([]byte, len(bracketedPasteEnd))
		marker[0] = first
		for index := 1; index < len(marker); index++ {
			marker[index], err = readViByte(input)
			if err != nil {
				return viKey{}, err
			}
		}
		if string(marker) == bracketedPasteEnd {
			return viKey{kind: viPasteEndKey}, nil
		}
		return viKey{kind: viUnknownKey, raw: string(marker)}, nil
	}
	if first == '\r' || first == '\n' {
		return viKey{kind: viEnterKey}, nil
	}
	if first < utf8.RuneSelf {
		return viKey{kind: viRuneKey, rune: rune(first)}, nil
	}
	return readViUTF8Key(input, first)
}

func readViKey(input *terminalKeyReader) (viKey, error) {
	first, err := readViByte(input)
	if err != nil {
		return viKey{}, err
	}
	switch first {
	case '\x1b':
		if input.buffered() == 0 {
			return viKey{kind: viEscapeKey}, nil
		}
		second, err := readViByte(input)
		if err != nil {
			return viKey{kind: viEscapeKey}, nil
		}
		if second != '[' && second != 'O' {
			input.unread([]byte{second})
			return viKey{kind: viEscapeKey}, nil
		}
		sequence := []byte{'\x1b', second}
		for len(sequence) < 32 {
			next, err := readViByte(input)
			if err != nil {
				return viKey{kind: viUnknownKey, raw: string(sequence)}, nil
			}
			sequence = append(sequence, next)
			if next >= 0x40 && next <= 0x7e {
				break
			}
		}
		return decodeViSequence(string(sequence)), nil
	case '\r', '\n':
		return viKey{kind: viEnterKey}, nil
	case '\x7f', '\b':
		return viKey{kind: viBackspaceKey}, nil
	case '\x03':
		return viKey{kind: viInterruptKey}, nil
	}

	if first < utf8.RuneSelf {
		return viKey{kind: viRuneKey, rune: rune(first)}, nil
	}
	return readViUTF8Key(input, first)
}

func readViUTF8Key(input *terminalKeyReader, first byte) (viKey, error) {
	width := utf8SequenceLength(first)
	if width == 0 {
		return viKey{kind: viRuneKey, rune: utf8.RuneError}, nil
	}
	encoded := make([]byte, width)
	encoded[0] = first
	for index := 1; index < width; index++ {
		next, err := readViByte(input)
		if err != nil {
			return viKey{}, err
		}
		encoded[index] = next
	}
	decoded, decodedWidth := utf8.DecodeRune(encoded)
	if decoded == utf8.RuneError && decodedWidth == 1 {
		return viKey{kind: viRuneKey, rune: utf8.RuneError}, nil
	}
	if decoded == terminalKeyDeleteForward {
		return viKey{kind: viDeleteKey}, nil
	}
	return viKey{kind: viRuneKey, rune: decoded}, nil
}

func readViByte(input io.Reader) (byte, error) {
	var value [1]byte
	_, err := io.ReadFull(input, value[:])
	return value[0], err
}

func utf8SequenceLength(first byte) int {
	switch {
	case first&0xe0 == 0xc0:
		return 2
	case first&0xf0 == 0xe0:
		return 3
	case first&0xf8 == 0xf0:
		return 4
	default:
		return 0
	}
}

var viSequenceKinds = map[string]viKeyKind{
	bracketedPasteStart: viPasteStartKey,
	bracketedPasteEnd:   viPasteEndKey,
	"\x1b[A":            viUpKey,
	"\x1bOA":            viUpKey,
	"\x1b[B":            viDownKey,
	"\x1bOB":            viDownKey,
	"\x1b[C":            viRightKey,
	"\x1bOC":            viRightKey,
	"\x1b[D":            viLeftKey,
	"\x1bOD":            viLeftKey,
	"\x1b[H":            viHomeKey,
	"\x1bOH":            viHomeKey,
	"\x1b[F":            viEndKey,
	"\x1bOF":            viEndKey,
	"\x1b[3~":           viDeleteKey,
}

func decodeViSequence(sequence string) viKey {
	if kind, found := viSequenceKinds[sequence]; found {
		return viKey{kind: kind}
	}
	return viKey{kind: viUnknownKey, raw: sequence}
}
