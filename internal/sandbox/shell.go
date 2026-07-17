package sandbox

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

type Result struct {
	Output        string
	Commands      []string
	PipelineWidth int
	Editor        *EditorRequest
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

func (s *Sandbox) Execute(line string) (Result, error) {
	if strings.TrimSpace(line) != "" {
		s.History = append(s.History, line)
		if len(s.History) > 100 {
			s.History = append([]string(nil), s.History[len(s.History)-100:]...)
		}
	}
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
	pipelineWidth := len(parsed.stages)

	s.commandTrace = nil
	// Interactive commands are preflighted before any pipeline stage runs. This
	// prevents a rejected composition such as `touch changed | vi file` from
	// mutating the virtual filesystem before vi reports the unsupported pipeline.
	for _, stage := range parsed.stages {
		args := s.expandWords(stage.args)
		if args[0] != "vi" {
			continue
		}
		s.commandTrace = append(s.commandTrace, "vi")
		result := Result{Commands: s.trace(), PipelineWidth: pipelineWidth}
		if pipelineWidth != 1 {
			return result, fmt.Errorf("vi: pipelines are not supported")
		}
		if stage.inputPath != "" || stage.outputPath != "" {
			return result, fmt.Errorf("vi: redirection is not supported")
		}
		request, err := s.cmdVi(args[1:])
		if err != nil {
			return result, fmt.Errorf("vi: %w", err)
		}
		result.Editor = request
		return result, nil
	}

	stdin := ""
	for _, stage := range parsed.stages {
		if stage.inputPath != "" {
			stdin, err = s.FS.ReadFile(s.Resolve(stage.inputPath))
			if err != nil {
				return Result{Commands: s.trace(), PipelineWidth: pipelineWidth}, fmt.Errorf("redirect: %w", err)
			}
		}
		args := s.expandWords(stage.args)
		name := args[0]
		stdin, err = s.run(args, stdin)
		if err != nil {
			return Result{Commands: s.trace(), PipelineWidth: pipelineWidth}, fmt.Errorf("%s: %w", name, err)
		}
		if stage.outputPath != "" {
			target := s.Resolve(stage.outputPath)
			if stage.append {
				err = s.FS.AppendFile(target, stdin)
			} else {
				err = s.FS.WriteFile(target, stdin, 0)
			}
			if err != nil {
				return Result{Commands: s.trace(), PipelineWidth: pipelineWidth}, fmt.Errorf("redirect: %w", err)
			}
			s.removeArchiveMetadata(target)
			stdin = ""
		}
	}
	return Result{Output: stdin, Commands: s.trace(), PipelineWidth: pipelineWidth}, nil
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
	} else {
		parsed.stages = append(parsed.stages, stage)
	}
	return parsed, nil
}

func lex(line string, env map[string]string) ([]token, error) {
	var tokens []token
	var current strings.Builder
	var quote rune
	tokenStarted := false
	wordGlob := false
	runes := []rune(line)

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
				current.WriteRune(runes[i])
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
				current.WriteString(value)
				wordGlob = wordGlob || strings.ContainsAny(value, "*?[")
				i += consumed
				tokenStarted = true
				continue
			default:
				current.WriteRune(char)
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
			current.WriteRune(runes[i])
			continue
		}
		if quote == '"' && char == '$' {
			value, consumed := expandVariable(runes[i:], env)
			current.WriteString(value)
			i += consumed
			continue
		}
		current.WriteRune(char)
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

func (s *Sandbox) run(args []string, stdin string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	s.commandTrace = append(s.commandTrace, args[0])
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
		return s.cmdFind(args[1:])
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
	return "Available lab commands:\n  " + strings.Join(commands, "  ") + "\n\nShell features: pipelines (|), input (<), output (>), and append (>>) redirection.\nUse help COMMAND for examples.\n", nil
}

var commandNames = []string{
	"awk", "basename", "cat", "cd", "chmod", "chown", "clear", "cp", "cut", "dirname", "du", "echo", "env", "export",
	"find", "grep", "gzip", "gunzip", "head", "help", "history", "kill", "less", "ls", "man", "mkdir", "mv",
	"printf", "ps", "pwd", "rm", "rmdir", "sed", "sort", "stat", "tail", "tar", "touch", "tr", "uniq", "vi", "wc", "whoami",
}

// CommandNames returns the commands accepted by the teaching-shell dispatcher.
// Interactive completion uses the same list as shell help so the two surfaces
// cannot drift apart.
func CommandNames() []string {
	commands := append([]string(nil), commandNames...)
	sort.Strings(commands)
	return commands
}

var commandManuals = map[string]string{
	"awk":      "awk '{print $N}' [FILE] — print a whitespace-separated field",
	"basename": "basename PATH [SUFFIX] — print the final path component",
	"cat":      "cat [FILE...] — concatenate files or pipeline input",
	"cd":       "cd [DIR] — change directory; cd - returns to the previous directory",
	"chmod":    "chmod MODE FILE... — change octal permissions, for example chmod 750 deploy.sh",
	"chown":    "chown OWNER FILE... — change a file owner",
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
	"sort":     "sort [-nru] [FILE...] — sort lines",
	"stat":     "stat PATH... — inspect type, size, owner, and mode",
	"tail":     "tail [-n COUNT] [FILE...] — print the last lines",
	"tar":      "tar -xf ARCHIVE [-C DIR] — extract; -cf creates and -tf lists",
	"touch":    "touch FILE... — create empty files when they do not exist",
	"tr":       "tr [-ds] SET1 [SET2] — translate, delete, or squeeze characters",
	"uniq":     "uniq [-c] [FILE] — collapse adjacent duplicate lines",
	"vi": "vi FILE — edit one virtual UTF-8 text file up to 256 KiB interactively\n" +
		"Normal mode: h/j/k/l or arrows move, i inserts, x deletes a character, and dd deletes a line.\n" +
		"Insert mode: type text, Enter adds a line, Backspace deletes, and Esc returns to Normal mode.\n" +
		"Commands: :w writes, :q quits an unchanged buffer, :wq writes and quits, and :q! discards changes.\n" +
		"Options, multiple files, pipelines, redirection, shell escapes, plugins, and other vi features are unsupported.",
	"wc":     "wc [-l|-w|-c] [FILE...] — count lines, words, or bytes",
	"whoami": "whoami — print the current virtual user",
}

func (s *Sandbox) expandWords(words []shellWord) []string {
	expanded := make([]string, 0, len(words))
	for _, word := range words {
		if word.glob {
			matches := s.FS.Glob(s.CWD, word.value)
			if len(matches) > 0 {
				expanded = append(expanded, matches...)
				continue
			}
		}
		expanded = append(expanded, word.value)
	}
	return expanded
}
