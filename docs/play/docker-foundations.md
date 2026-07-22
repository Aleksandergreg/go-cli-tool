---
description: Prepare and play the optional, disposable OpsQuest Docker Foundations mission.
audience: players
status: current
---

# Docker Foundations

Docker Foundations is an optional track. Linux gameplay never requires Docker, and OpsQuest never pulls an image automatically.

## Prepare the first lab

Install and start Docker, then explicitly fetch the pinned fixture image:

```console
$ docker pull docker.io/library/busybox@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662
$ opsquest doctor
$ opsquest play 17
```

You can also discover the track explicitly:

```console
$ opsquest map --track docker
$ opsquest play --track docker
```

## Current lesson

**Container Census** teaches the difference between images, running containers, and stopped containers. The delivered teaching subset recognizes container listing plus `start`, `restart`, `inspect`, and `help` forms. It does not forward arbitrary Docker CLI arguments.

## Isolation boundary

OpsQuest generates unique names and ownership labels, maps player-visible aliases to exact container IDs, applies resource restrictions, and removes only resources verified as belonging to the current attempt. Labs do not use privileged mode, host bind mounts, host networking, devices, or a mounted Docker socket.

The Docker daemon remains a powerful external dependency. OpsQuest constrains the lesson and cleanup scope; it does not present the daemon itself as an untrusted-code security boundary.

See [Sandbox and safety](../technical/sandbox-and-safety.md#docker-teaching-boundary) for the full lifecycle and [Docker Foundations roadmap](../roadmap/docker-foundations.md) for possible later lessons.
