---
description: Prepare and play the optional, disposable OpsQuest Docker Foundations missions.
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

The six beginner missions cover a deliberately narrow operational loop:

1. **Container Census** — list containers and start an existing stopped service.
2. **Last Broadcast** — read bounded logs from an exited one-shot job.
3. **Exit Code Detective** — distinguish success from failure using sanitized inspection state.
4. **Quiet the Worker** — stop one target while preserving a healthy service.
5. **Recovery Pair** — restore two stopped services without replacing them.
6. **Shift Handoff** — combine `start` and `stop` while preserving supporting metrics.

The delivered teaching subset recognizes container listing plus `start`, `restart`, `stop`, `inspect`, `logs`, and `help` forms. It accepts exact logical aliases without forwarding arbitrary Docker CLI arguments or flags.

## Isolation boundary

OpsQuest generates unique names and ownership labels, maps player-visible aliases to exact container IDs, applies resource restrictions, and removes only resources verified as belonging to the current attempt. Labs do not use privileged mode, host bind mounts, host networking, devices, or a mounted Docker socket.

The selected Docker-compatible engine remains a powerful external dependency. OpsQuest constrains the lesson and cleanup scope; it does not present the engine itself as an untrusted-code security boundary.

See [Sandbox and safety](../technical/sandbox-and-safety.md#docker-teaching-boundary) for the full lifecycle and [Docker Foundations roadmap](../roadmap/docker-foundations.md) for possible later lessons.
