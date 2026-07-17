package sandbox

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

type Result struct {
	Output   string
	Commands []string
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
}

type commandLine struct {
	stages     [][]string
	inputPath  string
	outputPath string
	append     bool
}

func (s *Sandbox) Execute(line string) (Result, error) {
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

	stdin := ""
	if parsed.inputPath != "" {
		stdin, err = s.FS.ReadFile(s.Resolve(parsed.inputPath))
		if err != nil {
			return Result{}, err
		}
	}

	commands := make([]string, 0, len(parsed.stages))
	for _, stage := range parsed.stages {
		name := stage[0]
		commands = append(commands, name)
		stdin, err = s.run(stage, stdin)
		if err != nil {
			return Result{Commands: commands}, fmt.Errorf("%s: %w", name, err)
		}
	}

	if parsed.outputPath != "" {
		target := s.Resolve(parsed.outputPath)
		if parsed.append {
			err = s.FS.AppendFile(target, stdin)
		} else {
			err = s.FS.WriteFile(target, stdin, 0)
		}
		if err != nil {
			return Result{Commands: commands}, fmt.Errorf("redirect: %w", err)
		}
		stdin = ""
	}
	return Result{Output: stdin, Commands: commands}, nil
}

func parseCommandLine(tokens []token) (commandLine, error) {
	if len(tokens) == 0 {
		return commandLine{}, nil
	}
	parsed := commandLine{}
	stage := make([]string, 0)
	for i := 0; i < len(tokens); i++ {
		current := tokens[i]
		switch current.kind {
		case wordToken:
			stage = append(stage, current.value)
		case pipeToken:
			if len(stage) == 0 {
				return commandLine{}, fmt.Errorf("unexpected pipe")
			}
			parsed.stages = append(parsed.stages, stage)
			stage = nil
		case redirectToken, appendToken, inputToken:
			if i+1 >= len(tokens) || tokens[i+1].kind != wordToken {
				return commandLine{}, fmt.Errorf("redirection requires a file")
			}
			i++
			file := tokens[i].value
			if current.kind == inputToken {
				if parsed.inputPath != "" {
					return commandLine{}, fmt.Errorf("multiple input redirections")
				}
				parsed.inputPath = file
			} else {
				if parsed.outputPath != "" {
					return commandLine{}, fmt.Errorf("multiple output redirections")
				}
				parsed.outputPath = file
				parsed.append = current.kind == appendToken
			}
		}
	}
	if len(stage) == 0 {
		if len(parsed.stages) > 0 {
			return commandLine{}, fmt.Errorf("pipeline cannot end with a pipe")
		}
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
	runes := []rune(line)

	flush := func() {
		if tokenStarted {
			tokens = append(tokens, token{kind: wordToken, value: current.String()})
			current.Reset()
			tokenStarted = false
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
				i += consumed
				tokenStarted = true
				continue
			default:
				current.WriteRune(char)
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
	case "whoami":
		return s.Env["USER"] + "\n", nil
	case "clear":
		return "", nil
	case "help":
		return shellHelp(), nil
	default:
		return "", fmt.Errorf("command not available in this lab; type help to see supported commands")
	}
}

func shellHelp() string {
	commands := []string{
		"awk", "cat", "cd", "chmod", "chown", "cp", "cut", "echo", "env", "export",
		"find", "grep", "gzip", "gunzip", "head", "kill", "less", "ls", "mkdir", "mv",
		"printf", "ps", "pwd", "rm", "rmdir", "sort", "tail", "tar", "touch", "uniq", "wc", "whoami",
	}
	sort.Strings(commands)
	return "Available lab commands:\n  " + strings.Join(commands, "  ") + "\n\nShell features: pipelines (|), input (<), output (>), and append (>>) redirection.\n"
}

func expandGlobs(fs *FileSystem, cwd string, args []string) []string {
	expanded := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.ContainsAny(arg, "*?[") {
			matches := fs.Glob(cwd, arg)
			if len(matches) > 0 {
				expanded = append(expanded, matches...)
				continue
			}
		}
		expanded = append(expanded, arg)
	}
	return expanded
}
