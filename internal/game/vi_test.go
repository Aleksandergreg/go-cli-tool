package game

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
	"github.com/aleksandergregersen/opsquest/internal/sandbox"
)

func viTestEditor(t *testing.T, content string) *viEditor {
	t.Helper()
	editor, err := newViEditor(sandbox.EditorRequest{
		Command:     "vi",
		Path:        "/work/note.txt",
		DisplayPath: "note.txt",
		Content:     content,
	})
	if err != nil {
		t.Fatal(err)
	}
	return editor
}

func viRune(char rune) viKey {
	return viKey{kind: viRuneKey, rune: char}
}

func enterViCommand(editor *viEditor, command string, save viSaveFunc) {
	editor.handle(viRune(':'), save)
	for _, char := range command {
		editor.handle(viRune(char), save)
	}
	editor.handle(viKey{kind: viEnterKey}, save)
}

func TestViInitialContentRoundTripsExactly(t *testing.T) {
	for _, content := range []string{"", "one line", "one line\n", "one\n\ntwo\n", "hej 🙂\n"} {
		editor := viTestEditor(t, content)
		if got := editor.content(); got != content {
			t.Errorf("content round trip = %q, want %q", got, content)
		}
	}
}

func TestViNormalMotionsKeepPreferredColumn(t *testing.T) {
	editor := viTestEditor(t, "abcd\nx\nwxyz")
	editor.handle(viRune('h'), nil)
	if editor.column != 0 {
		t.Fatalf("h crossed line start: column = %d", editor.column)
	}
	for range 3 {
		editor.handle(viRune('l'), nil)
	}
	editor.handle(viRune('j'), nil)
	if editor.row != 1 || editor.column != 0 || editor.preferredCol != 3 {
		t.Fatalf("short-line motion = row %d col %d preferred %d", editor.row, editor.column, editor.preferredCol)
	}
	editor.handle(viKey{kind: viDownKey}, nil)
	if editor.row != 2 || editor.column != 3 {
		t.Fatalf("preferred column was not restored: row %d col %d", editor.row, editor.column)
	}
	editor.handle(viKey{kind: viUpKey}, nil)
	editor.handle(viRune('k'), nil)
	if editor.row != 0 || editor.column != 3 {
		t.Fatalf("up motion = row %d col %d", editor.row, editor.column)
	}
	editor.handle(viKey{kind: viHomeKey}, nil)
	if editor.column != 0 {
		t.Errorf("Home column = %d", editor.column)
	}
	editor.handle(viKey{kind: viEndKey}, nil)
	if editor.column != 3 {
		t.Errorf("End column = %d", editor.column)
	}
	editor.handle(viRune('l'), nil)
	editor.handle(viRune('j'), nil)
	editor.handle(viRune('j'), nil)
	editor.handle(viRune('j'), nil)
	if editor.row != 2 || editor.column != 3 {
		t.Errorf("motion crossed bottom/right boundary: row %d col %d", editor.row, editor.column)
	}
}

func TestViInsertModeHandlesUnicodeNewlinesBackspaceAndEscape(t *testing.T) {
	editor := viTestEditor(t, "cat")
	editor.handle(viRune('i'), nil)
	editor.handle(viRune('é'), nil)
	editor.handle(viKey{kind: viEnterKey}, nil)
	editor.handle(viRune('🙂'), nil)
	if got := editor.content(); got != "é\n🙂cat" {
		t.Fatalf("inserted content = %q", got)
	}
	editor.handle(viKey{kind: viBackspaceKey}, nil)
	editor.handle(viKey{kind: viBackspaceKey}, nil)
	if got := editor.content(); got != "écat" {
		t.Fatalf("backspaced content = %q", got)
	}
	editor.handle(viKey{kind: viEscapeKey}, nil)
	if editor.mode != viNormalMode || editor.column != 0 || !editor.dirty {
		t.Fatalf("after Escape: mode %d col %d dirty %v", editor.mode, editor.column, editor.dirty)
	}
}

