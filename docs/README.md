---
description: Learn Linux and container operations through safe, story-driven terminal missions.
audience: players and contributors
status: current
---

# OpsQuest

OpsQuest is **Duolingo meets a terminal sandbox**: a command-line game where you learn Linux and container operations by resolving fictional production incidents.

```console
$ go run ./cmd/opsquest play
```

The game describes an observable objective, gives you a disposable environment, and accepts any supported command sequence that produces the right result. Linux commands operate only on in-memory state; optional Docker missions use a deliberately small, attempt-scoped command subset.

## Choose your route

| I want to… | Start here |
| --- | --- |
| Install the game and begin a mission | [Quick start](play/quick-start.md) |
| Move mission guidance into a local browser | [Web mission companion](play/web-companion.md) |
| Understand objectives, hints, routes, and rewards | [Missions, hints, and progress](play/missions-and-progress.md) |
| Look up terminal controls and supported commands | [Controls and commands](play/controls-and-commands.md) |
| See what every world and mission teaches | [Mission map](play/mission-map.md) |
| Understand the implementation | [Technical architecture](technical/architecture.md) |
| Review the isolation guarantees | [Sandbox and safety](technical/sandbox-and-safety.md) |
| Contribute a change | [Contributing and quality gates](technical/contributing.md) |
| Explore planned improvements | [Roadmap](roadmap/README.md) |

## Current scope

- 23 Linux missions across four ordered learning worlds
- 6 optional Docker Foundations missions
- Outcome-based validation rather than one required command transcript
- An in-memory filesystem, process table, environment, archives, editor, and shell scripts
- Persistent XP, ranks, command practice, hints, completions, and achievements
- An optional local web companion with live objective progress
- Interactive completion, editing, history, and mission navigation

## The safety promise

Player-entered Linux commands are parsed and executed by OpsQuest's Go teaching shell. They never reach a host shell or host path. Docker gameplay is opt-in, accepts only typed teaching actions, and operates on exact disposable resources labeled for the current attempt.

The optional browser companion is a loopback-only read model. It receives
sanitized mission snapshots but cannot execute commands or declare success.

Read [Sandbox and safety](technical/sandbox-and-safety.md) for the complete trust model and limits.

## About these docs

This site is built from Markdown and diagram sources reviewed with the code.
Player guidance, current design, and contributor reference are public here.
Forward-looking work is explicitly labeled in the roadmap. Internal decisions,
infrastructure runbooks, and iteration evidence remain in the repository rather
than the public site.
