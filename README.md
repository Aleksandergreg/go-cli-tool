# OpsQuest

OpsQuest is **Duolingo meets a terminal sandbox**: a command-line game that teaches Linux and container operations through short, story-driven missions.

```console
$ opsquest play --once 4

MISSION 04: The Missing Log File
================================
Linux · World 1/4: First Day · Stage 4/5
Difficulty: beginner · Reward: 75 XP

INCIDENT
The web server is failing, and the monitoring dashboard has chosen this
moment to become an abstract art installation.

OBJECTIVE
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

Version 0.3 starts Docker Foundations while preserving the complete Linux campaign:

- 19 hand-written Linux missions across four ordered learning worlds
- One optional, disposable Docker mission in **It Works on My Machine**
- An isolated, in-memory filesystem and process table
- Interactive line editing with Tab completion and Up/Down command recall
- A first-play guide covering objectives, hints, progression, navigation, and sandbox safety
- World/stage maps, soft world jumping, and continuous play with clear transition feedback
- Quote-aware globbing, pipelines, stage-local redirection, variables, and command history
- Safe virtual shell scripts with bounded nesting and line-numbered errors
- Outcome-based validation for output, files, permissions, processes, archives, and environment variables
- Progressive hints that introduce relevant tools and syntax, with persistent XP penalties
- A free Bandit-style command guide for every mission, separate from XP-costing hints
- Mission status and environment restart controls
- Persistent XP, ranks, mission completion, command mastery, and six learning achievements
- Replayable missions without duplicate XP
- Mission previews, campaign filters, profile naming, and built-in diagnostics
- Multi-step boss incidents in **Production Friday** and **The Automation Shift**

The sandbox implements a focused teaching subset of:

```text
awk basename cat cd chmod chown clear cp cut dirname du echo env export find
grep gzip gunzip head help history kill less ls man mkdir mv printf ps pwd rm
rmdir sed sh sort stat tail tar touch tr uniq vi wc whoami
```

It also supports pipelines (`|`) and input/output redirection (`<`, `>`, `>>`).

## Quick start

OpsQuest requires Go 1.22 or newer. It uses the Go project's small `x/term` module for portable interactive line editing. Linux gameplay remains local and in memory; the optional Docker lab uses only explicitly prepared, disposable containers.

Color automatically appears when output is connected to an interactive terminal. Page headings, world/stage context, objectives, progress, rewards, and achievements use distinct semantic roles; red is reserved for actual failures. Piped or redirected output stays plain for scripts and logs; set `NO_COLOR=1` to disable decorative color explicitly.

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

To prepare the optional first Docker lab, install and start Docker, then explicitly fetch its pinned fixture image:

```console
$ docker pull docker.io/library/busybox@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662
$ opsquest doctor
$ opsquest play 17
```

Useful commands:

```console
$ opsquest guide
$ opsquest map
$ opsquest map --ids
$ opsquest map --track docker
$ opsquest play --world 2
$ opsquest list # alias for map; supports the same filters
$ opsquest list --track docker
$ opsquest play 4
$ opsquest play --once 4
$ opsquest play 17
$ opsquest play 18
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

Inside a mission, use `hint`, `objective`, `status`, `restart`, or `quit`. Type `help` to list lab commands and, where available, `help COMMAND` for focused examples.

A fresh Linux profile sees a concise quick start once before its first mission. `opsquest guide` provides the longer explanation whenever it is useful and records that onboarding has been viewed. Mission maps use compact numbered rows by default; add `--ids` when stable mission IDs are needed.

You can navigate without leaving the mission prompt:

```console
opsquest:/backups$ list --completed
opsquest:/backups$ map
opsquest:/backups$ world 2
opsquest:/srv/release$ play 3
opsquest:/home/operator$ next
opsquest:/backups$ previous
```

The `opsquest` prefix is optional inside a mission, so `opsquest map` and `opsquest play 3` also work. An in-mission number selects that stage in the current world: from World 2, `play 3` opens Mission 08, not global Mission 03. Use a stable mission ID from `list --ids` to jump across worlds. Listing preserves the current sandbox; switching missions or worlds starts the selected stage with a fresh sandbox while keeping persistent profile progress. Type `?` for the full mission and line-editing guide.

