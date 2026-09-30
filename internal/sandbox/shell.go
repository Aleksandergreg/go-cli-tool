package sandbox

import (
	"fmt"
	"slices"
	"strings"
)

const (
	maxCommandLineBytes       = 64 * 1024
	maxCommandOutputBytes     = maxVirtualFileBytes
	maxExpandedTokenBytes     = maxCommandOutputBytes
	maxExpandedArguments      = maxVirtualEntries
	maxPipelineStages         = 64
	maxExecutionDispatchSteps = 512
)

type Result struct {
	Output        string
	Commands      []string
	PipelineWidth int
	Editor        *EditorRequest
}

type executionContext struct {
	commands         []string
	maxPipelineWidth int
	dispatchSteps    int
	scriptStack      []string
	scriptSteps      int
}

const (
	wordToken tokenKind = iota
	pipeToken
	redirectToken
	appendToken
	inputToken
)

func (s *Sandbox) Execute(line string) (Result, error) {
	if len(line) > maxCommandLineBytes {
		return Result{}, fmt.Errorf("command line exceeds the %d KiB limit", maxCommandLineBytes/1024)
	}
	if strings.TrimSpace(line) != "" {
		s.History = append(s.History, line)
		if len(s.History) > 100 {
			s.History = slices.Clone(s.History[len(s.History)-100:])
		}
	}
	context := &executionContext{}
	result, err := s.executeLine(line, context, true)
	result.Commands = slices.Clone(context.commands)
	result.PipelineWidth = context.maxPipelineWidth
	return result, err
}

func (s *Sandbox) executeLine(line string, context *executionContext, allowInteractive bool) (Result, error) {
	tokens, err := lex(line, s.Env)
	if err != nil {
		return Result{}, err
	}
	parsed, err := parseCommandLine(tokens)
	if err != nil {
		return Result{}, err
	}
	if len(parsed.stages) == 0 {
		return Result{}, nil
	}
	if !allowInteractive {
		if err := validateScriptCommands(parsed); err != nil {
			return Result{}, err
		}
	}
	expanded, err := s.expandCommandLine(parsed)
	if err != nil {
		return Result{}, err
	}
	pipelineWidth := len(expanded.stages)
	context.maxPipelineWidth = max(context.maxPipelineWidth, pipelineWidth)

	// Interactive commands are preflighted before any pipeline stage runs. This
	// prevents a rejected composition such as `touch changed | vi file` from
	// mutating the virtual filesystem before vi reports the unsupported pipeline.
	for _, stage := range expanded.stages {
		if stage.args[0] != "vi" {
			continue
		}
		context.commands = append(context.commands, "vi")
		if !allowInteractive {
			return Result{}, fmt.Errorf("vi: interactive commands are not supported in scripts")
		}
		result := Result{}
		if pipelineWidth != 1 {
			return result, fmt.Errorf("vi: pipelines are not supported")
		}
		if stage.inputPath != "" || stage.outputPath != "" {
			return result, fmt.Errorf("vi: redirection is not supported")
		}
		request, err := s.cmdVi(stage.args[1:])
		if err != nil {
			return result, fmt.Errorf("vi: %w", err)
		}
		result.Editor = request
		return result, nil
	}
	// A script can produce pipeline output, but this teaching subset does not
	// model a shared stdin stream across its independent lines. Reject incoming
	// pipeline or file input before an earlier stage can mutate sandbox state.
	for index, stage := range expanded.stages {
		if !isScriptInvocation(stage.args[0]) {
			continue
		}
		if index > 0 || stage.inputPath != "" {
			return Result{}, fmt.Errorf("%s: script input from pipelines or redirection is not supported", stage.args[0])
		}
	}

	stdin := ""
	for _, stage := range expanded.stages {
		if stage.inputPath != "" {
			stdin, err = s.FS.ReadFile(s.Resolve(stage.inputPath))
			if err != nil {
				return Result{}, fmt.Errorf("redirect: %w", err)
			}
		}
		name := stage.args[0]
		stdin, err = s.run(context, stage.args, stdin)
		if err != nil {
			return Result{}, fmt.Errorf("%s: %w", name, err)
		}
		if stage.outputPath != "" {
			target := s.Resolve(stage.outputPath)
			if stage.append {
				err = s.FS.AppendFile(target, stdin)
			} else {
				err = s.FS.WriteFile(target, stdin, 0)
			}
			if err != nil {
				return Result{}, fmt.Errorf("redirect: %w", err)
			}
			s.removeArchiveMetadata(target)
			stdin = ""
		}
	}
	return Result{Output: stdin}, nil
}
