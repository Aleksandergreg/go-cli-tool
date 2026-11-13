package sandbox

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// commandListItem is one pipeline of a command list and the operator that
// joins it to the previous pipeline ("" for the first).
type commandListItem struct {
	operator string
	line     string
}

// splitCommandList splits line at unquoted ;, &&, and || into pipelines.
// Quotes, escapes, and comments follow the lexer, so `echo 'a; b'` stays one
// pipeline. As in sh, a trailing ; is allowed but a trailing && or || is not.
func splitCommandList(line string) ([]commandListItem, error) {
	runes := []rune(line)
	var items []commandListItem
	operator := ""
	start := 0
	var quote rune
	escaped, tokenStarted := false, false
	for index := 0; index < len(runes); index++ {
		char := runes[index]
		if escaped {
			escaped = false
			tokenStarted = true
			continue
		}
		if quote == '\'' {
			if char == quote {
				quote = 0
			}
			continue
		}
		if char == '\\' {
			escaped = true
			continue
		}
		if quote == '"' {
			if char == quote {
				quote = 0
			}
			continue
		}
		if unicode.IsSpace(char) {
			tokenStarted = false
			continue
		}
		if char == '#' && !tokenStarted {
			break
		}
		if char == '\'' || char == '"' {
			quote = char
			tokenStarted = true
			continue
		}
		next := ""
		switch {
		case char == ';':
			next = ";"
		case char == '&' && index+1 < len(runes) && runes[index+1] == '&':
			next = "&&"
		case char == '|' && index+1 < len(runes) && runes[index+1] == '|':
			next = "||"
		}
		if next == "" {
			// A pipe or redirection ends a word, so a following # is a comment.
			tokenStarted = !strings.ContainsRune("|<>", char)
			continue
		}
		segment := string(runes[start:index])
		if strings.TrimSpace(segment) == "" {
			return nil, fmt.Errorf("syntax error near unexpected %q", next)
		}
		items = append(items, commandListItem{operator: operator, line: segment})
		operator = next
		index += len(next) - 1
		start = index + 1
		tokenStarted = false
	}
	if len(items) == 0 {
		return []commandListItem{{line: line}}, nil
	}
	rest := string(runes[start:])
	if trimmed := strings.TrimSpace(rest); trimmed == "" || strings.HasPrefix(trimmed, "#") {
		if operator != ";" {
			return nil, fmt.Errorf("command list cannot end with %q", operator)
		}
		return items, nil
	}
	return append(items, commandListItem{operator: operator, line: rest}), nil
}

// executeLine runs one input line: a single pipeline or a command list. Each
// pipeline expands variables and globs only when its turn comes, so
// `export X=1; echo $X` prints 1 and `cd dir && ls` lists dir. As in sh,
// a && b runs b only after a succeeds, a || b runs b only after a fails, and
// the two operators bind equally from left to right.
//
// For a list, the result's Transcript holds every output and handled error
// in order, a failed pipeline contributes no practiced commands, and the
// returned error is the last pipeline's failure, if any.
func (s *Sandbox) executeLine(line string, context *executionContext, allowInteractive bool) (Result, error) {
	items, err := splitCommandList(line)
	if err == nil && len(items) > 1 {
		err = s.preflightCommandList(items, allowInteractive)
	}
	if err != nil {
		// Like sh, a syntax error runs nothing and sets $? to 2.
		s.lastStatus = statusTrouble
		return Result{}, withExitStatus(statusTrouble, err)
	}
	if len(items) == 1 {
		return s.executePipeline(items[0].line, context, allowInteractive)
	}

	result := Result{list: true}
	var output strings.Builder
	transcriptSize := 0
	add := func(chunk OutputChunk) error {
		if chunk.Text == "" {
			return nil
		}
		if len(chunk.Text) > maxCommandOutputBytes-transcriptSize {
			return commandOutputLimitError()
		}
		transcriptSize += len(chunk.Text)
		result.Transcript = append(result.Transcript, chunk)
		if !chunk.Error {
			output.WriteString(chunk.Text)
		}
		return nil
	}
	// pending holds a failure until the next pipeline runs: then the failure
	// was handled and is reported in order; otherwise it is the list's result.
	var pending error
	failed := false
	for index, item := range items {
		if index > 0 && (item.operator == "&&" && failed || item.operator == "||" && !failed) {
			continue
		}
		if pending != nil {
			if err := add(OutputChunk{Text: pending.Error(), Error: true}); err != nil {
				return Result{}, err
			}
			pending = nil
		}
		commandsBefore, stderrBefore := len(context.commands), len(context.stderr)
		pipeline, err := s.executePipeline(item.line, context, allowInteractive)
		diagnostics := slices.Clone(context.stderr[stderrBefore:])
		context.stderr = context.stderr[:stderrBefore]
		for _, message := range diagnostics {
			if err := add(OutputChunk{Text: message, Error: true}); err != nil {
				return Result{}, err
			}
		}
		if err != nil {
			context.commands = context.commands[:commandsBefore]
			pending, failed = err, true
			continue
		}
		if err := add(OutputChunk{Text: pipeline.Output}); err != nil {
			return Result{}, err
		}
		failed = pipeline.statusFailed
	}
	result.Output = output.String()
	result.statusFailed = failed && pending == nil
	return result, pending
}

// preflightCommandList checks every pipeline before the first one runs, so a
// syntax error late in a list changes nothing, as in sh.
func (s *Sandbox) preflightCommandList(items []commandListItem, allowInteractive bool) error {
	for _, item := range items {
		if err := validateShellSyntax(item.line); err != nil {
			return err
		}
		tokens, err := lex(item.line, s.Env, s.lastStatus)
		if err != nil {
			return err
		}
		parsed, err := parseCommandLine(tokens)
		if err != nil {
			return err
		}
		if !allowInteractive {
			if err := validateScriptCommands(parsed); err != nil {
				return err
			}
		}
		// The editor runs after Execute returns, so it cannot sit in a list.
		for _, stage := range parsed.stages {
			if stage.args[0].value != "vi" {
				continue
			}
			if !allowInteractive {
				return fmt.Errorf("vi: interactive commands are not supported in scripts")
			}
			return fmt.Errorf("vi: interactive commands cannot be combined with ;, &&, or ||")
		}
	}
	return nil
}