Interactive editing supports Left/Right cursor movement, Up/Down history, Home/End and Ctrl-A/E line boundaries, Option/Ctrl-Left/Right word movement, Tab completion, Backspace, Delete, and Ctrl-W. Command-arrow sequences are treated as Home/End when the terminal reports the modifier.

The teaching shell also includes a compact, modal `vi` simulator for one virtual file at a time:

```console
opsquest:/workspace$ vi notes.txt
```

In normal mode, use `h`/`j`/`k`/`l` or the arrow keys to move, `i` to enter insert mode, `x` to delete a character, and `dd` to delete a line. Insert mode accepts text, newlines, Backspace, and literal bracketed paste; paste is ignored in normal mode so it cannot become editor commands. Press Esc to return to normal mode. Use `:w` to save, `:q` to quit an unchanged file, `:wq` to save and quit, or `:q!` to discard changes. Writes stay entirely inside the mission's virtual filesystem.

This is deliberately not full Vim: flags, multiple files, search, registers, plugins, shell escapes, external commands, pipelines, and redirection are unsupported. The editor accepts one UTF-8 text file up to 256 KiB; display width for wide or combining Unicode is approximate. Because it needs raw interactive key input, `vi` cannot be launched through redirected or other non-interactive stdin.

### Virtual shell scripts

Scripts are interpreted by the same safe teaching shell as commands entered at the mission prompt:

```console
opsquest:/workspace$ vi report.sh
opsquest:/workspace$ sh report.sh
opsquest:/workspace$ chmod 750 report.sh
opsquest:/workspace$ ./report.sh
```

`sh FILE` accepts one virtual UTF-8 file and does not require executable permission. Direct paths such as `./report.sh` require an executable mode and either `#!/bin/sh` or `#!/usr/bin/env sh`. Blank lines, comments, existing lab commands, exported variables, pipelines, and virtual redirection work normally. Script output can feed a later pipeline stage or be redirected to another virtual file.

Each script runs with child-shell working-directory and environment scope: `cd` and `export` affect later lines in that script but are restored on return, while virtual files, archives, and mission processes keep their resulting state. Execution stops at the first error and reports its virtual filename and line. Source size, line length, nesting, dispatched commands, and output are bounded.

This is not a complete POSIX shell. Options such as `sh -c`, positional arguments, stdin-fed scripts, standalone assignments, loops, conditionals, functions, substitutions, background jobs, external programs, and interactive `vi` calls from a script are rejected rather than approximated. Use `help sh` for the exact limits.

`opsquest play` resumes the first incomplete mission and follows the recommended path. At the top level, a direct mission jump such as `opsquest play 4` uses the stable global mission number or ID and advances sequentially from there. Inside a mission, `play 4` selects Stage 4 of the current world and keeps the route inside that world; a stable ID remains the unambiguous cross-world form. `opsquest play --world 2` and the in-lab `world 2` also stay within World 2 and stop at its boundary. Add `--once` to return after one completed mission. Worlds are guidance rather than locks: every stage remains directly playable and replayable.

The game accepts any supported command sequence that produces the objective's final outcome. After an incomplete command it reports how many checks pass; `status` lists each satisfied and missing outcome without prescribing a command. `restart` rebuilds the mission environment while retaining hint penalties and command mastery.

## Learning worlds

- **World 1 · First Day** (Missions 1–5) — orientation, directories, files, search, and movement
- **World 2 · The Logpocalypse** (Missions 6–10) — permissions and environment first, then processes, archives, pipelines, and an advanced boss
- **World 3 · Production Friday** — logs, aggregation, text transformation, ownership, disk usage, and a multi-fault boss incident
- **World 4 · The Automation Shift** — modal editing, reusable shell scripts, executable modes, and child-shell scope
- **Docker World 1 · It Works on My Machine** — an optional Docker Foundations world beginning with container lifecycle and inspection

`opsquest map` shows world and stage progress without confusing stable global mission references with curriculum position. Completing a world does not lock or hide earlier content. The first two Linux worlds deliberately use five stages each: World 1 stays beginner-focused, while World 2 scales from beginner through intermediate to its advanced boss.

Achievements reward learning behavior rather than decoration: completing a first fix, building a three-command pipeline, practicing ten commands, solving missions without hints, beating an advanced incident, and finishing Linux.

## Safety model

Player input—including `sh` scripts—is parsed by OpsQuest itself. It is never passed to a host `sh`, `bash`, or another host process, and paths only address the mission's in-memory filesystem. Tab completion queries that same virtual filesystem and cannot expose host paths. Unsupported commands return a teaching-shell error.

