---
description: Prepare a disposable local Kubernetes cluster and play Checkout on Call.
audience: players and contributors
status: current
---

# Checkout on Call

Three incidents teach Pod discovery, application logs, and Deployment scaling.
Each mission starts in a fresh single-node kind cluster with its own checkout
namespace. Allow about a minute to prepare a stage or restart it.

## Local setup

Install kind v0.33.0, kubectl compatible with Kubernetes 1.35, and a local Docker
engine supporting platform-specific image export (Docker API 1.48 or newer).
Start the engine and preload the kind node and BusyBox fixture images.

```sh
docker pull kindest/node:v1.35.8@sha256:07b2536e30b803ed61d1677a79df6115f798ce64c80f9e22f6ed45afd09323c0
docker pull docker.io/library/busybox@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662
opsquest doctor
opsquest play --track kubernetes --world 1
```

The node image comes from the [kind v0.33.0 release](https://github.com/kubernetes-sigs/kind/releases/tag/v0.33.0).
Use `opsquest map --track kubernetes --ids` to see all three stages, or
`opsquest play --once 45` to jump to the scaling challenge.

## Commands in the lab

| Command | Result |
| --- | --- |
| `kubectl get pods` | List Pod names and phases in the lab namespace |
| `kubectl get pods -l app=LABEL` | Select Pods with a fixture app label. The long flag is `--selector`. |
| `kubectl logs POD` | Read the last ten lines of a named fixture Pod, up to 1 KiB |
| `kubectl get deployments` | Compare desired and ready replica counts |
| `kubectl scale deployment/NAME --replicas=N` | Set a fixture Deployment to one, two, or three replicas |

Use `help kubectl` to recall the syntax. After scaling, run
`kubectl get deployments` again while the new Pods start. Completion checks
read the current generation, desired count, and updated and ready replicas.

Quitting or restarting closes the cluster. If OpsQuest is forcibly killed,
starting another Kubernetes mission on the same local Docker endpoint retries
cleanup using the saved attempt identity. The shared image cache remains
available for later missions.
