---
description: Reference for OpsQuest terminal editing, navigation, teaching-shell commands, vi, and safe scripts.
audience: players
status: current
---

# Controls and commands

OpsQuest provides line editing and a focused Linux teaching shell inside each mission.

With `opsquest play --web`, command entry, completion, history, and raw command
output remain in the terminal. Mission narrative, objectives, revealed hints,
outcome checks, and the field guide appear in the [web mission companion](web-companion.md).

## Interactive editing

| Keys | Action |
| --- | --- |
| Left / Right | Move by one character |
| Up / Down | Recall command history |
| Home / End or Ctrl-A / Ctrl-E | Move to a line boundary |
| Option/Ctrl-Left or Option/Ctrl-Right | Move by one word when supported by the terminal |
| Tab | Complete commands and virtual paths |
| Backspace / Delete | Remove text before or under the cursor |
| Ctrl-W | Delete the previous word |

Completion reads only the active environment's command vocabulary and virtual filesystem.

## Teaching-shell command set

```text
awk basename cat cd chmod chown clear cp cut dirname du echo env export find
grep gzip gunzip head help history kill less ls man mkdir mv printf ps pwd rm
rmdir sed sh sort stat tail tar touch tr uniq vi wc whoami
```

The shell also supports quote-aware variables and globs, pipelines (`|`), input redirection (`<`), and output redirection (`>` and `>>`). It intentionally implements a teaching subset of each command. Use `help COMMAND` for the exact supported flags and examples.

## Mission navigation

```console
opsquest:/backups$ map
opsquest:/backups$ list --completed
opsquest:/backups$ world 2
opsquest:/srv/release$ play 3
opsquest:/home/operator$ next
opsquest:/backups$ previous
```

The optional `opsquest` prefix also works inside a mission.

## Virtual `vi`

The compact modal editor opens one virtual UTF-8 text file:

```console
opsquest:/workspace$ vi notes.txt
```

Normal mode supports `h`, `j`, `k`, `l`, arrow movement, `i`, `x`, and `dd`. Press Esc to leave insert mode. Use `:w`, `:q`, `:wq`, or `:q!` to save or leave. The editor has no plugins, shell escape, external commands, search, registers, multi-file mode, pipeline placement, or redirected input.

## Virtual shell scripts

Scripts run through the same parser and command dispatcher as interactive input:

```console
opsquest:/workspace$ vi report.sh
opsquest:/workspace$ sh report.sh
opsquest:/workspace$ chmod 750 report.sh
opsquest:/workspace$ ./report.sh
```

`sh FILE` does not require executable permission. Direct paths require an executable mode and a supported shell shebang. Files, archives, and mission processes retain changes, while a script's working directory and exported environment are restored when it returns.

This is not a complete POSIX shell. Loops, conditionals, functions, substitutions, background jobs, external programs, positional arguments, `sh -c`, stdin-fed source, and interactive editor calls from scripts are rejected.

For implementation limits and trust boundaries, see [Sandbox and safety](../technical/sandbox-and-safety.md).