The simulator also rejects virtual-root/current-directory removal, prevents file/directory type corruption during copies and moves, keeps virtual archive metadata synchronized, and blocks archive entries that try to escape their extraction directory. `OLDPWD` is the single source used by `cd -`, including inside the child scope of a virtual script.

Resource ceilings keep a lab deterministic: command lines are limited to 64 KiB, expanded token text to 2 MiB, expanded arguments to 4,096, pipelines to 64 stages, and one execution to 512 dispatches across pipelines, `find -exec`, and nested scripts. A virtual file and one command's output are limited to 2 MiB; total virtual-file content and logical archive payload are each limited to 8 MiB; virtual paths are limited to 4,096 bytes; and filesystem and archive metadata are each capped at 4,096 entries. The virtual environment is capped at 256 entries and 256 KiB. Expansion, pipeline shape, writes, recursive operations, environment updates, and archive operations preflight their affected state; extraction publishes filesystem and archive metadata together only after every entry succeeds. `tar -C` is intentionally limited to extraction rather than silently approximated for create or list operations.

This makes the introductory Linux experience safe and portable, with two intentional tradeoffs:

- It implements a useful subset of common command behavior rather than every shell feature and flag.
- Mission state exists only for the current attempt; player progress persists separately.

Docker Foundations is opt-in. The Linux campaign never requires Docker, and bare `opsquest play` remains on the Linux track. Run `opsquest doctor` to check the optional Docker CLI, daemon, and fixture image. The first lab uses the pinned Docker Official Image fixture documented above; OpsQuest never pulls it automatically.

Docker commands are parsed into a small teaching subset before any engine call. OpsQuest constructs fixed `docker` CLI arguments itself, assigns unique labels and resource limits, exposes only logical container names, and removes only exact resources owned by the current attempt. Setup is capped at 16 image aliases and 32 containers per mission. Raw player lines are never passed to a host shell or unrestricted Docker invocation. Labs do not use privileged mode, host bind mounts, host networking, devices, or a mounted Docker socket. Cleanup seals the attempt immediately, retains only unresolved owned resources after a failure, and retries those resources on a later close.

## Progress storage

Profiles are stored as `profile.json` in the platform user configuration directory, under `opsquest`. Writes are atomic and use owner-only permissions. The additive onboarding marker prevents the first-run quick start from repeating after an immediate quit. Display names accept up to 40 printable Unicode characters; terminal controls and other non-printable text are rejected, while older unsafe names are normalized when loaded.

Two environment variables are useful for development or portable installs:

- `OPSQUEST_HOME` changes the profile directory.
- `OPSQUEST_PLAYER` sets the display name for a new profile.

## Project structure

```text
cmd/opsquest/       Composition root and executable entry point
internal/buildinfo/ Release Please-managed executable version
internal/cli/       Top-level commands and presentation, independent of environment adapters
internal/ui/        Terminal-aware ANSI styles and color policy
internal/game/      Interactive session, terminal/editor integration, and outcome validation
internal/mission/   Mission contracts, immutable world-aware catalog, and embedded mission data
internal/profile/   XP, ranks, mastery, and atomic JSON persistence
internal/sandbox/   Virtual filesystem, shell parser, and commands
internal/dockerlab/ Optional, label-scoped Docker lab adapter
```

Mission content lives in [`internal/mission/data`](internal/mission/data). JSON keeps the binary dependency-free while retaining the proposed declarative setup/validation design. Mission decoding rejects unknown fields, missing or malformed suggested-command guides, unsafe paths, conflicting setup entries, invalid modes, duplicate PIDs, unknown or malformed validators, unsupported difficulties, hint counts outside one to five, oversized Docker setup, non-contiguous numbering, and a campaign that reappears as two worlds in the same track. World/stage placement is derived from ordered campaigns without changing stable mission IDs or profile data. Catalog results are deep copies, so adapters cannot mutate embedded mission state. A future external mission-pack loader can add YAML support without coupling content to the game engine.

Each mission defines:

```text
story + objective + environment type
setup: simulated state or attempt-scoped Docker fixtures
suggested command names + progressive hints + explanation + rewards
validation: one or more observable outcome conditions
```

### Project documentation

The documentation map starts at [`docs/README.md`](docs/README.md) and separates material by purpose:

