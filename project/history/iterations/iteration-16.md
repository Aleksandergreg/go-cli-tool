---
description: Historical delivery record for OpsQuest iteration 16.
audience: contributors and maintainers
status: historical
search:
  exclude: true
---

# OpsQuest iteration 16

> Historical delivery record. See the [repository README](../../../README.md) for current behavior.

Date: 2026-07-22
Version: 0.6.0 (unchanged)

## Summary

OpsQuest now ships 29 missions: 23 Linux missions across four worlds and six beginner Docker Foundations missions. The Linux additions fill early reading, focused log-preview, simple counting, and supplied-script gaps; the Docker world now teaches bounded logs, sanitized exit status, targeted stop, multi-container recovery, and a mixed lifecycle handoff while retaining the exact-resource safety boundary.

## Delivered

- Added one beginner mission to each Linux world: read the correct handoff file, preview a log's opening lines, count error records through either `grep -c` or a pipeline, and inspect then run a supplied report script.
- Expanded **It Works on My Machine** from one to six beginner missions with exited-job logs, exit-code diagnosis, targeted stop, two-service recovery, and a start/stop handoff.
- Added exact `docker logs ALIAS` and `docker stop ALIAS` teaching actions, including their `docker container` forms, help text, completion candidates, and strict rejection of extra flags or aliases.
- Added declarative stopped one-shot fixtures with a paired bounded `log` and `exit_code`, a fixed non-interpolating shell program, setup-time start/wait verification, and sanitized logical inspection output.
- Added the `docker_container_stopped` observable condition and retained owned-container counts and required healthy-state checks for lifecycle missions.
- Added canonical, alternative, and incomplete-outcome coverage for all nine new missions, adapter success and rejection coverage, and real OrbStack integration coverage for diagnostics and targeted stop.
- Updated CLI navigation expectations, deterministic smoke coverage, current docs, safety diagrams, curriculum maps, and the Docker roadmap for the 29-mission layout.

## Compatibility and safety

- Existing mission IDs are unchanged, so version 2 profile completions, hint history, XP, and achievements remain readable without migration. Global display numbers from the first insertion onward changed to preserve contiguous world ordering; numeric bookmarks and scripts must use the new map, while stable IDs remain the compatibility-safe reference.
- The mission schema change is additive: Docker containers may now declare `log` and `exit_code` together, and validations may use `docker_container_stopped`. Catalog loading rejects partial diagnostic declarations, logs over 8 KiB, NUL data, exit codes outside 0–255, and diagnostic fixtures not declared stopped.
- The profile schema, profile version, executable version, Linux sandbox grammar, and dependency set are unchanged.
- Player Docker text still cannot reach a host shell or raw Docker passthrough. The new parser forms accept one validated logical alias, resolve it to the tracked exact ID, and construct fixed Docker arguments internally.
- Diagnostic log text is passed as positional data to a fixed `/bin/sh` program rather than interpolated into shell syntax. Existing no-network, read-only, non-root, capability, resource, labeling, ownership-verification, output-cap, timeout, and cleanup controls remain in force.
- Real OrbStack validation removed every disposable OpsQuest-labeled integration container; a follow-up label-filtered listing returned no IDs.

## Validation

| Command | Observed result |
| --- | --- |
| `go test ./internal/mission ./internal/game ./internal/dockerlab` | PASS; focused catalog, canonical/alternative/incomplete mission, and Docker adapter tests completed successfully. |
| `go test ./...` | PASS; all Go packages completed successfully. |
| `make smoke-test` | PASS; onboarding, continuous and one-shot play, 23-mission Linux maps, six-mission Docker discovery, doctor output, and world-local navigation passed. |
| `make orbstack-integration` | PASS against the active OrbStack context; real diagnostic logs/exit status, targeted stop, ownership-scoped lifecycle, and cleanup completed successfully. |
| `docker container ls --all --quiet --filter label=com.opsquest.managed=true` | PASS; no managed container IDs remained after integration. |
| `make check-all` | PASS; agent docs, embedded mission integrity, all packages, vet, build, smoke coverage, and race detection passed. |
| `make docs-check ZENSICAL=/tmp/opsquest-doc-venv/bin/zensical` | PASS; Zensical 0.0.51 completed a clean strict build with no issues. |
| `git diff --check` | PASS; no whitespace errors were reported. |

An early `make validate-missions` run correctly failed while count- and navigation-sensitive tests still described the old 20-mission catalog. Those fixtures were updated, and the same mission-validation target passed inside the final `make check-all` run.

## Remaining work

- Environment-aware creation, port publication, arbitrary `exec`, volumes, networking, Compose, and Kubernetes remain outside the typed Docker teaching boundary and require separate threat models before implementation.
- The expansion intentionally keeps every new lesson at beginner difficulty; later missions can deepen the same skills without weakening current observable outcomes.

## Final repository state

The `main` worktree contains only the intentional mission catalog, Docker adapter, tests, current documentation, diagrams, smoke expectations, and this iteration report. No profile migration, dependency change, generated release artifact, or unrelated user edit is included.
