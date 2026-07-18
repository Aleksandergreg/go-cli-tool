# Docker Foundations implementation plan

Status: the first v0.3 vertical slice described below is implemented. This document now records the delivered boundary and the next campaign increments.

## Delivered vertical slice

The first milestone adds the environment boundary and one Docker mission end-to-end:

1. Introduce small execution and observation interfaces plus an environment factory.
2. Wrap the existing in-memory sandbox without changing Linux gameplay.
3. Add an environment type to missions, defaulting existing content to `simulated`.
4. Detect the Docker CLI, daemon, and pinned fixture image through `opsquest doctor`.
5. Implement Mission 17, **Container Census**, with disposable, attempt-owned containers and reliable teardown.
6. Validate the container’s observable state, independent of the exact commands used.

Container Census includes three progressive teaching hints: understand image/container lifecycle, discover stopped containers with `docker ps -a`, then start the existing container with `docker start api`. Hints remain optional and reduce the XP reward.

The portable test gate uses a fake Docker runner. `make docker-integration` is the explicit real-engine lifecycle check and requires Docker plus the pinned fixture image documented in the README.

## Next campaign increments: “It Works on My Machine”

After the vertical slice proves reliable, expand it to roughly five missions:

- **Container Census** — distinguish images from running and stopped containers.
- **The Silent Crash** — diagnose a failed container using `ps` and `logs`.
- **Wrong Environment** — recreate a service with the correct environment variable.
- **Port Problems** — publish container port 8080 on host port 3000.
- **Emergency Shell** — inspect a running container with `exec` and repair the incident.

The delivered typed subset is `docker ps`, `docker container ls`, `start`, `restart`, and `inspect`. Each later mission should add only the semantics it teaches; likely next commands are `logs`, environment-aware `run`, and then port publication.

## Important boundaries

- Docker remains optional; all Linux missions work without it.
- Player input is parsed and allowlisted by OpsQuest, never passed to a host shell.
- Every resource receives unique OpsQuest labels and resource limits.
- Success, restart, quit, terminal EOF/Ctrl-C, switching, setup failure, and ordinary errors reconcile and clean up only exact OpsQuest-owned resources.
- No privileged containers, host bind mounts, or Docker socket exposure inside labs.
- Dockerfiles, volumes, networking, Compose, and Kubernetes remain follow-up iterations.

This vertical slice tests genuine Docker gameplay without committing OpsQuest to the entire Docker campaign at once. A scripting-focused Linux mini-campaign also remains a sensible later content addition because `vi` and `sh` are implemented but do not yet have dedicated missions.