- [Game and learning](docs/game/README.md) covers curriculum, missions, worlds, progression, and learning design.
- [Technical](docs/technical/README.md) covers architecture, runtime behavior, sandbox safety, Docker isolation, and their diagrams.
- [Roadmap](docs/roadmap/README.md) contains explicitly labeled proposals and unfinished improvements.
- [Delivery history](docs/history/README.md) preserves numbered iteration reports without treating them as the source of truth for current behavior.

Editable Mermaid and Excalidraw sources live beside their owning section with repository-viewable SVG exports.

## Development

Codex reads [`AGENTS.md`](AGENTS.md) for the repository's scope, safety invariants, package boundaries, and definition of done. Task-specific skills live in [`.agents/skills`](.agents/skills):

- `$add-mission` guides declarative mission content, outcome validation, and solution coverage.
- `$extend-sandbox-command` guides changes to the simulated shell's teaching subset.
- `$prepare-iteration` guides release-sized validation, documentation, and `docs/history/iterations/iteration-NN.md` reporting.

Mention a skill by name in a Codex request (for example, “use `$add-mission`”) or ask for the matching task so Codex can discover it from its trigger description.

The Makefile is the local source of truth for validation:

```console
$ make test
$ make validate-missions
$ make check-agent-docs
$ make smoke-test
$ make docker-integration # requires an available Docker daemon and pinned fixture image
$ make release-check      # requires GoReleaser v2
$ make release-snapshot   # builds local archives in dist/ without publishing
$ make vet
$ make build
$ make race
$ make check
$ make check-all
```

`make check` runs agent-document validation, all Docker-independent Go tests (including embedded mission integrity and canonical solutions), vet, a binary build, and the isolated CLI smoke test. `make check-all` is the complete portable quality gate and adds race detection. GitHub Actions runs that same target for pull requests and pushes to `main` without requiring Docker; superseded runs for the same PR are canceled. `make docker-integration` is the explicit real-engine lifecycle gate for machines that have Docker and the pinned fixture image available. `make release-check` validates `.goreleaser.yml`, while `make release-snapshot` also exercises every release build and archive locally without contacting GitHub.

The tests exercise outcome-based mission solutions, profile compatibility, achievements, persistence, filters, previews, diagnostics, mission controls, parser behavior, archives, the virtual filesystem, and host-isolation invariants. The smoke test builds a temporary binary, uses a temporary `OPSQUEST_HOME`, and removes Docker from its controlled `PATH`, so it neither writes to the developer's real profile nor contacts a local daemon.

Third-party module licenses are recorded in [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).

## Releases and security scanning

Release Please maintains a release pull request from Conventional Commit messages merged to `main`. Use `fix:` for patch changes, `feat:` for minor changes, and a `!` or `BREAKING CHANGE` footer for incompatible changes; before 1.0, breaking changes intentionally bump the minor version. Squash-merge titles should follow the same convention. The release pull request updates [`CHANGELOG.md`](CHANGELOG.md), the manifest, `internal/buildinfo.Version`, and the version reference in [`initial_prompt.md`](initial_prompt.md).

Merging that release pull request creates a `vX.Y.Z` tag and GitHub release. The same workflow then uses GoReleaser to attach SHA-256-checked archives for macOS and Linux on amd64/arm64 and Windows on amd64. Archives contain the executable, README, changelog, Beer-Ware license, and third-party notices. Release Please and GoReleaser share one workflow because GitHub does not start a second workflow for a release created by the default repository token.

The release workflow uses only `GITHUB_TOKEN`; no publishing secret is required. Repository Actions settings must allow workflows to create pull requests. GitHub may require approval before checks run on a release pull request created by that token; use a narrowly scoped GitHub App or fine-grained token only if unattended release-PR checks become necessary. `workflow_dispatch` is available to retry reconciliation, but it does not bypass the release-pull-request process.

The repository-managed CodeQL workflow analyzes Go on pull requests, pushes to `main`, a weekly schedule, and manual dispatch. It uses manual build mode with `make build`. Do not enable GitHub's CodeQL default setup at the same time, because this workflow is the advanced setup and duplicate configurations can block or duplicate uploads.

## Roadmap

- v0.3.x: expand Docker Foundations through logs, environment variables, ports, volumes, builds, networking, and Compose
- Later: Kubernetes missions backed by an isolated local cluster
- External mission packs, streaks, efficiency medals, and generated daily incidents
