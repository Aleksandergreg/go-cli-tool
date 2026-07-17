# OpsQuest

OpsQuest is **Duolingo meets a terminal sandbox**: a command-line game that teaches Linux through short, story-driven operations missions.

```console
$ opsquest play

MISSION 04: The Missing Log File
================================

The web server is failing, and the monitoring dashboard has chosen this
moment to become an abstract art installation.

Find every file ending in .log inside /var/app that contains the word ERROR.

opsquest:/var/app$ find . -name "*.log" -exec grep -l "ERROR" {} \;
./api/app.log
./worker/worker.log

✓ Mission complete!
+75 XP
New command discovered: find
```

The game validates the result, not a prescribed command. For example, the file-moving mission accepts `mv` or an equivalent `cp` followed by `rm`.

## What is included

Version 0.1 is a complete Linux foundations campaign:

- 10 hand-written missions across two story chapters
- An isolated, in-memory filesystem and process table
- Individual commands, pipelines, quoting, variables, and redirection
- Outcome-based validation for output, files, permissions, processes, archives, and environment variables
- Hints with XP penalties
- Persistent XP, ranks, mission completion, and command mastery
- Replayable missions without duplicate XP
- `play`, `list`, `profile`, `commands`, and `reset` commands

The sandbox implements a focused teaching subset of:

```text
awk cat cd chmod chown cp cut echo env export find grep gzip gunzip
head kill less ls mkdir mv printf ps pwd rm rmdir sort tail tar touch
uniq wc whoami
```

It also supports pipelines (`|`) and input/output redirection (`<`, `>`, `>>`).

## Quick start

OpsQuest requires Go 1.22 or newer and has no third-party dependencies.

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
$ opsquest profile
$ opsquest commands
$ opsquest reset
```

Inside a mission, use `hint`, `objective`, or `quit`. Type `help` to see the available lab commands.

## Safety model

Player input is parsed by OpsQuest itself. It is never passed to `sh`, `bash`, or another host process, and paths only address the mission's in-memory filesystem. Unsupported commands return a teaching-shell error.

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

Mission content lives in [`internal/mission/data`](internal/mission/data). JSON keeps v0.1 dependency-free while retaining the proposed declarative setup/validation design. A future external mission-pack loader can add YAML support without coupling content to the game engine.

Each mission defines:

```text
story + objective + starting directory
setup: directories, files, processes, environment, archives
hints + explanation + rewards
validation: one or more observable outcome conditions
```

## Development

```console
$ make test
$ make vet
```

The tests run canonical solutions for all ten missions, exercise alternative outcome-based solutions, cover profile persistence and CLI behavior, and verify that attempts to invoke a host shell or remove the virtual root are rejected.

## Roadmap

- v0.2: disposable Docker labs, container troubleshooting, ports, volumes, builds, networking, and Compose
- Later: Kubernetes missions backed by an isolated local cluster
- Mission packs, achievements, streaks, boss incidents, and generated daily challenges
