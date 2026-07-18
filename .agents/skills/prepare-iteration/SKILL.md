---
name: prepare-iteration
description: "Use when completing an OpsQuest iteration, release-sized milestone, or delivery report that needs the full quality gate and version documentation. Do not use for routine single-feature handoffs."
---

# Prepare an OpsQuest iteration

Use [references/iteration-template.md](references/iteration-template.md) for the final `iteration_N.md`. The report is an evidence record, so include only commands and results actually observed.

## Workflow

1. Establish the milestone boundary.
   - Read `iteration_1.md`, `iteration_2.md`, later iteration reports, README, `internal/cli/app.go`, and the milestone request.
   - Identify the next iteration number and intended version. Do not change version strings or roadmap claims unless the milestone requires it.

2. Review the complete change set.
   - Run `git status --short --branch`, `git diff --stat`, and `git diff` before final validation.
   - Separate intentional milestone changes from pre-existing user work. Do not discard, overwrite, stage, or rewrite unrelated changes.
   - Check package boundaries, error paths, help output, tests, mission counts, and safety behavior against `AGENTS.md`.

3. Audit compatibility and documentation.
   - Call out CLI output/flag changes, mission schema changes, profile-version or migration effects, and any dependency change.
   - Update README feature descriptions, commands, supported sandbox list, development instructions, and roadmap only where delivered behavior changed.
   - Keep `internal/buildinfo.Version`, the Release Please manifest, README version statements, and the iteration report consistent when releasing a new version.

4. Run release validation.
   - Run focused tests for touched packages first if they were not already observed.
   - Run `make check-all`; this is the comprehensive source-of-truth gate and includes agent docs, all Go tests and mission integrity, vet, the release binary build, deterministic CLI smoke testing, and race detection.
   - For changes to `.goreleaser.yml` or the release workflow, run `make release-check` followed by `make release-snapshot`. These targets require GoReleaser v2 and never publish; record the exact reason if the tool is unavailable.
   - Use individual targets such as `make build` or `make smoke-test` only to isolate a failure or when the full target cannot run. Verify the built release binary with `./bin/opsquest version` when the version changes.
   - Record exact commands, exit results, and relevant observed output. Never infer that an unrun check passed because a related command succeeded.

5. Write `iteration_N.md`.
   - Summarize delivered user behavior, architecture or tooling changes, and tests without copying commit history.
   - Describe compatibility and safety considerations, including intentionally unchanged formats or boundaries.
   - List validation results actually observed and remaining work that is genuinely outside the iteration.

6. Perform the final audit.
   - Re-read the report and README against the executable behavior.
   - Run `git diff --check`, inspect `git diff --stat`, and finish with `git status --short --branch`.
   - Report every check not run with the exact constraint, plus delivered behavior, compatibility considerations, validation, remaining risks, and final working-tree status.
