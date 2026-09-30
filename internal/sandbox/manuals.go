package sandbox

import (
	"fmt"
	"strings"
)

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
