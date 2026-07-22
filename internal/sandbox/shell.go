package sandbox

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
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

type tokenKind int

const (
	wordToken tokenKind = iota
	pipeToken
	redirectToken
	appendToken
	inputToken
)

type token struct {
	kind  tokenKind
	value string
	glob  bool
}

type shellWord struct {
	value string
	glob  bool
}

type pipelineStage struct {
	args       []shellWord
	inputPath  string
	outputPath string
	append     bool
}

type commandLine struct {
	stages []pipelineStage
}

type expandedPipelineStage struct {
	args       []string
	inputPath  string
	outputPath string
	append     bool
}

type expandedCommandLine struct {
	stages []expandedPipelineStage
}

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

func parseCommandLine(tokens []token) (commandLine, error) {
	if len(tokens) == 0 {
		return commandLine{}, nil
	}
	parsed := commandLine{}
	stage := pipelineStage{}
	for i := 0; i < len(tokens); i++ {
		current := tokens[i]
		switch current.kind {
		case wordToken:
			stage.args = append(stage.args, shellWord{value: current.value, glob: current.glob})
		case pipeToken:
			if len(stage.args) == 0 {
				return commandLine{}, fmt.Errorf("unexpected pipe")
			}
			if len(parsed.stages) == maxPipelineStages-1 {
				return commandLine{}, fmt.Errorf("pipeline stage limit of %d exceeded", maxPipelineStages)
			}
			parsed.stages = append(parsed.stages, stage)
			stage = pipelineStage{}
		case redirectToken, appendToken, inputToken:
			if i+1 >= len(tokens) || tokens[i+1].kind != wordToken {
				return commandLine{}, fmt.Errorf("redirection requires a file")
			}
			i++
			file := tokens[i].value
			if current.kind == inputToken {
				if stage.inputPath != "" {
					return commandLine{}, fmt.Errorf("multiple input redirections")
				}
				stage.inputPath = file
			} else {
				if stage.outputPath != "" {
					return commandLine{}, fmt.Errorf("multiple output redirections")
				}
				stage.outputPath = file
				stage.append = current.kind == appendToken
			}
		}
	}
	if len(stage.args) == 0 {
		if len(parsed.stages) > 0 {
			return commandLine{}, fmt.Errorf("pipeline cannot end with a pipe")
		}
		return commandLine{}, fmt.Errorf("redirection requires a command")
	}
	if len(parsed.stages) == maxPipelineStages {
		return commandLine{}, fmt.Errorf("pipeline stage limit of %d exceeded", maxPipelineStages)
	}
	parsed.stages = append(parsed.stages, stage)
	return parsed, nil
}

func (s *Sandbox) expandCommandLine(parsed commandLine) (expandedCommandLine, error) {
	expanded := expandedCommandLine{stages: make([]expandedPipelineStage, 0, len(parsed.stages))}
	argumentCount := 0
	tokenBytes := 0
	consumeTokenBytes := func(value string) error {
		if len(value) > maxExpandedTokenBytes-tokenBytes {
			return fmt.Errorf("expanded command exceeds the %d KiB token limit", maxExpandedTokenBytes/1024)
		}
		tokenBytes += len(value)
		return nil
	}
	for _, stage := range parsed.stages {
		args := make([]string, 0, len(stage.args))
		for _, word := range stage.args {
			values := []string{word.value}
			if word.glob {
				if matches := s.FS.Glob(s.CWD, word.value); len(matches) > 0 {
					values = matches
				}
			}
			for _, value := range values {
				if argumentCount == maxExpandedArguments {
					return expandedCommandLine{}, fmt.Errorf("expanded command exceeds the %d-argument limit", maxExpandedArguments)
				}
				if err := consumeTokenBytes(value); err != nil {
					return expandedCommandLine{}, err
				}
				argumentCount++
				args = append(args, value)
			}
		}
		if err := consumeTokenBytes(stage.inputPath); err != nil {
			return expandedCommandLine{}, err
		}
		if err := consumeTokenBytes(stage.outputPath); err != nil {
			return expandedCommandLine{}, err
		}
		expanded.stages = append(expanded.stages, expandedPipelineStage{
			args:       args,
			inputPath:  stage.inputPath,
			outputPath: stage.outputPath,
			append:     stage.append,
		})
	}
	return expanded, nil
}

