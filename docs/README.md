---
description: Learn Linux and container operations through safe, story-driven terminal missions.
audience: players and contributors
status: current
---

# OpsQuest documentation

OpsQuest is **Duolingo meets a terminal sandbox**: a command-line game where you learn Linux and container operations by resolving fictional production incidents.

```console
$ go run ./cmd/opsquest play
```

The game describes an observable objective, gives you a disposable environment, and accepts any supported command sequence that produces the right result. Linux commands operate only on in-memory state; optional Docker missions use a deliberately small, attempt-scoped command subset.

## Choose your route

| I want to… | Start here |
| --- | --- |
| Install the game and begin a mission | [Quick start](play/quick-start.md) |
| Understand objectives, hints, and navigation | [How missions work](play/how-missions-work.md) |
| Look up terminal controls and supported commands | [Controls and commands](play/controls-and-commands.md) |
| See what every world and mission teaches | [Curriculum and mission map](game/curriculum.md) |
| Understand the implementation | [Technical architecture](technical/architecture.md) |
| Review the isolation guarantees | [Sandbox and safety](technical/sandbox-and-safety.md) |
| Explore planned improvements | [Roadmap](roadmap/README.md) |

## Current scope

- 23 Linux missions across four ordered learning worlds
- Six optional Docker Foundations missions
- Outcome-based validation rather than one required command transcript
- An in-memory filesystem, process table, environment, archives, editor, and shell scripts
- Persistent XP, ranks, command practice, hints, completions, and achievements
- Interactive completion, editing, history, and mission navigation

## The safety promise

Player-entered Linux commands are parsed and executed by OpsQuest's Go teaching shell. They never reach a host shell or host path. Docker gameplay is opt-in, accepts only typed teaching actions, and operates on exact disposable resources labeled for the current attempt.

Read [Sandbox and safety](technical/sandbox-and-safety.md) for the complete trust model and limits.

## About these docs

This site is built from the same Markdown and diagram sources reviewed with the code. Current behavior lives under `play/`, `game/`, and `technical/`; proposals live under `roadmap/`; point-in-time delivery evidence lives under `history/`.

Contributors should start with [Contributing and quality gates](technical/contributing.md).
