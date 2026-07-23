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
	"github.com/aleksandergregersen/opsquest/internal/ui"
)

type colorSessionReader struct {
	lines   []string
	index   int
	prompts []string
}

func (r *colorSessionReader) ReadLine(prompt string, _ CompletionSource) (string, error) {
	r.prompts = append(r.prompts, prompt)
	if r.index >= len(r.lines) {
		return "", io.EOF
	}
	line := r.lines[r.index]
	r.index++
	return line, nil
}

func (r *colorSessionReader) Edit(_ sandbox.EditorRequest, _ viSaveFunc) error {
	return ErrInteractiveEditor
}

func TestSessionColorsChromeWithoutColoringSandboxOutput(t *testing.T) {
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, found := catalog.Find("1")
	if !found {
		t.Fatal("mission 1 not found")
	}

	player := profile.New("tester")
	style := ui.New(true)
	reader := &colorSessionReader{lines: []string{"pwd"}}
	out := &bytes.Buffer{}
	session := Session{
		Mission: item,
		Player:  &player,
		Saver:   profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "tester"),
		Out:     out,
		ErrOut:  &bytes.Buffer{},
		Reader:  reader,
		Catalog: catalog,
		Now:     func() time.Time { return time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC) },
		Style:   style,
	}

	result, err := session.Run()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Completed {
		t.Fatalf("session result = %#v", result)
	}
	if len(reader.prompts) != 1 || reader.prompts[0] != style.Prompt("/home/operator") {
		t.Fatalf("prompts = %#v, want colored mission prompt", reader.prompts)
	}

	output := out.String()
	for _, expected := range []string{
		style.Header("MISSION 01: Where Am I?"),
		style.World("Linux · World 1/4: First Day · Stage 1/6"),
		style.Section("INCIDENT"),
		style.Section("OBJECTIVE"),
		style.Difficulty("beginner"),
		style.CommandGuide(item.SuggestedCommands),
		style.Accent("hint"),
		style.Success("✓ Mission complete!"),
		style.Reward("+40 XP"),
		style.Accent("New command discovered: pwd"),
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("colored session output missing %q:\n%s", expected, output)
		}
	}
	// Command output is deliberately outside the presentation palette. Keeping
	// both adjacent newlines proves no reset or color code was inserted around
	// the value that also feeds outcome validation.
	if !strings.Contains(output, "\n/home/operator\n") {
		t.Fatalf("pwd output was not emitted as raw text:\n%q", output)
	}
}

func TestSessionColorsOnlyTheHintPrefix(t *testing.T) {
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, _ := catalog.Find("1")
	player := profile.New("tester")
	style := ui.New(true)
	out := &bytes.Buffer{}
	session := Session{
		Mission: item,
		Player:  &player,
		Saver:   profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "tester"),
		Out:     out,
		ErrOut:  &bytes.Buffer{},
		Reader:  &colorSessionReader{lines: []string{"hint", "quit"}},
		Catalog: catalog,
		Style:   style,
	}
	if _, err := session.Run(); err != nil {
		t.Fatal(err)
	}
	prefix := fmt.Sprintf("Hint 1/%d (-10 XP):", len(item.Hints))
	if !strings.Contains(out.String(), style.Warning(prefix)+" "+item.Hints[0]) {
		t.Fatalf("hint prefix was not styled independently:\n%q", out.String())
	}
	if strings.Contains(out.String(), style.Warning(prefix+" "+item.Hints[0])) {
		t.Fatalf("hint explanation was colored as one long warning:\n%q", out.String())
	}
}

func TestSessionUsesIndependentErrorStyle(t *testing.T) {
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, found := catalog.Find("1")
	if !found {
		t.Fatal("mission 1 not found")
	}

	player := profile.New("tester")
	style := ui.New(true)
	reader := &colorSessionReader{lines: []string{"definitely-not-a-command", "quit"}}
	errOut := &bytes.Buffer{}
	session := Session{
		Mission:    item,
		Player:     &player,
		Saver:      profile.NewStore(filepath.Join(t.TempDir(), "profile.json"), "tester"),
		Out:        &bytes.Buffer{},
		ErrOut:     errOut,
		Reader:     reader,
		Catalog:    catalog,
		Style:      ui.Style{},
		ErrorStyle: style,
	}

	result, err := session.Run()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Quit {
		t.Fatalf("session result = %#v", result)
	}
	want := style.Failure("definitely-not-a-command: command not available in this lab; type help to see supported commands")
	if !strings.Contains(errOut.String(), want+"\n") {
		t.Fatalf("stderr = %q, want styled error %q", errOut.String(), want)
	}
}
