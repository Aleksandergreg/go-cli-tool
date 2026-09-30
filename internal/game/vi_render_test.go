package game

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/sandbox"
)

func TestViRendererSanitizesFileAndStatusControlBytes(t *testing.T) {
	editor, err := newViEditor(sandbox.EditorRequest{
		Path:        "/work/file",
		DisplayPath: "bad\x1bname",
		Content:     "safe\x1b[31m\tend CLIPPED_MARKER",
	})
	if err != nil {
		t.Fatal(err)
	}
	editor.message = ""
	output := &bytes.Buffer{}
	if err := renderViEditor(output, editor, 20, 6); err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	if strings.Contains(rendered, "safe\x1b[31m") || strings.Contains(rendered, "bad\x1bname") {
		t.Fatalf("rendered virtual controls verbatim: %q", rendered)
	}
	if !strings.Contains(rendered, "safe^[[31m") || !strings.Contains(rendered, "bad^[name") {
		t.Fatalf("rendered content was not visibly sanitized: %q", rendered)
	}
	if strings.Contains(rendered, "CLIPPED_MARKER") {
		t.Fatalf("rendered line was not clipped to the viewport: %q", rendered)
	}
}

func TestViRendererScrollsHorizontallyWithTheCursor(t *testing.T) {
	editor := viTestEditor(t, "0123456789abcdefghijklmnopqrstuv")
	editor.column = len(editor.lines[0]) - 1
	editor.preferredCol = editor.column
	output := &bytes.Buffer{}
	if err := renderViEditor(output, editor, 20, 6); err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	if strings.Contains(rendered, "0123456789") || !strings.Contains(rendered, "klmnopqrstuv") {
		t.Fatalf("horizontal viewport did not follow the cursor: %q", rendered)
	}
	if !strings.Contains(rendered, "\x1b[1;20H") {
		t.Fatalf("cursor was not placed at the visible right edge: %q", rendered)
	}
}

func TestDisplayViWindowClipsWithoutLosingSpecialCharacterColumns(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		offset int
		width  int
		want   string
	}{
		{name: "plain", line: "abcdef", offset: 2, width: 3, want: "cde"},
		{name: "inside tab", line: "a\tb", offset: 6, width: 4, want: "  b"},
		{name: "control notation", line: "x\x1by", offset: 1, width: 3, want: "^[y"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := displayViWindow([]rune(test.line), test.offset, test.width); got != test.want {
				t.Fatalf("displayViWindow() = %q, want %q", got, test.want)
			}
		})
	}
}

func BenchmarkRenderViLongLine(b *testing.B) {
	editor, err := newViEditor(sandbox.EditorRequest{
		Path:        "/work/long.txt",
		DisplayPath: "long.txt",
		Content:     strings.Repeat("x", maxViFileBytes),
	})
	if err != nil {
		b.Fatal(err)
	}
	for _, benchmark := range []struct {
		name   string
		column int
	}{
		{name: "cursor-start", column: 0},
		{name: "cursor-end", column: len(editor.lines[0]) - 1},
	} {
		b.Run(benchmark.name, func(b *testing.B) {
			editor.column = benchmark.column
			b.ReportAllocs()
			for range b.N {
				if err := renderViEditor(io.Discard, editor, 80, 24); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