func TestViXAndDDDeleteUnicodeAndLines(t *testing.T) {
	editor := viTestEditor(t, "🙂a\ntwo\nthree")
	editor.handle(viRune('x'), nil)
	if got := editor.content(); got != "a\ntwo\nthree" || editor.byteSize != len(got) {
		t.Fatalf("x content = %q, byte size = %d", got, editor.byteSize)
	}
	editor.handle(viRune('j'), nil)
	editor.handle(viRune('d'), nil)
	if got := editor.content(); got != "a\ntwo\nthree" {
		t.Fatalf("one d changed content = %q", got)
	}
	editor.handle(viRune('d'), nil)
	if got := editor.content(); got != "a\nthree" || editor.row != 1 {
		t.Fatalf("middle dd = %q at row %d", got, editor.row)
	}
	editor.handle(viRune('d'), nil)
	editor.handle(viRune('d'), nil)
	if got := editor.content(); got != "a" || editor.row != 0 {
		t.Fatalf("last dd = %q at row %d", got, editor.row)
	}
	editor.handle(viRune('d'), nil)
	editor.handle(viRune('d'), nil)
	if got := editor.content(); got != "" || len(editor.lines) != 1 {
		t.Fatalf("only-line dd = %q with %d lines", got, len(editor.lines))
	}
}

func TestViDDDeletesTheFirstLine(t *testing.T) {
	editor := viTestEditor(t, "first\nsecond")
	editor.handle(viRune('d'), nil)
	editor.handle(viRune('d'), nil)
	if got := editor.content(); got != "second" || editor.row != 0 || editor.column != 0 {
		t.Fatalf("first-line dd = %q at %d,%d", got, editor.row, editor.column)
	}
}

func TestViWriteQuitAndDiscardCommands(t *testing.T) {
	editor := viTestEditor(t, "old")
	editor.handle(viRune('x'), nil)
	saves := 0
	var savedPath, savedContent string
	save := func(path, content string) error {
		saves++
		savedPath, savedContent = path, content
		return nil
	}
	enterViCommand(editor, "w", save)
	if editor.quit || editor.dirty || saves != 1 || savedPath != "/work/note.txt" || savedContent != "ld" {
		t.Fatalf(":w state = quit %v dirty %v saves %d path %q content %q", editor.quit, editor.dirty, saves, savedPath, savedContent)
	}
	enterViCommand(editor, "q", save)
	if !editor.quit {
		t.Fatal(":q did not quit a clean buffer")
	}

	dirty := viTestEditor(t, "old")
	dirty.handle(viRune('x'), nil)
	enterViCommand(dirty, "q", save)
	if dirty.quit || !strings.Contains(dirty.message, "No write since last change") {
		t.Fatalf("dirty :q = quit %v message %q", dirty.quit, dirty.message)
	}
	enterViCommand(dirty, "q!", save)
	if !dirty.quit || saves != 1 {
		t.Fatalf(":q! = quit %v saves %d", dirty.quit, saves)
	}

	writeQuit := viTestEditor(t, "old")
	writeQuit.handle(viRune('x'), nil)
	enterViCommand(writeQuit, "wq", save)
	if !writeQuit.quit || writeQuit.dirty || saves != 2 {
		t.Fatalf(":wq = quit %v dirty %v saves %d", writeQuit.quit, writeQuit.dirty, saves)
	}
}

func TestViReportsUnsupportedCommandsAndWriteFailures(t *testing.T) {
	editor := viTestEditor(t, "text")
	enterViCommand(editor, "set number", func(string, string) error { return nil })
	if !strings.Contains(editor.message, "Not an editor command") || editor.quit {
		t.Fatalf("unsupported command message = %q", editor.message)
	}
	editor.handle(viRune('x'), nil)
	enterViCommand(editor, "wq", func(string, string) error { return errors.New("virtual disk full") })
	if editor.quit || !editor.dirty || !strings.Contains(editor.message, "virtual disk full") {
		t.Fatalf("failed write = quit %v dirty %v message %q", editor.quit, editor.dirty, editor.message)
	}
}

