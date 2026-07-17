# OpsQuest shell semantics

## Execution boundary

`Sandbox.Execute` is the sole entry for a player command. It records bounded in-memory history, lexes the line, parses a pipeline, expands virtual globs, dispatches only whitelisted handlers, and returns output plus a command trace. A command handler may touch only `Sandbox` state: `FileSystem`, `Env`, `Processes`, `Archives`, navigation state, and history.

`sh FILE` and executable virtual paths reuse the private line executor with one shared execution context. Script source is never added to interactive history, while nested commands contribute to the returned command trace and maximum pipeline width. Script execution is bounded by source, line, nesting, dispatched-command, and output limits.

Do not add a fallback to the host. Unknown commands must remain errors, and player input must never reach `sh`, `bash`, `os/exec`, host file APIs, or host process controls.

## Current parsing subset

- Words are separated by unquoted whitespace.
- Single quotes preserve literal text. Double quotes allow basic `$NAME` and `${NAME}` expansion plus escaped characters.
- A backslash escapes the following character; unfinished escapes and quotes are errors.
- A `#` beginning a new unquoted token starts a comment.
- Unquoted `*`, `?`, and `[` patterns expand against the virtual filesystem with POSIX-style path matching. Quoted patterns remain literal.
- `|` creates a pipeline. `<`, `>`, and `>>` are stage-local redirections backed by virtual files.
- Each parsed stage requires a command. Missing redirection paths, duplicate same-direction redirects, leading/trailing pipes, and empty stages are errors.
- Control operators, subshells, command substitution, background jobs, functions, aliases, and a general shell language are outside the supported subset.

## Script subset

- `sh FILE` accepts exactly one virtual UTF-8 file. It does not support flags, stdin-fed source, or positional arguments.
- A path containing `/`, such as `./deploy.sh`, is treated as a virtual executable script. It requires an executable mode and a supported `#!/bin/sh` or `#!/usr/bin/env sh` shebang.
- Blank lines and comments are skipped. Every other line uses the same lexer, parser, globbing, dispatcher, pipelines, and virtual redirection as an interactive command.
- Script `cd` and `export` state is visible to later lines but restored on return. Virtual filesystem, archive, and process mutations persist.
- Scripts stop on the first error and add the resolved virtual filename and source line to the diagnostic.
- Interactive commands, shell control language, standalone assignments, substitutions, recursion, external programs, and host execution remain unsupported.
- Script stdout may feed a later pipeline stage or virtual output redirection. Incoming pipeline and redirected stdin are rejected because the simulator does not model a shared stdin stream across script lines.

Keep a parser change narrow. If a new syntax form is not implemented faithfully enough for teaching, reject it clearly rather than treating punctuation as ordinary text in a misleading way.

## Virtual paths and state

- Paths use Go's `path` package so lab behavior is slash-based on every host OS.
- `Sandbox.Resolve` cleans absolute or CWD-relative paths and expands `~` or `~/` through the virtual `HOME`.
- `FileSystem` owns all entries, content, modes, owners, traversal, globbing, copies, moves, and removal. Virtual `/` is not the host root.
- Redirection reads and writes only through `FileSystem`. Overwriting a virtual archive also clears its virtual archive metadata.
- `cd` updates virtual `PWD`, `OLDPWD`, CWD, and previous-directory state.
- `ps` and `kill` inspect or mutate only mission processes. Archive and compression commands operate on virtual metadata and files.

## Command consistency checklist

- Define accepted flags and whether combined short flags are supported.
- Define stdin behavior, file operand behavior, and precedence when both are possible.
- Match existing newline and multi-file output conventions when practical.
- Resolve every filename once through the sandbox and return useful errors for missing or wrong-kind entries.
- Preserve file mode, owner, and archive metadata when the analogous existing operation does.
- Keep glob expansion quote-aware and decide how unmatched globs are handled using the existing shell rule.
- Ensure pipeline output becomes the next stage's stdin and redirection remains local to its stage.
- Trace successful nested or top-level command dispatch consistently for command mastery.
- Add a concise manual line that describes only supported syntax.
- Test successful behavior, edge cases, failures, unsupported features, and isolation.
