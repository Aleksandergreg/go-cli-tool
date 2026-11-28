package terminal_test

import (
	"io"
	"strings"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/sandbox"
	"github.com/aleksandergregersen/opsquest/internal/terminal"
)

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
			if err := terminal.RunTestEditor(
				strings.NewReader(test.keys),
				io.Discard,
				terminal.EditorRequest{Path: result.Editor.Path, DisplayPath: result.Editor.DisplayPath, Content: result.Editor.Content},
				box.SaveEditorFile,
				func() (int, int) { return 80, 12 },
			); err != nil {
				t.Fatal(err)
			}
			complete, err := game.Validate(item.Validation, box, "")
			if err != nil {
				t.Fatal(err)
			}
			if complete != test.complete {
				t.Fatalf("mission complete = %v, want %v", complete, test.complete)
			}
		})
	}
}