func TestViRejectsOversizeAndNonUTF8Content(t *testing.T) {
	request := sandbox.EditorRequest{DisplayPath: "large.txt", Content: strings.Repeat("x", maxViFileBytes+1)}
	if _, err := newViEditor(request); err == nil || !errors.Is(err, ErrUnsupportedEditorFile) || !strings.Contains(err.Error(), "teaching-editor limit") {
		t.Fatalf("oversize error = %v", err)
	}
	request = sandbox.EditorRequest{DisplayPath: "binary.dat", Content: string([]byte{0xff})}
	if _, err := newViEditor(request); err == nil || !errors.Is(err, ErrUnsupportedEditorFile) || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("UTF-8 error = %v", err)
	}

	atLimit := viTestEditor(t, strings.Repeat("x", maxViFileBytes))
	atLimit.handle(viRune('i'), nil)
	atLimit.handle(viRune('y'), nil)
	if atLimit.byteSize != maxViFileBytes || !strings.Contains(atLimit.message, "editor limit") {
		t.Fatalf("at-limit insertion = %d bytes, message %q", atLimit.byteSize, atLimit.message)
	}
}

func TestViKeyReaderHandlesEscapeSequencesAndUnicode(t *testing.T) {
	input := newTerminalKeyReader(strings.NewReader("\x1b:wq\r\x1b[A\x1b[B\x1b[C\x1b[D\x1b[H\x1b[F\x1b[3~\x1b[999~🙂"))
	wants := []viKey{
		{kind: viEscapeKey},
		{kind: viRuneKey, rune: ':'},
		{kind: viRuneKey, rune: 'w'},
		{kind: viRuneKey, rune: 'q'},
		{kind: viEnterKey},
		{kind: viUpKey},
		{kind: viDownKey},
		{kind: viRightKey},
		{kind: viLeftKey},
		{kind: viHomeKey},
		{kind: viEndKey},
		{kind: viDeleteKey},
		{kind: viUnknownKey},
		{kind: viRuneKey, rune: '🙂'},
	}
	for index, want := range wants {
		got, err := readViKey(input)
		if err != nil {
			t.Fatalf("key %d error = %v", index, err)
		}
		if got.kind != want.kind || got.rune != want.rune {
			t.Fatalf("key %d = %#v, want %#v", index, got, want)
		}
	}

	standalone := newTerminalKeyReader(strings.NewReader("\x1b"))
	key, err := readViKey(standalone)
	if err != nil || key.kind != viEscapeKey {
		t.Fatalf("standalone Escape = %#v, %v", key, err)
	}
}

func TestRunViEditorSavesACompleteModalSequence(t *testing.T) {
	input := newTerminalKeyReader(strings.NewReader("ihello\x1b:wq\r"))
	output := &bytes.Buffer{}
	var saved string
	err := runViEditor(input, output, sandbox.EditorRequest{
		Command: "vi", Path: "/work/new.txt", DisplayPath: "new.txt",
	}, func(_ string, content string) error {
		saved = content
		return nil
	}, func() (int, int) { return 40, 8 })
	if err != nil {
		t.Fatal(err)
	}
	if saved != "hello" {
		t.Fatalf("saved content = %q", saved)
	}
	if !strings.Contains(output.String(), "INSERT") || !strings.Contains(output.String(), ":wq") {
		t.Fatalf("rendered output did not expose editor modes: %q", output.String())
	}
}

func TestRunViEditorTreatsBracketedPasteAsLiteralInsertText(t *testing.T) {
	input := newTerminalKeyReader(strings.NewReader("i" + bracketedPasteStart + "dd\n:q!" + bracketedPasteEnd + "\x1b:wq\r"))
	output := &bytes.Buffer{}
	var saved string
	err := runViEditor(input, output, sandbox.EditorRequest{
		Command: "vi", Path: "/work/new.txt", DisplayPath: "new.txt",
	}, func(_ string, content string) error {
		saved = content
		return nil
	}, func() (int, int) { return 40, 8 })
	if err != nil {
		t.Fatal(err)
	}
	if saved != "dd\n:q!" {
		t.Fatalf("pasted content = %q", saved)
	}
}

func TestRunViEditorIgnoresBracketedPasteInNormalMode(t *testing.T) {
	input := newTerminalKeyReader(strings.NewReader(bracketedPasteStart + "dd:q!" + bracketedPasteEnd + ":q\r"))
	output := &bytes.Buffer{}
	request := sandbox.EditorRequest{Command: "vi", Path: "/work/note", DisplayPath: "note", Content: "keep"}
	if err := runViEditor(input, output, request, func(string, string) error {
		t.Fatal("normal-mode paste unexpectedly wrote the file")
		return nil
	}, func() (int, int) { return 40, 8 }); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Paste ignored in Normal mode") {
		t.Fatalf("missing normal-mode paste guidance: %q", output.String())
	}
}

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

