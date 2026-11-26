# OpsQuest iteration 17

Date: 2026-11-26

Version: unreleased

## Summary

Checkout on Call adds one optional Kubernetes world with three stages. Global
missions 43–45 teach Pod selectors, log diagnosis, and Deployment scaling
through a disposable local kind cluster for each attempt.

## Delivered

- Track selection, map, profile, doctor, command help, and web companion track
  display support Kubernetes. Bare play still starts or resumes Linux.
- The mission schema accepts one to four fixed Pod or Deployment fixtures.
  Readiness checks compare the Deployment's observed generation and desired,
  actual, updated, and ready replica counts.
- A local Docker endpoint, preloaded pinned images, and private kubeconfig
  isolate the lab from the player's active Kubernetes context. The game
  limits player operations through both its parser and namespace RBAC.
- Quit, restart, and failed setup release resources by verified identity.
  The next Kubernetes attempt also checks private ownership records for
  abandoned clusters on the same host and endpoint.

## Compatibility and safety

Existing IDs, mission numbers, profile format, and first-completion XP rules
are preserved. The catalog now has 45 missions. Linux and Docker players do
not need kind or kubectl. No production Go dependency was added. The external
cluster backend requires a trusted local Docker engine able to run a kind
control-plane node. Only setup code imports fixture images into that node.

## Validation

| Command | Observed result |
| --- | --- |
| `go test ./internal/mission ./internal/game ./internal/kubelab ./internal/cli` | Passed catalog, canonical outcomes, command boundary, readiness, and CLI tests. |
| `make validate-missions` | Initial sandbox run reached the web companion tests and failed to bind loopback. The same target passed with loopback access under `make check-all`. |
| `make check-all` | Passed agent and documentation checks, full tests, vet, build, CLI smoke tests, and race detection. |
| `make kubernetes-integration` | Passed on ARM64 OrbStack with kind 0.33.0. Three independent clusters exercised discovery, logs, and scaling. RBAC denied Secrets and cross-namespace Pod reads. Recovery preserved a live owner and removed the dead-owner fixture. Canceled setup cleaned up its node. |
| `make orbstack-integration` | Passed the existing Docker adapter lifecycle tests. |
| `make docs-check ZENSICAL=/private/tmp/opsquest-k8s-docs-env/bin/zensical` | Passed strict site build and documentation checks for all 18 public pages. |
| `opsquest play --track kubernetes --world 1 --once`, `opsquest play --once 43`, `opsquest profile` | A temporary profile completed stage 43 after a restart, then replayed with the long selector flag. Profile showed one of three complete and 60 XP. |
| `git diff --check` | Passed. |

The first live prototype exposed an image import failure on a host with a
single platform of a multi-platform manifest. The adapter now exports the
local platform and imports it under an attempt-local image name. Isolated
cluster setup took 30–31 seconds per stage. The CLI run with one restart took
67 seconds through completion, and replay took 34 seconds.

## Remaining work

Live lifecycle measurements cover ARM64 OrbStack. Other Docker providers and
architectures have not been measured for this world.

## Final repository state

The iteration is uncommitted on `first-kubernetes-missions`. The pre-existing
roadmap proposal was replaced by the shipped player guide. Release metadata
continues to be managed by Release Please.
