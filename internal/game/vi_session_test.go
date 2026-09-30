package game

import (
	"bytes"
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
