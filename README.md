# OpsQuest

OpsQuest is **Duolingo meets a terminal sandbox**: a Go CLI game that teaches Linux and container operations through short, story-driven missions.

```console
$ opsquest play --once 5

MISSION 05: The Missing Log File
================================
Linux · World 1/4: First Day · Stage 5/6
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

- 23 Linux missions across four ordered learning worlds
- 6 optional, disposable Docker Foundations missions
- An isolated in-memory filesystem, environment, process table, and archives
- Quote-aware globs and variables, pipelines, redirection, and command history
- A compact virtual `vi` and bounded virtual shell scripts
- Progressive hints, free command guidance, XP, ranks, and six achievements
- An optional loopback-only web companion for mission guidance and live progress
- World maps, direct mission practice, replay, and persistent profile progress
- Outcome validation for output, paths, content, permissions, processes, environment, and Docker state

## Quick start

OpsQuest requires Go 1.26 or newer when building from source.

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
$ opsquest play --web
$ opsquest profile
$ opsquest achievements
$ opsquest doctor
```

The complete player guide covers [installation and first play](docs/play/quick-start.md),
[the local web companion](docs/play/web-companion.md),
[missions and progress](docs/play/missions-and-progress.md), and
[controls and commands](docs/play/controls-and-commands.md).

## Optional Docker Foundations

Linux gameplay never requires a container engine. Docker Foundations works with Docker Engine, Docker Desktop, or OrbStack through the Docker CLI. Start your chosen engine and explicitly fetch the pinned fixture:

```console
$ docker pull docker.io/library/busybox@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662
$ opsquest doctor
$ opsquest play 20
```

OpsQuest never pulls the image automatically. See
[Docker Foundations](docs/play/docker-foundations.md) for Docker Desktop,
Docker Engine, and OrbStack setup, plus the teaching subset and isolation
boundary.

## Safety model

Player-entered Linux commands—including virtual scripts—are parsed and executed by OpsQuest. They never reach a host shell, host process, or host path. The teaching shell operates only on bounded in-memory state.

Docker input is parsed into a small set of typed teaching actions. OpsQuest constructs Docker arguments itself, uses only exact resources labeled for the current attempt, applies resource restrictions, and verifies ownership again during cleanup. It does not expose arbitrary Docker passthrough, privileged mode, host mounts, host networking, devices, or the Docker socket.

The optional web companion binds only to an ephemeral `127.0.0.1` port, uses a one-time pairing URL, and exposes a read-only mission projection. It cannot submit commands, mutate the profile or attempt, address Docker resources, or approve completion.

Read [Sandbox and safety](docs/technical/sandbox-and-safety.md) for trust boundaries, quotas, cleanup behavior, and threat controls.

## Documentation

The hosted site is published at [aleksandergreg.github.io/go-cli-tool](https://aleksandergreg.github.io/go-cli-tool/). Its source is organized by purpose:

- [Play OpsQuest](docs/play/README.md) — player setup, missions, controls, worlds, and Docker
- [How OpsQuest works](docs/game/README.md) — learning philosophy, architecture, and safety
- [Contribute](docs/technical/README.md) — quality gates, mission authoring, and compatibility
- [Roadmap](docs/roadmap/README.md) — explicitly proposed, unshipped work

Point-in-time iteration evidence stays in the repository-only
[delivery history](project/history/README.md).

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
internal/webapp/    Loopback-only mission companion and embedded browser UI
docs/               Zensical documentation source and diagrams
project/            Repository-only decisions and delivery history
```

## Development

The repository [agent guide](AGENTS.md) defines safety invariants, package boundaries, and the definition of done. The Makefile is the local validation interface:

```console
$ make check             # tests, mission validation, vet, build, and smoke test
$ make check-all         # normal gate plus race detection
$ make check-docs        # dependency-free documentation consistency checks
$ make docker-integration
$ make orbstack-integration
$ make docs-check        # strict Zensical build and link validation
$ make tofu-check        # GitHub governance formatting and schema validation
$ make release-check
$ make release-snapshot
```

For documentation development, install the pinned toolchain from `requirements-docs.txt` in a Python 3.10+ virtual environment, then use `make docs-serve`.

Task-specific Codex workflows live in [`.agents/skills`](.agents/skills): `$add-mission`, `$extend-sandbox-command`, and `$prepare-iteration`.

## Releases and security

Release Please maintains semantic-version release pull requests from
Conventional Commits. Merging a release pull request creates the tag and GitHub
release; GoReleaser attaches checksummed macOS, Linux, and Windows archives.
Repository-managed CodeQL analyzes the real Go build path. GitHub settings and
rulesets are declared under [`infra/github`](infra/github); its
[governance runbook](infra/github/GOVERNANCE.md) defines the credential, state,
and reviewed-apply boundaries.

See [CHANGELOG.md](CHANGELOG.md), [GitHub Releases](https://github.com/Aleksandergreg/go-cli-tool/releases),
and [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

## Roadmap

Docker campaign increments, distribution improvements, and longer-term ideas
are tracked in the [roadmap](docs/roadmap/README.md). Kubernetes remains future
scope.

## License

OpsQuest is distributed under the [Beer-Ware License](LICENSE).
