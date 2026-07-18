# OpsQuest — current implementation brief

OpsQuest is a Go CLI game described as **Duolingo meets a terminal sandbox**. Its Linux curriculum runs through short, story-driven operations missions in deterministic, in-memory environments; the current release also includes one optional Docker Foundations lab backed by isolated, attempt-owned containers.

The current executable reports version **0.3.0**. <!-- x-release-please-version --> Kubernetes remains a later expansion and is not implemented.

## Product today

OpsQuest currently ships:

- 19 hand-written Linux missions across four ordered learning worlds
- One optional Docker mission in the **It Works on My Machine** world
- Strict, embedded JSON mission definitions
- Observable-outcome validation rather than prescribed command sequences
- An in-memory filesystem, process table, environment, and virtual archive model
- A quote-aware teaching shell with globbing, pipelines, redirection, and variables
- A first-play guide explaining objectives, hints, XP, worlds, navigation, and sandbox boundaries
- Interactive prompt editing, history, path completion, world jumping, and in-mission navigation
- Semantic terminal color for CLI and mission presentation, with plain redirected output
- A compact modal `vi` teaching editor
- A bounded, simulated shell-script runner
- Persistent profiles, XP, ranks, hints, command mastery, and six achievements
- Continuous world-aware play, explicit mission replay, previews, filters, and diagnostics
- Test, vet, race, build, smoke-test, mission-validation, and CI quality gates

Linux commands, scripts, and virtual paths are never executed by or resolved against the host operating system. Docker-lab input is parsed into a typed teaching subset; OpsQuest constructs fixed engine operations that can address only exact, labeled resources belonging to that attempt.

## Core gameplay loop

Each mission contains:

- A short incident or story
- Declarative setup for an isolated virtual environment
- An objective that does not prescribe one exact command
- A free list of relevant command names without flags or a prescribed sequence
- Between one and five progressive hints with XP penalties
- One or more observable validation conditions
- A completion explanation and XP reward

For example:

```console
$ opsquest play 4

MISSION 04: The Missing Log File
================================
Linux · World 1/4: First Day · Stage 4/5

Find every file ending in .log inside /var/app that contains ERROR.

opsquest:/var/app$ find . -name "*.log" -exec grep -l "ERROR" {} \;
./api/app.log
./worker/worker.log

✓ Mission complete!
+75 XP
New commands discovered: find, grep
```

The validator judges the resulting output or environment. If `mv` and `cp` followed by `rm` both produce the required final state, both approaches can succeed.

## Worlds and progression

The current Linux curriculum is split into track-local worlds:

1. **First Day** (Missions 1–5) — orientation, directories, files, search, and movement
2. **The Logpocalypse** (Missions 6–10) — beginner permissions/environment, intermediate processes/archives, and an advanced pipeline boss
3. **Production Friday** — aggregation, transformation, ownership, disk usage, and a multi-step boss incident
4. **The Automation Shift** — modal editing, reusable scripts, executable modes, and child-shell scope

Global mission numbers and IDs remain stable references. The UI separately shows `World N/M` and `Stage N/M`, and `opsquest map` summarizes progress. Worlds provide a recommended path rather than locks: players may jump directly to any world, mission number, or ID and replay completed content.

XP, ranks, active and completed hint usage, completed missions, practiced commands, achievements, and the additive onboarding marker persist in a versioned JSON profile. A mission sandbox resets between attempts; profile progress does not. Display names are limited to 40 printable Unicode characters so persisted terminal controls cannot spoof CLI output.

Ranks progress from Intern through Operator, Sysadmin, and SRE levels. The six current achievements reward first completion, pipeline practice, command breadth, hint-free solutions, advanced incidents, and completing the Linux campaign.

## CLI surface

Top-level commands are:

```text
play       map/list/campaign   guide/tutorial     show/mission
profile    commands            achievements       doctor
reset      version             help
```

Inside a mission, players can use `hint`, `objective`, `status`, `restart`, `quit`, and `?` for the expanded control guide. They can navigate with `map`, `world NUMBER`, `list`, `play MISSION`, `next`, and `previous`, optionally prefixed with `opsquest`.

