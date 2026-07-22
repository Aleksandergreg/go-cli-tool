---
description: Install OpsQuest, launch the Linux campaign, and discover its main commands.
audience: players
status: current
---

# Quick start

OpsQuest requires Go 1.22 or newer. Clone the repository, then start the recommended Linux route:

```console
$ go run ./cmd/opsquest play
```

The first Linux launch shows a concise guide before opening the first incomplete mission. Replay that guide at any time:

```console
$ go run ./cmd/opsquest guide
```

## Build a reusable binary

```console
$ make build
$ ./bin/opsquest play
```

You can also install from the checkout into your Go binary directory:

```console
$ go install ./cmd/opsquest
$ opsquest play
```

## Useful commands

```console
$ opsquest guide                 # replay onboarding
$ opsquest map                   # view worlds, stages, and progress
$ opsquest map --ids             # include stable mission IDs
$ opsquest play --world 2        # stay within Linux World 2
$ opsquest play 4                # begin at global Mission 4
$ opsquest play --once 4         # return after one mission
$ opsquest show 16               # preview without starting
$ opsquest profile               # view XP, rank, and progress
$ opsquest commands              # view practiced commands
$ opsquest achievements          # view achievement progress
$ opsquest doctor                # check catalog, profile, and Docker readiness
```

`opsquest list` is an alias for `opsquest map` and supports the same filters.

## Next steps

- [How missions work](how-missions-work.md)
- [Controls and commands](controls-and-commands.md)
- [Linux worlds](linux-worlds.md)
- [Optional Docker setup](docker-foundations.md)