func TestNonTerminalViRefusalDoesNotConsumeTheNextCommand(t *testing.T) {
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, _ := catalog.Find("linux-config-surgery")
	player := profile.New("tester")
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	session := Session{
		Mission: item, Player: &player, Saver: profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "tester"),
		In: strings.NewReader("vi app.env\nquit\n"), Out: out, ErrOut: errOut, Catalog: catalog,
	}
	result, err := session.Run()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Quit || player.Commands["vi"] != 0 {
		t.Fatalf("result = %#v, vi mastery = %d", result, player.Commands["vi"])
	}
	if !strings.Contains(errOut.String(), "vi: interactive editor requires a terminal") {
		t.Fatalf("stderr = %q", errOut.String())
	}
	if !strings.Contains(out.String(), "Mission paused") {
		t.Fatalf("following quit command was not consumed: %q", out.String())
	}
}

type fakeViReader struct {
	lines []string
	index int
	edit  func(sandbox.EditorRequest, viSaveFunc) error
}

func (r *fakeViReader) ReadLine(_ string, _ CompletionSource) (string, error) {
	if r.index >= len(r.lines) {
		return "", io.EOF
	}
	line := r.lines[r.index]
	r.index++
	return line, nil
}

func (r *fakeViReader) Edit(request sandbox.EditorRequest, save viSaveFunc) error {
	return r.edit(request, save)
}

func TestViSessionCompletesOutcomeAndRecordsMastery(t *testing.T) {
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, _ := catalog.Find("linux-config-surgery")
	player := profile.New("tester")
	store := profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "tester")
	reader := &fakeViReader{
		lines: []string{"vi app.env"},
		edit: func(request sandbox.EditorRequest, save viSaveFunc) error {
			if request.Path != "/etc/byteworks/app.env" || !strings.Contains(request.Content, "LOG_LEVEL=debug") {
				return fmt.Errorf("unexpected editor request: %#v", request)
			}
			return save(request.Path, strings.Replace(request.Content, "LOG_LEVEL=debug", "LOG_LEVEL=info", 1))
		},
	}
	out := &bytes.Buffer{}
	session := Session{
		Mission: item, Player: &player, Saver: store,
		Out: out, ErrOut: &bytes.Buffer{}, Reader: reader, Catalog: catalog,
		Now: func() time.Time { return time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC) },
	}
	result, err := session.Run()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Completed || player.Commands["vi"] != 1 || !player.IsComplete(item.ID) {
		t.Fatalf("result = %#v, vi mastery = %d, completed = %v", result, player.Commands["vi"], player.IsComplete(item.ID))
	}
	if !strings.Contains(out.String(), "Mission complete") || !strings.Contains(out.String(), "New command discovered: vi") {
		t.Fatalf("session output = %q", out.String())
	}
}

func TestModalFirstAidAcceptsViAndRejectsDiscardedChanges(t *testing.T) {
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, found := catalog.Find("linux-vi-first-aid")
	if !found {
		t.Fatal("Modal First Aid mission missing")
	}

	for _, test := range []struct {
		name     string
		keys     string
		complete bool
	}{
		{name: "dd and write-quit", keys: "dd:wq\r", complete: true},
		{name: "dd and discard", keys: "dd:q!\r", complete: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			box, err := sandbox.New(item.Setup, item.StartDir)
			if err != nil {
				t.Fatal(err)
			}
			result, err := box.Execute("vi release.env")
			if err != nil {
				t.Fatal(err)
			}
			if result.Editor == nil {
				t.Fatal("vi did not return an editor request")
			}
			if err := runViEditor(
				newTerminalKeyReader(strings.NewReader(test.keys)),
				io.Discard,
				*result.Editor,
				box.SaveEditorFile,
				func() (int, int) { return 80, 12 },
			); err != nil {
				t.Fatal(err)
			}
			complete, err := Validate(item.Validation, box, "")
			if err != nil {
				t.Fatal(err)
			}
			if complete != test.complete {
				t.Fatalf("mission complete = %v, want %v", complete, test.complete)
			}
		})
	}
}