Bare `opsquest play` continues through incomplete Linux missions after a success. `opsquest play --world NUMBER` follows one world's incomplete stages and stops at its boundary. `opsquest play NUMBER_OR_ID` runs only the selected mission, including an explicitly selected Docker lab. `opsquest map --track docker` discovers the optional Docker track, and `--ids` reveals stable mission IDs. A fresh Linux profile receives a concise quick start once before its first mission; an additive profile marker prevents repetition after an immediate quit, while `opsquest guide` shows the comprehensive version later.

## Teaching shell

The current command subset is:

```text
awk basename cat cd chmod chown clear cp cut dirname du echo env export find
grep gzip gunzip head help history kill less ls man mkdir mv printf ps pwd rm
rmdir sed sh sort stat tail tar touch tr uniq vi wc whoami
```

The parser supports:

- Unquoted whitespace-separated words
- Single and double quotes
- Escaped characters
- `$NAME` and `${NAME}` expansion
- Virtual path globbing
- Comments beginning with `#` at a token boundary
- Pipelines with `|`
- Virtual input, output, and append redirection with `<`, `>`, and `>>`

Unknown commands fail. There is no fallback to a host executable.

Navigation keeps the sandbox working directory and `PWD` synchronized. `OLDPWD` is the single source of truth for `cd -`; scripts snapshot and restore it with the rest of their child-shell environment.

## Interactive terminal and vi

The mission prompt supports Tab completion against the virtual filesystem, Up/Down history, arrows, Home/End, common word movement, Backspace, forward Delete, and bracketed-paste isolation.

Decorative color automatically enables on an interactive output terminal. Piped and redirected output remains plain, and the presence of the `NO_COLOR` environment variable disables color. Semantic roles distinguish page headers, section labels, worlds, objectives, rewards, achievements, and progress; red is reserved for actual failures rather than advanced difficulty. Color belongs to the presentation layer: sandbox command results and mission validation continue to use unstyled text.

`vi FILE` opens one virtual UTF-8 text file up to 256 KiB. Its deliberate teaching subset includes:

- Normal and Insert modes
- `h`, `j`, `k`, `l`, and arrow movement
- `i`, `x`, and `dd`
- Text insertion, newline, Backspace, and Delete
- `:w`, `:q`, `:wq`, and `:q!`

It does not provide plugins, registers, search, shell escapes, multiple buffers, or the rest of Vim. Writes go only to the virtual filesystem.

## Safe shell-script runner

Players can write a file with `vi`, redirection, or other virtual commands and run it with:

```console
opsquest:/workspace$ sh report.sh
```

They can also run it directly:

```console
opsquest:/workspace$ chmod 750 report.sh
opsquest:/workspace$ ./report.sh
```

`sh FILE` accepts exactly one virtual file and does not require executable permission. Direct path execution requires at least one executable mode bit and a first line of either `#!/bin/sh` or `#!/usr/bin/env sh`.

Each non-empty, non-comment line returns through the same OpsQuest lexer, parser, dispatcher, and virtual filesystem as an interactive command. Existing quoting, environment expansion, globbing, pipelines, and redirection therefore behave consistently. CRLF input is accepted.

Scripts have child-shell state: `cd` and `export` affect subsequent lines in that script but the prior working directory and environment are restored when it returns. Virtual file, archive, and process changes persist. Execution stops at the first error and reports the virtual script path and line number.

The runner is deliberately bounded:

- 64 KiB per script
- 8 KiB per source line
- A maximum nesting depth of eight scripts
- 256 dispatched commands across one invocation
- 1 MiB of collected output per script
- Direct and indirect recursion rejection

The shared teaching shell limits each command line to 64 KiB. After variable and glob expansion, one line may contain at most 2 MiB of token text and 4,096 arguments. A pipeline may contain at most 64 stages, and one top-level execution may dispatch at most 512 commands across pipelines, `find -exec`, and nested scripts.

Each virtual file and command result is capped at 2 MiB; aggregate virtual-filesystem content and logical archive payload are capped at 8 MiB each. Virtual paths are limited to 4,096 bytes, and the filesystem and archive metadata may each contain at most 4,096 entries. The virtual environment is capped at 256 entries and 256 KiB. Expansion and pipeline limits are checked before any stage mutates state. Writes, appends, recursive directory creation and copies, environment updates, and archive creation and copying preflight their affected state; archive extraction applies its filesystem and metadata changes transactionally only after every entry succeeds.

