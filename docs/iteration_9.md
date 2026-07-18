# OpsQuest iteration 9

Date: 2026-07-18
Version: 0.3.0 (unchanged)

## Summary

This iteration hardens the existing Linux and Docker gameplay paths and clarifies the repository's dependency boundaries without adding new missions or commands. Extreme shell input is now predictably bounded, archive extraction and Docker cleanup recover more safely from failure, mission content cannot be mutated through catalog results, and several persistent-profile, navigation, and reporting bugs are fixed.

## Delivered

- Made `cmd/opsquest` the composition root for the catalog, profile store, and combined environment factory; the CLI no longer depends on the Docker adapter.
- Added indexed, deep-copying catalog lookups and track-aware first, last, and adjacent navigation; centralized condition vocabulary, Docker setup validation, and achievement reconciliation.
- Fixed replay hint cleanup (including legacy completed profiles), malformed negative hint progress, quoted in-mission navigation, campaign-scoped totals, duplicate Docker play probes, and slow live Docker checks in `profile`.
- Bounded expanded shell text (including redirection paths), arguments, pipeline stages, and total dispatches; each command line is expanded once and rejected before mutation when a preflight ceiling is exceeded.
- Made tar extraction transactional across filesystem and archive metadata, made `OLDPWD` the sole previous-directory state, propagated setup ownership errors, and explicitly rejected `tar -C` outside extraction.
- Made Docker teardown seal gameplay immediately while retaining and retrying only unresolved owned resources, including ambiguously failed creates whose generated names are not visible on the first inspection.
- Reduced long-line `vi` redraw allocation to the visible viewport and added a repeatable benchmark.
- Made the CLI smoke test incapable of finding or contacting a developer's Docker executable, and updated repository instructions and current-state documentation.

## Compatibility and safety

- The top-level CLI commands, 20 mission IDs/numbers, profile format version 2, executable version 0.3.0, and existing production dependencies are unchanged.
- Existing embedded mission JSON remains valid. Catalog loading now additionally enforces the existing beginner/intermediate/advanced vocabulary, one to five hints, condition-specific fields even when an unsupported field has an explicit zero value, and per-mission Docker fixture ceilings of 16 images and 32 containers.
- Linux player input still never reaches a host shell, process, or filesystem. New limits affect only oversized expanded commands, pipelines above 64 stages, or executions above 512 dispatches.
- `tar -C` remains supported for extraction; create/list uses now fail explicitly instead of silently ignoring the requested directory change.
- Docker operations remain opt-in, typed, label-verified, and restricted to exact attempt resources. Partial setup can now return an internal cleanup handle, which the game closes before surfacing the setup error.
- Replay completion preserves the original XP and completion timestamp while clearing transient replay hints. Existing achievement unlocks remain monotonic.

## Validation

| Command | Observed result |
| --- | --- |
| `GOCACHE=/private/tmp/opsquest-refactor-cache go test ./... -count=1` | PASS; every package test passed. |
| `GOCACHE=/private/tmp/opsquest-refactor-cache go test ./internal/game -run '^$' -bench BenchmarkRenderViLongLine -benchtime=20x -count=1` | PASS; both cursor-start and cursor-end long-line cases completed with 44 allocations per redraw. |
| `GOCACHE=/private/tmp/opsquest-refactor-cache make check-all` | PASS; agent docs, embedded mission/canonical validation, vet, build, deterministic smoke test, and `go test -race ./...` all passed. |
| `git diff --check` | PASS; no whitespace errors. |
| `./bin/opsquest version` | PASS; reported `OpsQuest 0.3.0`. |

`make docker-integration` was not run because `command -v docker` returned no executable in this environment. No daemon, network access, or external service was required by the completed quality gate.

## Remaining work

- A SIGKILL, host crash, or persistently unavailable Docker daemon can still leave labeled resources; a future explicit stale-resource cleanup command would improve recovery.
- Concurrent OpsQuest processes still use last-writer-wins profile persistence.
- The terminal reader and modal editor remain in `internal/game`; a later mechanical package split may help if those adapters grow, but is not required for current behavior.
- Long-line editor redraw at a far-right cursor is allocation-bounded but still scans the line to calculate display columns.

## Final repository state

The branch is `feature/docker-integration`, tracking `origin/feature/docker-integration`. The working tree began clean and now contains only the intentional implementation, tests, skill-reference, README/current-brief, smoke-test, agent-guide, and iteration-report changes from this task; no unrelated user changes were discarded.
