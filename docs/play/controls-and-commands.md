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
awk basename cat cd chmod chown clear cp cut dirname du echo env export false find
grep gzip gunzip head help history kill less ls man mkdir mv printf ps pwd rm
rmdir sed sh sort stat tail tar touch tr true uniq vi wc whoami
```

The shell also supports quote-aware variables and globs, pipelines (`|`), input redirection (`<`), and output redirection (`>` and `>>`). It intentionally implements a teaching subset of each command. Use `help COMMAND` for the exact supported flags and examples.

Command lists join commands on one line, as in `sh`:

- `a; b` always runs `b`.
- `a && b` runs `b` only when `a` succeeds.
- `a || b` runs `b` only when `a` fails.

`&&` and `||` bind equally from left to right, so `a && b || c` runs `c` when either `a` or `b` fails. Each command expands variables and globs when its turn comes, so `cd logs && ls *` lists the new directory. A failure is shown, then the list continues as its operators allow. `grep` without a match fails silently, which makes `grep -q ERROR app.log && echo alert` work; `true` and `false` set a status and nothing else.

Output redirection with `>` empties its file before the command runs, so `sort notes.txt > notes.txt` leaves an empty file, just as on Linux. If the command fails, the file is put back. Wildcards skip hidden names unless the pattern starts with a dot: `*` ignores `.env`, while `.*` matches it.

`$?` holds the exit status of the last command, as on Linux: `ls /missing; echo $?` prints `2`. The lab reports `0` for success, `1` for most failures (including `grep` without a match and `false`), `2` for shell syntax errors and `ls`, `grep`, or `sort` trouble, `126` for a script that exists but cannot run, and `127` for a command the lab does not provide. A blank line keeps the previous status, and each script starts at `0`.

Background jobs (`&`), subshells, command substitution, other special parameters such as `$1` or `$$`, and file-descriptor redirection such as `2>` are rejected with an explanation before anything runs. Quote or escape these characters to use them as data.

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

Command lists work in scripts too: a failure handled by `||` is reported with its script location and the script continues, and a script's exit status is its last command's. This is not a complete POSIX shell. Loops, `if` conditionals, functions, substitutions, background jobs, external programs, positional arguments, `sh -c`, stdin-fed source, and interactive editor calls from scripts are rejected.

For implementation limits and trust boundaries, see [Sandbox and safety](../technical/sandbox-and-safety.md).