The following are intentionally unsupported:

- `sh -c`, other flags, stdin-fed source, or positional arguments
- Standalone variable assignments; use `export NAME=value`
- Loops, conditionals, case statements, functions, and sourced files
- `;`, `&&`, `||`, subshells, and background jobs
- Backtick or `$()` command substitution
- Positional and special parameters such as `$1` and `$?`
- External binaries, host shell escapes, and interactive `vi` inside a script
- `tar -C` during archive creation or listing; destination changes are extraction-only

Script output may feed a later pipeline stage or be redirected to a virtual file. Feeding pipeline or redirected input into a script is rejected because this teaching model does not emulate one shared script stdin stream.

## Automation Shift world

Missions 18–20 turn the existing editor and runner into a short Linux curriculum:

- **Modal First Aid** introduces Normal mode, `dd`, and `:wq` by removing a bad line while preserving the rest of a configuration file.
- **Report on Repeat** has the player repair a pipeline script, set its executable mode, and generate a sorted unique incident report.
- **Boss Battle: Scope Creep** combines editing, direct script execution, pipelines, redirection, `cd`, and `export`. Its outcomes prove that file changes persist while the caller's directory and environment are restored.

Every mission supplies progressive tool-oriented hints. Completion remains outcome-based: the canonical solutions exercise non-interactive test paths, while focused tests also prove valid `vi`, `sh FILE`, and direct-execution alternatives and reject incomplete or state-leaking attempts.

## Docker Foundations vertical slice

Mission 17, **Container Census**, begins the optional **It Works on My Machine** track. The player inspects two attempt-owned containers, identifies the stopped `api` container, and gets both original containers running while keeping the attempt at exactly two containers. Three progressive hints introduce the relevant inspection tool, the option that includes stopped containers, and finally the `docker start` syntax.

Docker remains an optional capability. The Linux track, profile loading, normal quality gate, and bare `opsquest play` do not require a Docker CLI or daemon. `opsquest doctor` distinguishes an unavailable CLI/daemon from a missing pinned fixture image and gives an explicit preparation command. OpsQuest does not pull images automatically.

The first supported teaching subset is deliberately narrow: `docker ps`, `docker container ls`, `start`, `restart`, and `inspect`, plus lab help. Player text is parsed into typed actions before OpsQuest constructs fixed Docker CLI arguments; the raw line is never given to a host shell or unrestricted Docker invocation.

Each attempt maps stable logical aliases such as `api` to generated engine names and exact returned container IDs. A mission may declare at most 16 image aliases and 32 containers. Resources are labeled with the mission and a cryptographically random session identifier, run without a network as a non-root user with a read-only filesystem, dropped capabilities, `no-new-privileges`, and bounded CPU, memory, process, and file-descriptor limits. Cleanup verifies ownership labels before removing exact IDs and is idempotent across completion, quit, restart, switching, setup failure, Ctrl-C cancellation, and ordinary errors. Once cleanup begins, the lab rejects further commands; if inspection or removal fails, unresolved owned containers remain pending so a later close retries them, and the attempt is marked closed only after cleanup succeeds. Labs never request privileged mode, host bind mounts, host networking, host PID/IPC namespace sharing, devices, or a mounted Docker socket.

Docker mission validation observes only resources labeled for the current attempt. Container Census requires both original tracked containers to be running and queries ownership labels to require exactly two attempt containers, so an additional attempt-owned replacement cannot satisfy the objective. Existing profile version 2 remains compatible: mission completion maps accept the stable IDs, while Linux percentages and new Linux Completionist unlocks now consider all 19 Linux missions. Achievements are monotonic, so a Completionist achievement earned before the curriculum expanded remains unlocked.

## Mission format

Mission content lives in `internal/mission/data/*.json` and is embedded into the binary. Decoding rejects unknown fields. Catalog construction validates identifiers, contiguous numbering, track-local world contiguity, supported difficulties, suggested command names, one-to-five hint counts, paths, modes, setup conflicts, archive traversal, duplicate PIDs, rewards, validation field shapes, and bounded Docker fixtures. World/stage placement is derived from ordered campaigns without changing the mission or profile schema. Lookups return deep copies so runtime adapters cannot mutate embedded content.

