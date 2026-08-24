# OpsQuest agent guide

OpsQuest is a Go 1.26 CLI game with 23 in-memory Linux missions and 6 optional Docker Foundations labs. Linux player commands stay inside the teaching shell. Docker input is parsed into a deliberately small command subset that can affect only disposable, OpsQuest-labeled resources. Kubernetes remains future scope unless a task explicitly adds it.

## Repository map

- `cmd/opsquest`: process entry point, dependency construction, and exit reporting.
- `internal/buildinfo`: Release Please-managed executable version metadata.
- `internal/cli`: public CLI commands, flags, help, and presentation.
- `internal/ui`: terminal-aware ANSI styling and the shared color policy for presentation.
- `internal/game`: mission sessions, rewards, and observable-outcome validation.
- `internal/mission`: JSON schema, strict embedded catalog loading, track-local world/stage derivation, integrity checks, and `data/` mission content.
- `internal/sandbox`: parser, dispatcher, virtual filesystem, virtual processes, archives, and the supported command subset.
- `internal/dockerlab`: optional Docker capability detection, typed teaching commands, isolated resource lifecycle, and Docker observations.
- `internal/webapp`: loopback-only web companion, one-time browser pairing, sanitized attempt projection, and embedded static assets.
- `internal/profile`: versioned profile model and atomic JSON persistence.
- `scripts`: deterministic repository checks used by both Make and CI.
- `docs/game`: learning principles and the high-level product explanation.
- `docs/technical`: current architecture, safety, and implementation diagrams.
- `docs/roadmap`: explicitly labeled proposals and unfinished improvements.
- `project/history/iterations`: repository-only delivery reports for completed product iterations.
- `project/decisions`: implemented project decisions that should not appear on the public site.
- `mkdocs.yml` and `.github/workflows/docs.yml`: pinned Zensical navigation, strict site validation, and GitHub Pages delivery.

## Working workflow

1. Inspect `git status`, the relevant package, adjacent tests/content, and the current README before changing code. Use the matching repository skill for mission, sandbox-command, or iteration work.
2. Make the smallest cohesive change within existing package boundaries. Run focused package tests while iterating.
3. Run `make check` before handing off ordinary changes. Run `make check-all` for release-sized, concurrency-sensitive, persistence, parser, or sandbox work.
4. Review `git diff` and `git status`, then report the behavior changed and every validation command actually observed.

Safe local edits, tests, builds, and documentation updates within the requested scope are authorized. Do not contact external services, publish changes, generate mission content, expand Docker gameplay, or begin Kubernetes gameplay without explicit scope.

## Project invariants

- Never execute player-entered commands through a host shell or against the host filesystem.
- Sandbox changes must preserve isolation from host processes and files.
- Docker labs must reject raw command passthrough and operate only on exact resources owned and labeled by the current attempt.
- The web companion must remain loopback-only and must not accept player command text, mutate mission state, or approve completion.
- Mission validators must check observable outcomes, not require one exact command sequence.
- Mission content remains declarative in embedded JSON; engine behavior belongs in Go.
- Mission schema and persisted-profile changes are compatibility-sensitive and require explicit migration or compatibility reasoning.
- Do not weaken existing safety, parsing, schema, persistence, or validation behavior merely to make a new mission pass.
- New command behavior requires success, edge, and failure tests plus user-facing help where applicable.
- Mission changes require canonical success coverage and an incomplete or incorrect solution test.
- A new third-party production dependency requires a concrete repository-specific justification.
- Run the relevant quality gate and report any check that could not be run.
- Preserve unrelated user changes in the working tree.

## Validation and documentation

- Focused packages: `go test ./internal/<package>`.
- Embedded mission integrity: `make validate-missions`.
- Agent harness structure: `make check-agent-docs`.
- Deterministic CLI path: `make smoke-test`.
- Real Docker lifecycle (for Docker changes when prerequisites are available): `make docker-integration`; use `make orbstack-integration` to force OrbStack's Docker context.
- GoReleaser configuration and local packages (for release automation changes): `make release-check` and `make release-snapshot`.
- Hosted documentation: install `requirements-docs.txt`, then run `make docs-check`.
- Normal local gate: `make check`.
- Comprehensive gate, including race detection: `make check-all`.

Update README command lists, mission counts, safety notes, or roadmap claims when behavior changes. Use `$prepare-iteration` for a release-sized milestone and base its `project/history/iterations/iteration-NN.md` report on the supplied template.

## Definition of done

The requested behavior and relevant failure paths are covered, focused tests pass, the appropriate quality gate has been observed, documentation matches the shipped CLI/content, the diff contains only intentional changes, and the final report includes compatibility considerations and exact validation results.
