Implemented the first Docker Foundations vertical slice, including Mission 17 with progressive tool hints.

Key additions:

- Generic mission environment boundary while preserving all Linux gameplay.
- Safe Docker adapter using typed commands and fixed arguments—never a host shell.
- Disposable, resource-limited, label-owned containers with verified cleanup.
- Docker detection through `doctor`.
- Docker track discovery via `list --track docker`.
- Mission 17: **Container Census**, with hints covering container lifecycle, `docker ps -a`, and `docker start api`.
- Outcome validation checks both original containers and the attempt-owned container count.
- Linux Completionist and existing profile version remain compatible.
- Signal-aware cleanup, ambiguous-create recovery, and an optional real-engine integration target.

Primary implementation: [environment.go](/Users/aleksandergregersen/privat/go-cli-tool/internal/game/environment.go:49), [Docker environment](/Users/aleksandergregersen/privat/go-cli-tool/internal/dockerlab/environment.go:206), [Mission 17](/Users/aleksandergregersen/privat/go-cli-tool/internal/mission/data/17-container-census.json:6), and [current implementation brief](/Users/aleksandergregersen/privat/go-cli-tool/initial_prompt.md:168).

Validation passed:

- Focused CLI, game, mission, and Docker-lab tests.
- `make validate-missions`
- `make check-agent-docs`
- `make smoke-test`
- Full `make check-all`, including tests, vet, build, smoke test, and race detection.
- `git diff --check`

The real `make docker-integration` test was not run because `docker` is not installed in the current PATH.

Remaining risk: SIGKILL or a machine crash can still leave labeled containers; a future stale-resource reaper would improve recovery.

Repository status: `feature/docker-integration` is at `7cd1d0c` and aligned with origin. The subsequent safety-review hardening remains uncommitted across 15 modified task-related files, with no untracked files.
