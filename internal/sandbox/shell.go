package sandbox

import (
	"errors"
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
	// Output is standard output only; output validators compare it.
	Output        string
	Commands      []string
	PipelineWidth int
	Editor        *EditorRequest
	// Transcript orders standard output and error messages for display when
	// one line ran several commands (a; b, a && b, a || b) or a script handled
	// a failure. Execute then returns a nil error and the last failure, if
	// any, is the final entry. It is empty for a single command.
	Transcript []OutputChunk

	list         bool
	statusFailed bool
}

// OutputChunk is one piece of a Transcript: command output, or an error
// message the terminal shows on its error stream.
type OutputChunk struct {
	Text  string
	Error bool
}

type executionContext struct {
	commands         []string
	maxPipelineWidth int
	dispatchSteps    int
	scriptStack      []string
	scriptSteps      int
	// stderr collects messages from failures a command list handled inside a
	// script, for the caller's terminal.
	stderr []string
}

// errFailureStatus is a failing exit status without an error message, such
// as grep finding no match or false. The command's output stays valid; the
// status matters only to && and || and to a script's own exit status.
var errFailureStatus = errors.New("command reported a failing status")

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
	if !result.list && len(context.stderr) == 0 {
		return result, err
	}
	// A command list reports each command's output and error in order rather
	// than failing as a whole; earlier commands have already changed state.
	transcript := errorChunks(context.stderr)
	if result.list {
		transcript = append(transcript, result.Transcript...)
	} else if result.Output != "" {
		transcript = append(transcript, OutputChunk{Text: result.Output})
	}
	if err != nil {
		transcript = append(transcript, OutputChunk{Text: err.Error(), Error: true})
		if !result.list {
			// The single command failed as a whole, so none of it is practice.
			result.Commands = nil
		}
	}
	if transcriptBytes(transcript) > maxCommandOutputBytes {
		return Result{}, commandOutputLimitError()
	}
	result.Transcript = transcript
	return result, nil
}

// executePipeline runs one pipeline: validate, lex, parse, expand, open
// output redirections, and run each stage. statusFailed in the result reports
// the last stage's silent failing status, and the pipeline's exit status
// becomes the next $?. A blank or comment-only line leaves $? unchanged.
func (s *Sandbox) executePipeline(line string, context *executionContext, allowInteractive bool) (result Result, err error) {
	ran := true
	defer func() {
		if ran {
			s.lastStatus = exitStatus(err, result.statusFailed)
		}
	}()
	if err := validateShellSyntax(line); err != nil {
		return Result{}, withExitStatus(statusTrouble, err)
	}
	tokens, err := lex(line, s.Env, s.lastStatus)
	if err != nil {
		return Result{}, withExitStatus(statusTrouble, err)
	}
	parsed, err := parseCommandLine(tokens)
	if err != nil {
		return Result{}, withExitStatus(statusTrouble, err)
	}
	if len(parsed.stages) == 0 {
		ran = false
		return Result{}, nil
	}
	if !allowInteractive {
		if err := validateScriptCommands(parsed); err != nil {
			return Result{}, withExitStatus(statusTrouble, err)
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
	statusFailed := false
	for _, stage := range expanded.stages {
		// Like sh, > empties its file before the command runs, so `sort f > f`
		// reads nothing. If this stage then fails, the destination is restored:
		// a rejected command never destroys data.
		restore := func() error { return nil }
		if stage.outputPath != "" && !stage.append {
			restore, err = s.truncateRedirectTarget(s.Resolve(stage.outputPath))
			if err != nil {
				return Result{}, fmt.Errorf("redirect: %w", err)
			}
		}
		fail := func(err error) (Result, error) {
			if restoreErr := restore(); restoreErr != nil {
				err = errors.Join(err, fmt.Errorf("restore %s: %w", stage.outputPath, restoreErr))
			}
			return Result{}, err
		}
		if stage.inputPath != "" {
			stdin, err = s.FS.ReadFile(s.Resolve(stage.inputPath))
			if err != nil {
				return fail(fmt.Errorf("redirect: %w", err))
			}
		}
		name := stage.args[0]
		stdin, err = s.run(context, stage.args, stdin)
		// A pipeline's status is its last stage's, as in sh without pipefail.
		statusFailed = errors.Is(err, errFailureStatus)
		if statusFailed {
			err = nil
		}
		if err != nil {
			return fail(fmt.Errorf("%s: %w", name, err))
		}
		if stage.outputPath != "" {
			target := s.Resolve(stage.outputPath)
			if stage.append {
				err = s.FS.AppendFile(target, stdin)
			} else {
				err = s.FS.WriteFile(target, stdin, 0)
			}
			if err != nil {
				return fail(fmt.Errorf("redirect: %w", err))
			}
			s.removeArchiveMetadata(target)
			stdin = ""
		}
	}
	return Result{Output: stdin, statusFailed: statusFailed}, nil
}

// truncateRedirectTarget empties or creates target for > and returns a
// function that puts back its previous content, mode, owner, and archive
// metadata, or removes a file the truncation created.
func (s *Sandbox) truncateRedirectTarget(target string) (func() error, error) {
	previous, existed := s.FS.Entry(target)
	var saved Entry
	if existed {
		saved = *previous
	}
	archive, hadArchive := s.Archives[target]
	if err := s.FS.WriteFile(target, "", 0); err != nil {
		return nil, err
	}
	s.removeArchiveMetadata(target)
	return func() error {
		if !existed {
			return s.FS.Remove(target, false, true)
		}
		if err := s.FS.WriteFile(target, saved.Content, saved.Mode); err != nil {
			return err
		}
		if err := s.FS.Chown(target, saved.Owner); err != nil {
			return err
		}
		if !hadArchive {
			return nil
		}
		archives, err := s.planArchiveReplacement(target, archive)
		if err != nil {
			return err
		}
		s.Archives = archives
		return nil
	}, nil
}

func errorChunks(messages []string) []OutputChunk {
	chunks := make([]OutputChunk, 0, len(messages))
	for _, message := range messages {
		chunks = append(chunks, OutputChunk{Text: message, Error: true})
	}
	return chunks
}

func transcriptBytes(chunks []OutputChunk) int {
	total := 0
	for _, chunk := range chunks {
		total += len(chunk.Text)
	}
	return total
}
