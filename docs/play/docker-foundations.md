---
description: Prepare and play the optional, disposable OpsQuest Docker missions.
audience: players
status: current
---

# Docker Foundations

Docker Foundations is an optional track. Linux gameplay never requires a container engine, and OpsQuest never pulls an image automatically. The track works with Docker Engine, Docker Desktop, or OrbStack through the Docker CLI.

## Prepare the first lab

Install and start your chosen Docker-compatible engine, then explicitly fetch the pinned fixture image:

```console
$ docker pull docker.io/library/busybox@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662
$ opsquest doctor
$ opsquest play 20
```

### Use OrbStack on macOS

OrbStack exposes its engine through the standard Docker CLI and the `orbstack` Docker context. Select that context for both the image pull and OpsQuest so they use the same image store:

```console
$ DOCKER_CONTEXT=orbstack docker pull docker.io/library/busybox@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662
$ DOCKER_CONTEXT=orbstack opsquest doctor
$ DOCKER_CONTEXT=orbstack opsquest play 20
```

These one-command environment settings leave your global Docker context unchanged. Alternatively, run `docker context use orbstack` once and then use the ordinary commands above. `opsquest doctor` identifies OrbStack when its official context is active.

You can also discover the track explicitly:

```console
$ opsquest map --track docker
$ opsquest play --track docker
```

## Current lessons

The track has 11 missions in two worlds. **World 1: It Works on My Machine** is 6 beginner missions covering a deliberately narrow lifecycle loop:

1. **Container Census** — list containers and start an existing stopped service.
2. **Last Broadcast** — read bounded logs from an exited one-shot job.
3. **Exit Code Detective** — distinguish success from failure using sanitized inspection state.
4. **Quiet the Worker** — stop one target while preserving a healthy service.
5. **Recovery Pair** — restore two stopped services without replacing them.
6. **Shift Handoff** — combine `start` and `stop` while preserving supporting metrics.

**World 2: Container Triage** adds health checks, restart policies, log tails, and removal:

1. **Janitor Duty** — remove finished job containers while services keep running.
2. **Tail End** — show only the last lines of a long job log.
3. **Running Isn't Healthy** — pull a running but unhealthy replica from service and start its standby.
4. **Crash Loop** — stop a container its restart policy keeps relaunching, then show why it fails.
5. **Postmortem Triage** — combine health, exit codes, stop, and remove, and keep the evidence the postmortem needs.

## Teaching subset

| Command | Supported forms |
| --- | --- |
| List | `docker ps` or `docker container ls`, with `-a`/`--all` and up to four `--filter`/`-f` values: `status=created\|running\|restarting\|exited` or `health=starting\|healthy\|unhealthy\|none` |
| Lifecycle | `start`, `restart`, and `stop` with one alias |
| Remove | `docker rm ALIAS` for a stopped container; `--force` is refused, so stop the container first |
| Inspect | `docker inspect ALIAS` shows logical state, exit code, health, restart count, and restart policy |
| Logs | `docker logs ALIAS` or `docker logs --tail N ALIAS` (also `-n N`, `--tail=N`, and `--tail all`) |

Every command also has a `docker container ...` form. OpsQuest parses these forms itself and accepts exact logical aliases. It never forwards other Docker CLI arguments or flags, and filters run inside OpsQuest rather than being passed to Docker.

## Isolation boundary

OpsQuest generates unique names and ownership labels, maps player-visible aliases to exact container IDs, applies resource restrictions, and removes only resources verified as belonging to the current attempt. Labs do not use privileged mode, host bind mounts, host networking, devices, or a mounted Docker socket.

Health probes and restart policies are fixed fixture behaviors chosen by the mission, never player input. A crash-loop fixture's restart policy allows at most 50 retries.

## Cleaning up after a crash

OpsQuest removes a lab's containers when the attempt ends. If the process is killed first, its containers stay behind. Their processes exit on their own within 24 hours, and the next Docker mission removes them. You can also check and clean up yourself:

```console
$ opsquest doctor            # reports orphaned lab containers
$ opsquest doctor --cleanup  # removes them
```

A container counts as orphaned only when it has every OpsQuest ownership label and its generated name, and either its recorded owner process on this machine has exited or it is older than 24 hours. Containers belonging to other running OpsQuest sessions on this machine are left alone for their 24-hour lifetime.

The selected Docker-compatible engine remains a powerful external dependency. OpsQuest constrains the lesson and cleanup scope; it does not present the engine itself as an untrusted-code security boundary.

See [Sandbox and safety](../technical/sandbox-and-safety.md#docker-teaching-boundary)
for the full lifecycle and the [roadmap](../roadmap/README.md#docker-foundations)
for possible later expansions.