Each mission has this conceptual shape:

```text
Mission
├── ID, number, title, campaign, and difficulty
├── Story, objective, suggested commands, hints, and explanation
├── Start directory and declarative setup
├── One or more observable validation conditions
└── XP and hint-penalty rewards
```

Mission content remains declarative. Parser, command, filesystem, profile, and validator behavior belongs in Go rather than mission JSON.

## Repository architecture

```text
cmd/opsquest/       Composition root and executable entry point
internal/buildinfo/ Release Please-managed executable version
internal/cli/       Top-level commands, flags, help, and adapter-independent presentation
internal/ui/        Terminal-aware ANSI styles and color policy
internal/game/      Sessions, rewards, terminal input, vi, and outcome validation
internal/mission/   Mission contracts, immutable catalog views, and embedded JSON data
internal/profile/   Versioned progress model and atomic JSON persistence
internal/sandbox/   Parser, dispatcher, virtual state, commands, and script runner
internal/dockerlab/ Optional typed Docker adapter and attempt-owned resource lifecycle
scripts/            Deterministic checks shared by local development and CI
```

The executable constructs the catalog, profile store, and combined environment factory, then injects them into the CLI. The environment interface is the gameplay boundary. The simulated implementation owns virtual paths, environment variables, processes, archive metadata, command history, nested command tracing, and resource limits; the optional Docker implementation owns only its labeled, attempt-scoped containers. Catalog lookups return deep copies so adapters cannot mutate embedded mission definitions.

## Safety and compatibility invariants

- Never pass player input or script text to host `sh`, `bash`, `os/exec`, or another process.
- Never resolve a virtual path against the host filesystem.
- Docker player input must be parsed into supported typed actions; only internally constructed Docker arguments may target exact, label-verified attempt resources.
- Scripts may compose only commands already whitelisted by the teaching shell.
- Unknown or unsupported behavior must fail clearly instead of falling back or being misleadingly approximated.
- Mission validation checks observable outcomes, not one canonical solution.
- Mission definitions remain declarative and strictly validated.
- Mission-schema and profile-format changes are compatibility-sensitive.
- Existing parser, persistence, validation, and isolation guarantees must not be weakened for new content.

## Development and quality gate

Codex reads `AGENTS.md`. Repository-specific workflows live under `.agents/skills` for mission work, sandbox-command extensions, and release-sized iterations.

Common validation commands are:

```console
$ make test
$ make validate-missions
$ make check-agent-docs
$ make vet
$ make build
$ make smoke-test
$ make docker-integration
$ make release-check
$ make release-snapshot
$ make race
$ make check
$ make check-all
```

`make check-all` is the comprehensive Docker-independent gate. It validates agent documentation, runs all Go tests and embedded mission integrity checks through fake Docker contracts, vets, builds, executes an isolated CLI smoke test, and runs race detection. GitHub Actions invokes the same target. `make docker-integration` is an explicit, separately gated lifecycle test for a development machine with Docker and the pinned fixture image. GoReleaser v2 provides the separate `release-check` and non-publishing `release-snapshot` targets.

Release Please maintains the changelog, semantic version, release pull request, `vX.Y.Z` tag, and GitHub release from Conventional Commits. After a release is created, GoReleaser attaches cross-platform archives and SHA-256 checksums in the same workflow. The repository-managed CodeQL advanced workflow analyzes the Go build on pull requests, `main`, a weekly schedule, and manual dispatch.

## Roadmap

The first **v0.3 Docker Foundations** vertical slice is implemented. The next Docker increments are:

- Additional disposable, isolated Docker missions
- Images versus containers beyond the initial lifecycle lab
- `run`, `logs`, and `exec`
- Port mappings, volumes, and environment variables
- Dockerfiles, networking, and Compose
- Outcome-based container troubleshooting missions

Kubernetes remains a later campaign using an isolated local cluster. External mission packs, streaks, efficiency medals, and generated daily incidents are possible future additions, not current behavior.

The Docker and Kubernetes tracks must not weaken the safe in-memory Linux campaign or cause player commands to run against the host by default.
