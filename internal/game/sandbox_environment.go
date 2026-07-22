package game

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/sandbox"
)

// SandboxFactory preserves the existing in-memory Linux environment behind
// the generic game seam.
type SandboxFactory struct{}

func (SandboxFactory) Availability(ctx context.Context, item mission.Mission) Availability {
	ctx = defaultContext(ctx)
	if err := ctx.Err(); err != nil {
		return Availability{Detail: err.Error()}
	}
	if item.EffectiveEnvironment() != mission.EnvironmentSimulated {
		return Availability{Detail: fmt.Sprintf("environment %q requires a different factory", item.EffectiveEnvironment())}
	}
	return Availability{Available: true, Detail: "in-memory sandbox ready"}
}

func (SandboxFactory) Create(ctx context.Context, item mission.Mission) (Environment, error) {
	ctx = defaultContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if item.EffectiveEnvironment() != mission.EnvironmentSimulated {
		return nil, fmt.Errorf("environment %q is not supported by the simulated Linux factory", item.EffectiveEnvironment())
	}
	box, err := sandbox.New(item.Setup, item.StartDir)
	if err != nil {
		return nil, err
	}
	return newSandboxEnvironment(box), nil
}

type sandboxEnvironment struct {
	box *sandbox.Sandbox
}

func newSandboxEnvironment(box *sandbox.Sandbox) *sandboxEnvironment {
	return &sandboxEnvironment{box: box}
}

func (e *sandboxEnvironment) PromptLabel() string {
	return e.box.CWD
}

func (e *sandboxEnvironment) Execute(ctx context.Context, line string) (Execution, error) {
	if err := ctx.Err(); err != nil {
		return Execution{}, err
	}
	result, err := e.box.Execute(line)
	execution := Execution{
		Output:            result.Output,
		PracticedCommands: slices.Clone(result.Commands),
		PipelineWidth:     result.PipelineWidth,
	}
	if result.Editor != nil {
		request := *result.Editor
		execution.Interactive = &InteractiveAction{
			Command: request.Command,
			Run: func(reader CommandLineReader) error {
				return reader.Edit(request, e.box.SaveEditorFile)
			},
		}
	}
	return execution, err
}

func (e *sandboxEnvironment) Observe(ctx context.Context, condition mission.Condition) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	switch condition.Type {
	case mission.ConditionCWDEquals:
		return e.box.CWD == condition.Value, nil
	case mission.ConditionFileExists:
		entry, exists := e.box.FS.Entry(condition.Path)
		return exists && entry.Kind == sandbox.Regular, nil
	case mission.ConditionDirectoryExists:
		return e.box.FS.IsDir(condition.Path), nil
	case mission.ConditionPathMissing:
		return !e.box.FS.Exists(condition.Path), nil
	case mission.ConditionFileContentEquals:
		content, err := e.box.FS.ReadFile(condition.Path)
		if err != nil {
			return false, nil
		}
		return normalizeText(content) == normalizeText(condition.Value), nil
	case mission.ConditionFileContentContains:
		content, err := e.box.FS.ReadFile(condition.Path)
		if err != nil {
			return false, nil
		}
		return strings.Contains(content, condition.Value), nil
	case mission.ConditionFileLinesEqual:
		content, err := e.box.FS.ReadFile(condition.Path)
		if err != nil {
			return false, nil
		}
		return slices.EqualFunc(normalizedLines(content), condition.Values, func(actual, expected string) bool {
			return actual == strings.Join(strings.Fields(expected), " ")
		}), nil
	case mission.ConditionFileModeEquals:
		entry, exists := e.box.FS.Entry(condition.Path)
		if !exists || entry.Kind != sandbox.Regular {
			return false, nil
		}
		expected, err := strconv.ParseUint(condition.Value, 8, 12)
		if err != nil {
			return false, fmt.Errorf("invalid validation mode %q", condition.Value)
		}
		return entry.Mode == uint32(expected), nil
	case mission.ConditionFileOwnerEquals:
		entry, exists := e.box.FS.Entry(condition.Path)
		return exists && entry.Kind == sandbox.Regular && entry.Owner == condition.Value, nil
	case mission.ConditionProcessStopped:
		process, exists := e.box.Processes[condition.PID]
		return exists && !process.Running, nil
	case mission.ConditionProcessRunning:
		process, exists := e.box.Processes[condition.PID]
		return exists && process.Running, nil
	case mission.ConditionEnvironmentEquals:
		key, expected, found := strings.Cut(condition.Value, "=")
		if !found {
			return false, fmt.Errorf("env_equals value must be NAME=value")
		}
		actual, exists := e.box.Env[key]
		return exists && actual == expected, nil
	default:
		return false, fmt.Errorf("unknown validation type %q for simulated environment", condition.Type)
	}
}

func (e *sandboxEnvironment) CompletionSource() CompletionSource {
	return e
}

func (e *sandboxEnvironment) Close() error {
	return nil
}

func (e *sandboxEnvironment) CommandNames() []string {
	return sandbox.CommandNames()
}

func (e *sandboxEnvironment) PathCandidates(prefix string) []CompletionCandidate {
	if prefix == "~" && e.box.FS.IsDir(e.box.Resolve("~")) {
		return []CompletionCandidate{{Value: "~/", Directory: true}}
	}

	directoryPart, namePrefix := path.Split(prefix)
	lookupDirectory := directoryPart
	if lookupDirectory == "" {
		lookupDirectory = "."
	}
	children, err := e.box.FS.Children(e.box.Resolve(lookupDirectory))
	if err != nil {
		return nil
	}

	candidates := make([]CompletionCandidate, 0)
	for _, child := range children {
		name := path.Base(child)
		if !strings.HasPrefix(name, namePrefix) {
			continue
		}
		directory := e.box.FS.IsDir(child)
		value := directoryPart + name
		if directory {
			value += "/"
		}
		candidates = append(candidates, CompletionCandidate{Value: value, Directory: directory})
	}
	return candidates
}
