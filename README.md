# OpsQuest

OpsQuest is **Duolingo meets a terminal sandbox**: a command-line game that teaches Linux through short, story-driven operations missions.

```console
$ opsquest play

MISSION 04: The Missing Log File
================================
Campaign: The Logpocalypse · Difficulty: beginner · Reward: 75 XP

The web server is failing, and the monitoring dashboard has chosen this
moment to become an abstract art installation.

Find every file ending in .log inside /var/app that contains the word ERROR.

opsquest:/var/app$ find . -name "*.log" -exec grep -l "ERROR" {} \;
./api/app.log
./worker/worker.log

✓ Mission complete!
+75 XP
New commands discovered: find, grep
```

The game validates the result, not a prescribed command. For example, the file-moving mission accepts `mv` or an equivalent `cp` followed by `rm`.

## What is included

Version 0.2 is an expanded Linux campaign:

- 16 hand-written missions across three story chapters
- An isolated, in-memory filesystem and process table
- Interactive line editing with Tab completion and Up/Down command recall
- Quote-aware globbing, pipelines, stage-local redirection, variables, and command history
- Outcome-based validation for output, files, permissions, processes, archives, and environment variables
- Hints with persistent XP penalties, plus mission status and environment restart controls
- Persistent XP, ranks, mission completion, command mastery, and six learning achievements
- Replayable missions without duplicate XP
- Mission previews, campaign filters, profile naming, and built-in diagnostics
- A multi-step **Production Friday** boss incident

The sandbox implements a focused teaching subset of:

```text
awk basename cat cd chmod chown clear cp cut dirname du echo env export find
grep gzip gunzip head help history kill less ls man mkdir mv printf ps pwd rm
rmdir sed sort stat tail tar touch tr uniq wc whoami
```

It also supports pipelines (`|`) and input/output redirection (`<`, `>`, `>>`).

## Quick start

OpsQuest requires Go 1.22 or newer. It uses the Go project's small `x/term` module for portable interactive line editing; gameplay itself remains local and in memory.

```console
$ go run ./cmd/opsquest play
```

Build a reusable binary:

```console
$ make build
$ ./bin/opsquest play
```

Or install it into your Go binary directory:

```console
$ go install ./cmd/opsquest
$ opsquest play
```

Useful commands:

```console
$ opsquest list
$ opsquest play 4
$ opsquest play linux-find-logs
$ opsquest show 16
$ opsquest list --campaign "Production Friday"
$ opsquest profile
$ opsquest profile --name alex
$ opsquest commands
$ opsquest achievements
$ opsquest doctor
$ opsquest reset
```

Inside a mission, use `hint`, `objective`, `status`, `restart`, or `quit`. Type `help` to list lab commands and `help COMMAND` for focused examples. In an interactive terminal, press Tab to complete supported lab commands and paths in the mission filesystem; use Up/Down to browse commands entered during the current mission session.

`status` reports how many outcome checks currently pass without prescribing a solution. `restart` rebuilds the mission environment while retaining hint penalties and command mastery.

## Campaigns

- **First Day** — navigation, directories, and basic file operations
- **The Logpocalypse** — searching, permissions, environment, processes, archives, and pipelines
- **Production Friday** — logs, aggregation, text transformation, ownership, disk usage, and a multi-fault boss incident

Achievements reward learning behavior rather than decoration: completing a first fix, building a three-command pipeline, practicing ten commands, solving missions without hints, beating an advanced incident, and finishing Linux.

## Safety model

Player input is parsed by OpsQuest itself. It is never passed to `sh`, `bash`, or another host process, and paths only address the mission's in-memory filesystem. Tab completion queries that same virtual filesystem and cannot expose host paths. Unsupported commands return a teaching-shell error.

The simulator also rejects virtual-root/current-directory removal, prevents file/directory type corruption during copies and moves, keeps virtual archive metadata synchronized, and blocks archive entries that try to escape their extraction directory.

This makes the introductory campaign safe and portable, with two intentional tradeoffs:

- It implements a useful subset of common command behavior rather than every shell feature and flag.
- Mission state exists only for the current attempt; player progress persists separately.

Docker-backed labs can later provide a real shell for advanced Linux and Docker campaigns without weakening the beginner experience.

## Progress storage

Profiles are stored as `profile.json` in the platform user configuration directory, under `opsquest`. Writes are atomic and use owner-only permissions.

Two environment variables are useful for development or portable installs:

- `OPSQUEST_HOME` changes the profile directory.
- `OPSQUEST_PLAYER` sets the display name for a new profile.

## Project structure

```text
cmd/opsquest/       Executable entry point
internal/cli/       Top-level commands and presentation
internal/game/      Interactive session and outcome validation
internal/mission/   Mission model, catalog, and embedded mission data
internal/profile/   XP, ranks, mastery, and atomic JSON persistence
internal/sandbox/   Virtual filesystem, shell parser, and commands
```

Mission content lives in [`internal/mission/data`](internal/mission/data). JSON keeps the binary dependency-free while retaining the proposed declarative setup/validation design. Mission decoding rejects unknown fields, unsafe paths, conflicting setup entries, invalid modes, duplicate PIDs, unknown validators, and non-contiguous numbering. A future external mission-pack loader can add YAML support without coupling content to the game engine.

Each mission defines:

```text
story + objective + starting directory
setup: directories, files, processes, environment, archives
hints + explanation + rewards
validation: one or more observable outcome conditions
```

## Development

Codex reads [`AGENTS.md`](AGENTS.md) for the repository's scope, safety invariants, package boundaries, and definition of done. Task-specific skills live in [`.agents/skills`](.agents/skills):

- `$add-mission` guides declarative mission content, outcome validation, and solution coverage.
- `$extend-sandbox-command` guides changes to the simulated shell's teaching subset.
- `$prepare-iteration` guides release-sized validation, documentation, and `iteration_N.md` reporting.

Mention a skill by name in a Codex request (for example, “use `$add-mission`”) or ask for the matching task so Codex can discover it from its trigger description.

The Makefile is the local source of truth for validation:

```console
$ make test
$ make validate-missions
$ make check-agent-docs
$ make smoke-test
$ make vet
$ make build
$ make race
$ make check
$ make check-all
```

`make check` runs agent-document validation, all Go tests (including embedded mission integrity and canonical solutions), vet, a binary build, and the isolated CLI smoke test. `make check-all` is the complete local quality gate and adds race detection. GitHub Actions runs that same comprehensive target; validation logic remains in the repository scripts and Makefile rather than the workflow YAML.

The tests exercise outcome-based mission solutions, profile compatibility, achievements, persistence, filters, previews, diagnostics, mission controls, parser behavior, archives, the virtual filesystem, and host-isolation invariants. The smoke test builds a temporary binary and uses a temporary `OPSQUEST_HOME`, so it never writes to the developer's real profile.

Third-party module licenses are recorded in [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).

## Roadmap

- v0.3: disposable Docker labs, container troubleshooting, ports, volumes, builds, networking, and Compose
- Later: Kubernetes missions backed by an isolated local cluster
- External mission packs, streaks, efficiency medals, and generated daily incidents