func lex(line string, env map[string]string) ([]token, error) {
	var tokens []token
	var current strings.Builder
	var quote rune
	tokenStarted := false
	wordGlob := false
	runes := []rune(line)
	expandedBytes := 0

	reserve := func(size int) error {
		if size > maxExpandedTokenBytes-expandedBytes {
			return fmt.Errorf("expanded command exceeds the %d KiB token limit", maxExpandedTokenBytes/1024)
		}
		expandedBytes += size
		return nil
	}
	writeString := func(value string) error {
		if err := reserve(len(value)); err != nil {
			return err
		}
		current.WriteString(value)
		return nil
	}
	writeRune := func(value rune) error {
		size := utf8.RuneLen(value)
		if size < 0 {
			size = utf8.RuneLen(utf8.RuneError)
		}
		if err := reserve(size); err != nil {
			return err
		}
		current.WriteRune(value)
		return nil
	}

	flush := func() {
		if tokenStarted {
			tokens = append(tokens, token{kind: wordToken, value: current.String(), glob: wordGlob})
			current.Reset()
			tokenStarted = false
			wordGlob = false
		}
	}

	for i := 0; i < len(runes); i++ {
		char := runes[i]
		if quote == 0 {
			switch {
			case unicode.IsSpace(char):
				flush()
				continue
			case char == '\'' || char == '"':
				quote = char
				tokenStarted = true
				continue
			case char == '\\':
				if i+1 >= len(runes) {
					return nil, fmt.Errorf("unfinished escape")
				}
				i++
				if err := writeRune(runes[i]); err != nil {
					return nil, err
				}
				tokenStarted = true
				continue
			case char == '#' && !tokenStarted:
				flush()
				return tokens, nil
			case char == '|':
				flush()
				tokens = append(tokens, token{kind: pipeToken, value: "|"})
				continue
			case char == '<':
				flush()
				tokens = append(tokens, token{kind: inputToken, value: "<"})
				continue
			case char == '>':
				flush()
				if i+1 < len(runes) && runes[i+1] == '>' {
					i++
					tokens = append(tokens, token{kind: appendToken, value: ">>"})
				} else {
					tokens = append(tokens, token{kind: redirectToken, value: ">"})
				}
				continue
			case char == '$':
				value, consumed := expandVariable(runes[i:], env)
				if err := writeString(value); err != nil {
					return nil, err
				}
				wordGlob = wordGlob || strings.ContainsAny(value, "*?[")
				i += consumed
				tokenStarted = true
				continue
			default:
				if err := writeRune(char); err != nil {
					return nil, err
				}
				wordGlob = wordGlob || strings.ContainsRune("*?[", char)
				tokenStarted = true
				continue
			}
		}

		if char == quote {
			quote = 0
			continue
		}
		if quote == '"' && char == '\\' {
			if i+1 >= len(runes) {
				return nil, fmt.Errorf("unfinished escape")
			}
			i++
			if err := writeRune(runes[i]); err != nil {
				return nil, err
			}
			continue
		}
		if quote == '"' && char == '$' {
			value, consumed := expandVariable(runes[i:], env)
			if err := writeString(value); err != nil {
				return nil, err
			}
			i += consumed
			continue
		}
		if err := writeRune(char); err != nil {
			return nil, err
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote")
	}
	flush()
	return tokens, nil
}

func expandVariable(input []rune, env map[string]string) (string, int) {
	if len(input) < 2 {
		return "$", 0
	}
	if input[1] == '{' {
		for i := 2; i < len(input); i++ {
			if input[i] == '}' {
				return env[string(input[2:i])], i
			}
		}
		return "$", 0
	}
	i := 1
	for i < len(input) && (unicode.IsLetter(input[i]) || unicode.IsDigit(input[i]) || input[i] == '_') {
		i++
	}
	if i == 1 {
		return "$", 0
	}
	return env[string(input[1:i])], i - 1
}

func (s *Sandbox) run(context *executionContext, args []string, stdin string) (output string, err error) {
	if len(args) == 0 {
		return "", nil
	}
	context.dispatchSteps++
	if context.dispatchSteps > maxExecutionDispatchSteps {
		return "", fmt.Errorf("command dispatch limit of %d exceeded", maxExecutionDispatchSteps)
	}
	defer func() {
		if err == nil && len(output) > maxCommandOutputBytes {
			output = ""
			err = commandOutputLimitError()
		}
	}()
	if len(context.scriptStack) > 0 {
		context.scriptSteps++
		if context.scriptSteps > maxScriptSteps {
			return "", fmt.Errorf("script command limit of %d exceeded", maxScriptSteps)
		}
	}
	if isExecutableScriptPath(args[0]) {
		context.commands = append(context.commands, "sh")
		return s.cmdExecutableScript(context, args, stdin)
	}
	context.commands = append(context.commands, args[0])
	switch args[0] {
	case "pwd":
		return s.cmdPwd(args[1:])
	case "cd":
		return s.cmdCD(args[1:])
	case "ls":
		return s.cmdLS(args[1:])
	case "mkdir":
		return s.cmdMkdir(args[1:])
	case "touch":
		return s.cmdTouch(args[1:])
	case "cp":
		return s.cmdCopy(args[1:])
	case "mv":
		return s.cmdMove(args[1:])
	case "rm", "rmdir":
		return s.cmdRemove(args[0], args[1:])
	case "cat", "less":
		return s.cmdCat(args[1:], stdin)
	case "head":
		return s.cmdHeadTail(args[1:], stdin, true)
	case "tail":
		return s.cmdHeadTail(args[1:], stdin, false)
	case "history":
		return s.cmdHistory(args[1:])
	case "grep":
		return s.cmdGrep(args[1:], stdin)
	case "find":
		return s.cmdFind(context, args[1:])
	case "sh":
		return s.cmdSh(context, args[1:], stdin)
	case "chmod":
		return s.cmdChmod(args[1:])
	case "chown":
		return s.cmdChown(args[1:])
	case "ps":
		return s.cmdPS(args[1:])
	case "kill":
		return s.cmdKill(args[1:])
	case "tar":
		return s.cmdTar(args[1:])
	case "gzip", "gunzip":
		return s.cmdGzip(args[0], args[1:])
	case "echo":
		return cmdEcho(args[1:])
	case "printf":
		return cmdPrintf(args[1:])
	case "export":
		return s.cmdExport(args[1:])
	case "env":
		return s.cmdEnv(args[1:])
	case "sort":
		return s.cmdSort(args[1:], stdin)
	case "uniq":
		return s.cmdUniq(args[1:], stdin)
	case "wc":
		return s.cmdWC(args[1:], stdin)
	case "awk":
		return s.cmdAwk(args[1:], stdin)
	case "cut":
		return s.cmdCut(args[1:], stdin)
	case "sed":
		return s.cmdSed(args[1:], stdin)
	case "tr":
		return s.cmdTr(args[1:], stdin)
	case "du":
		return s.cmdDU(args[1:])
	case "stat":
		return s.cmdStat(args[1:])
	case "basename", "dirname":
		return cmdPathPart(args[0], args[1:])
	case "whoami":
		return s.Env["USER"] + "\n", nil
	case "clear":
		return "", nil
	case "vi":
		return "", fmt.Errorf("interactive commands are not supported inside find -exec")
	case "help", "man":
		return shellHelp(args[1:])
	default:
		return "", fmt.Errorf("command not available in this lab; type help to see supported commands")
	}
}

func shellHelp(args []string) (string, error) {
	if len(args) > 1 {
		return "", fmt.Errorf("usage: help [COMMAND]")
	}
	if len(args) == 1 {
		manual, exists := commandManuals[args[0]]
		if !exists {
			return "", fmt.Errorf("no help available for %s", args[0])
		}
		return manual + "\n", nil
	}
	commands := CommandNames()
	return "Available lab commands:\n  " + strings.Join(commands, "  ") +
		"\n\nShell features: pipelines (|), input (<), output (>), and append (>>) redirection." +
		fmt.Sprintf("\nSandbox limits: %d KiB per command line; %d KiB expanded tokens; %d expanded arguments; %d pipeline stages; %d command dispatches; %d MiB per file and command output; %d MiB filesystem content and %d MiB archive payload; %d filesystem entries and %d archive entries.",
			maxCommandLineBytes/1024, maxExpandedTokenBytes/1024, maxExpandedArguments, maxPipelineStages, maxExecutionDispatchSteps,
			maxVirtualFileBytes/(1024*1024), maxVirtualFileSystemBytes/(1024*1024), maxVirtualArchiveBytes/(1024*1024), maxVirtualEntries, maxVirtualArchiveEntries) +
		"\nUse help COMMAND for examples.\n", nil
}

// CommandNames returns the commands accepted by the teaching-shell dispatcher.
// Interactive completion uses the same list as shell help so the two surfaces
// cannot drift apart.
func CommandNames() []string {
	return sortedKeys(commandManuals)
}

var commandManuals = map[string]string{
	"awk":      "awk '{print $N}' [FILE] — print a whitespace-separated field",
	"basename": "basename PATH [SUFFIX] — print the final path component",
	"cat":      "cat [FILE...] — concatenate files or pipeline input",
	"cd":       "cd [DIR] — change directory; cd - returns to the previous directory",
	"chmod":    "chmod MODE FILE... — change octal permissions, for example chmod 750 deploy.sh",
	"chown":    "chown OWNER FILE... — change a file owner; OWNER is limited to 256 bytes",
	"clear":    "clear — clear output in a real terminal; it is a no-op in scripted labs",
	"cp":       "cp [-r] SOURCE... DEST — copy files or directory trees",
	"cut":      "cut -d DELIMITER -f FIELD [FILE] — select a delimited field",
	"dirname":  "dirname PATH — print a path without its final component",
	"du":       "du [-a|-s] [-b|-h] [PATH...] — show virtual file sizes",
	"echo":     "echo [-n] [TEXT...] — print arguments",
	"env":      "env — print the current environment",
	"export":   "export NAME=value... — set shell environment variables",
	"find":     "find [PATH] [-name GLOB] [-type f|d] [-exec COMMAND {} \\;]",
	"grep":     "grep [-rilnvcFwE] PATTERN [FILE...] — print lines matching a pattern",
	"gzip":     "gzip FILE... — add the .gz suffix to virtual compressed files",
	"gunzip":   "gunzip FILE.gz... — restore virtual compressed files",
	"head":     "head [-n COUNT] [FILE...] — print the first lines",
	"help":     "help [COMMAND] — list lab commands or show focused command help",
	"history":  "history — show commands entered in this mission attempt",
	"kill":     "kill [-9|-15] PID... — stop a mission process",
	"less":     "less FILE... — display file content in the non-interactive lab",
	"ls":       "ls [-la] [PATH...] — list directory contents",
	"man":      "man COMMAND — show the same focused help as help COMMAND",
	"mkdir":    "mkdir [-p] DIR... — create directories",
	"mv":       "mv SOURCE... DEST — move or rename paths",
	"printf":   "printf FORMAT [VALUE...] — print formatted text with %s and escapes",
	"ps":       "ps — list the mission's running processes",
	"pwd":      "pwd — print the current working directory",
	"rm":       "rm [-rf] PATH... — remove paths inside the virtual filesystem",
	"rmdir":    "rmdir DIR... — remove empty directories",
	"sed":      "sed [-i] 's/REGEX/REPLACEMENT/g' [FILE] — transform text",
	"sh": "sh FILE — run a virtual UTF-8 script through the OpsQuest teaching shell\n" +
		"Blank lines, comments, #!/bin/sh, existing commands, variables, pipelines, and redirection are supported.\n" +
		"Executable paths such as ./deploy.sh require a shebang and an executable mode; sh FILE does not.\n" +
		"Scripts stop at the first error, restore their working directory and environment, and report virtual file/line locations.\n" +
		"Limits: 64 KiB per script, 8 KiB per line, nesting depth 8, 256 dispatched commands, and 1 MiB output.\n" +
		"Options, arguments, stdin, loops, conditionals, functions, substitutions, background jobs, and external programs are unsupported.",
	"sort":  "sort [-nru] [FILE...] — sort lines",
	"stat":  "stat PATH... — inspect type, size, owner, and mode",
	"tail":  "tail [-n COUNT] [FILE...] — print the last lines",
	"tar":   "tar -xf ARCHIVE [-C DIR] — extract; -C is extraction-only; -cf creates and -tf lists",
	"touch": "touch FILE... — create empty files when they do not exist",
	"tr":    "tr [-ds] SET1 [SET2] — translate, delete, or squeeze characters",
	"uniq":  "uniq [-c] [FILE] — collapse adjacent duplicate lines",
	"vi": "vi FILE — edit one virtual UTF-8 text file up to 256 KiB interactively\n" +
		"Normal mode: h/j/k/l or arrows move, i inserts, x deletes a character, and dd deletes a line.\n" +
		"Insert mode: type text, Enter adds a line, Backspace deletes, and Esc returns to Normal mode.\n" +
		"Commands: :w writes, :q quits an unchanged buffer, :wq writes and quits, and :q! discards changes.\n" +
		"Options, multiple files, pipelines, redirection, shell escapes, plugins, and other vi features are unsupported.",
	"wc":     "wc [-l|-w|-c] [FILE...] — count lines, words, or bytes",
	"whoami": "whoami — print the current virtual user",
}
