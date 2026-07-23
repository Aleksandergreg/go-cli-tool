---
description: Delivered Docker Foundations boundary, beginner campaign, and proposed follow-up capabilities.
audience: players and contributors
status: partially delivered
---

# Docker Foundations roadmap

Status: the initial vertical slice and six-mission beginner campaign are implemented. This document records the delivered boundary and the capabilities intentionally left for later work.

## Delivered vertical slice

The first milestone added the environment boundary and one Docker mission end-to-end:

1. Introduce small execution and observation interfaces plus an environment factory.
2. Wrap the existing in-memory sandbox without changing Linux gameplay.
3. Add an environment type to missions, defaulting existing content to `simulated`.
4. Detect the Docker CLI, active Docker-compatible engine, and pinned fixture image through `opsquest doctor`, including provider-specific readiness for OrbStack's official context.
5. Implement **Container Census** (currently Mission 20) with disposable, attempt-owned containers and reliable teardown.
6. Validate the container’s observable state, independent of the exact commands used.

Container Census includes three progressive teaching hints: understand image/container lifecycle, discover stopped containers with `docker ps -a`, then start the existing container with `docker start api`. Hints remain optional and reduce the XP reward.

The portable test gate uses a fake Docker runner. `make docker-integration` checks the active Docker context, while `make orbstack-integration` explicitly checks OrbStack's `orbstack` context. Both real-engine targets require the pinned fixture image documented in the README.

## Delivered beginner campaign: “It Works on My Machine”

The first world now contains six beginner missions:

- **Container Census** — distinguish images from running and stopped containers.
- **Last Broadcast** — diagnose an exited one-shot job using bounded `logs`.
- **Exit Code Detective** — use sanitized inspection state to identify a successful job.
- **Quiet the Worker** — stop one exact running container while preserving another.
- **Recovery Pair** — start two existing stopped services.
- **Shift Handoff** — combine start and stop actions without changing owned resource count.

The delivered typed subset is `docker ps`, `docker container ls`, `start`, `restart`, `stop`, `inspect`, and `logs`. Diagnostic fixtures use a fixed entrypoint program with bounded declarative log data and exit codes; player text never becomes a host-shell or raw Docker command.

## Possible later increments

Environment-aware container creation, port publication, arbitrary in-container execution, volumes, and networking remain proposals. Each would materially widen the current boundary and therefore needs its own threat model, parser contract, resource ownership rules, and integration coverage before becoming gameplay.

## Important boundaries

- Docker remains optional; all Linux missions work without it.
- Player input is parsed and allowlisted by OpsQuest, never passed to a host shell.
- Every resource receives unique OpsQuest labels and resource limits.
- Success, restart, quit, terminal EOF/Ctrl-C, switching, setup failure, and ordinary errors reconcile and clean up only exact OpsQuest-owned resources.
- No privileged containers, host bind mounts, or Docker socket exposure inside labs.
- Dockerfiles, volumes, networking, Compose, and Kubernetes remain follow-up iterations.

The campaign tests genuine Docker gameplay while keeping the adapter intentionally smaller than the general Docker CLI. The separate **Automation Shift** Linux campaign provides dedicated `vi` and safe `sh` practice without widening the Docker boundary.
