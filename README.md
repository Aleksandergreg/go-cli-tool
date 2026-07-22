# OpsQuest

OpsQuest is **Duolingo meets a terminal sandbox**: a Go CLI game that teaches Linux and container operations through short, story-driven missions.

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
```

OpsQuest validates the result, not a prescribed command. Equivalent supported solutions pass when they produce the same observable outcome.

## What is included

- 19 Linux missions across four ordered learning worlds
- One optional, disposable Docker Foundations mission
- An isolated in-memory filesystem, environment, process table, and archives
- Quote-aware globs and variables, pipelines, redirection, and command history
- A compact virtual `vi` and bounded virtual shell scripts
- Progressive hints, free command guidance, XP, ranks, and six achievements
- World maps, direct mission practice, replay, and persistent profile progress
- Outcome validation for output, paths, content, permissions, processes, environment, and Docker state

## Quick start

OpsQuest requires Go 1.22 or newer.

```console
$ go run ./cmd/opsquest play
```

Build or install from the checkout:

```console
$ make build
$ ./bin/opsquest play

$ go install ./cmd/opsquest
$ opsquest play
```

Useful entry points:

```console
$ opsquest guide
$ opsquest map
$ opsquest play --world 2
$ opsquest play --once 4
$ opsquest profile
$ opsquest achievements
$ opsquest doctor
```

The complete player guide covers [installation and first play](docs/play/quick-start.md), [mission behavior](docs/play/how-missions-work.md), and [controls and commands](docs/play/controls-and-commands.md).

## Optional Docker Foundations

Linux gameplay never requires Docker. To prepare the first Docker mission, start Docker and explicitly fetch its pinned fixture:

```console
$ docker pull docker.io/library/busybox@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662
$ opsquest doctor
$ opsquest play 17
```

OpsQuest never pulls the image automatically. See [Docker Foundations](docs/play/docker-foundations.md) for the teaching subset and isolation boundary.

## Safety model

Player-entered Linux commands—including virtual scripts—are parsed and executed by OpsQuest. They never reach a host shell, host process, or host path. The teaching shell operates only on bounded in-memory state.

Docker input is parsed into a small set of typed teaching actions. OpsQuest constructs Docker arguments itself, uses only exact resources labeled for the current attempt, applies resource restrictions, and verifies ownership again during cleanup. It does not expose arbitrary Docker passthrough, privileged mode, host mounts, host networking, devices, or the Docker socket.

Read [Sandbox and safety](docs/technical/sandbox-and-safety.md) for trust boundaries, quotas, cleanup behavior, and threat controls.

## Documentation

The hosted site is published at [aleksandergreg.github.io/go-cli-tool](https://aleksandergreg.github.io/go-cli-tool/). Its source is organized by purpose:

- [Play OpsQuest](docs/play/README.md) — player setup, missions, controls, worlds, and Docker
- [Game and learning](docs/game/README.md) — learning philosophy, curriculum, mission design, and progression
- [Technical](docs/technical/README.md) — architecture, safety, mission content, profiles, and contribution workflows
- [Roadmap](docs/roadmap/README.md) — delivered foundations and proposed improvements
- [Delivery history](docs/history/README.md) — point-in-time iteration evidence

## Project structure

```text
cmd/opsquest/       Composition root and executable entry point
internal/buildinfo/ Release Please-managed executable version
internal/cli/       Top-level commands, routes, and presentation
internal/ui/        Terminal-aware ANSI styles and color policy
internal/game/      Mission sessions, input/editor integration, and validation
internal/mission/   Strict embedded catalog, worlds, and mission JSON
internal/profile/   XP, ranks, achievements, and atomic persistence
internal/sandbox/   Virtual filesystem, shell parser, and commands
internal/dockerlab/ Optional, label-scoped Docker adapter
docs/               Zensical documentation source and diagrams
```

## Development

The repository [agent guide](AGENTS.md) defines safety invariants, package boundaries, and the definition of done. The Makefile is the local validation interface:

```console
$ make check             # tests, mission validation, vet, build, and smoke test
$ make check-all         # normal gate plus race detection
$ make docker-integration
$ make docs-check        # strict Zensical build and link validation
$ make release-check
$ make release-snapshot
```

For documentation development, install the pinned toolchain from `requirements-docs.txt` in a Python 3.10+ virtual environment, then use `make docs-serve`.

Task-specific Codex workflows live in [`.agents/skills`](.agents/skills): `$add-mission`, `$extend-sandbox-command`, and `$prepare-iteration`.

## Releases and security

Release Please maintains semantic-version release pull requests from Conventional Commits. Merging a release pull request creates the tag and GitHub release; GoReleaser attaches checksummed macOS, Linux, and Windows archives. Repository-managed CodeQL analyzes the real Go build path.

See [CHANGELOG.md](CHANGELOG.md), [GitHub Releases](https://github.com/Aleksandergreg/go-cli-tool/releases), and the [CI/CD roadmap](docs/roadmap/ci-cd.md). Third-party module licenses are recorded in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

## Roadmap

Docker campaign increments, distribution improvements, and longer-term ideas are tracked in the [project roadmap](docs/roadmap/README.md). Kubernetes remains future scope.

## License

OpsQuest is distributed under the [Beer-Ware License](LICENSE).
