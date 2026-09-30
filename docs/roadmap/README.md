---
description: Forward-looking OpsQuest gameplay, distribution, and curriculum proposals that are not implemented.
audience: players, contributors, and maintainers
status: proposed
---

# Roadmap

This page records possible future work, not delivery promises or current
behavior. Shipped behavior belongs in the player and technical guides.

## Docker Foundations

The current two Docker worlds intentionally support only listing with fixed
filters, logs, sanitized inspection, bounded lifecycle actions, and removal of
stopped attempt-owned containers. Fixture health probes and crash loops are
fixed behaviors chosen by the mission.

Possible later increments include:

- environment-aware container creation through a limited `docker run`;
- limited port publication;
- bounded volumes and networking;
- Dockerfile or Compose concepts, possibly taught in the simulated shell;
- restart-to-recover health lessons, which need fixture state that survives a
  restart.

Each expansion would widen the external-engine boundary. It requires its own
parser contract, threat model, resource ownership rules, cleanup behavior, and
real-engine integration coverage before becoming gameplay. Privileged mode,
host bind mounts, host networking, devices, and Docker socket exposure remain
out of scope.

## Delivery and distribution

Possible follow-ups include:

- scheduled `govulncheck ./...` after its update behavior is understood;
- artifact attestations for released archives;
- opt-in scheduled Docker lifecycle testing;
- signed checksums, macOS signing and notarization, or a Homebrew tap when
  external binary distribution justifies their credentials and maintenance.

## Curriculum and product ideas

Longer-term ideas include external mission packs, streaks, efficiency medals,
and generated daily incidents. Kubernetes remains future scope and would need
an isolated disposable cluster plus a separate safety design.

When a proposal ships, remove it from this page and update its canonical
current guide in the same change.
