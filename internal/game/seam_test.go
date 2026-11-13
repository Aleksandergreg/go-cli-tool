package game

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/sandbox"
)

type seamCompletionSource struct {
	commands []string
	paths    map[string][]CompletionCandidate
}

func (s *seamCompletionSource) CommandNames() []string {
	return append([]string(nil), s.commands...)
}

func (s *seamCompletionSource) PathCandidates(prefix string) []CompletionCandidate {
	return append([]CompletionCandidate(nil), s.paths[prefix]...)
}

type seamEnvironment struct {
	prompt      string
	completions CompletionSource
	execute     func(context.Context, string) (Execution, error)
	observe     func(context.Context, mission.Condition) (bool, error)
	close       func() error
	closeCount  int
}

func (e *seamEnvironment) PromptLabel() string {
	return e.prompt
}

func (e *seamEnvironment) Execute(ctx context.Context, line string) (Execution, error) {
	if e.execute == nil {
		return Execution{}, nil
	}
	return e.execute(ctx, line)
}

func (e *seamEnvironment) Observe(ctx context.Context, condition mission.Condition) (bool, error) {
	if e.observe == nil {
		return false, fmt.Errorf("unexpected observation %s", condition.Type)
	}
	return e.observe(ctx, condition)
}

func (e *seamEnvironment) CompletionSource() CompletionSource {
	return e.completions
}

func (e *seamEnvironment) Close() error {
	e.closeCount++
	if e.close == nil {
		return nil
	}
	return e.close()
}

type seamReader struct {
	lines       []string
	index       int
	endErr      error
	prompts     []string
	completions []CompletionSource
	edit        func(sandbox.EditorRequest, viSaveFunc) error
}

func (r *seamReader) ReadLine(prompt string, completions CompletionSource) (string, error) {
	r.prompts = append(r.prompts, prompt)
	r.completions = append(r.completions, completions)
	if r.index >= len(r.lines) {
		if r.endErr != nil {
			return "", r.endErr
		}
		return "", io.EOF
	}
	line := r.lines[r.index]
	r.index++
	return line, nil
}

func (r *seamReader) Edit(request sandbox.EditorRequest, save viSaveFunc) error {
	if r.edit == nil {
		return ErrInteractiveEditor
	}
	return r.edit(request, save)
}

func seamCatalogMission(t *testing.T, ref string) (mission.Catalog, mission.Mission) {
	t.Helper()
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, found := catalog.Find(ref)
	if !found {
		t.Fatalf("mission %s not found", ref)
	}
	return catalog, item
}

type unavailableFactory struct {
	Factory
	availability Availability
}

func (f unavailableFactory) Availability(context.Context, mission.Mission) Availability {
	return f.availability
}
