# OpsQuest iteration 13

Date: 2026-07-18
Version: Unreleased (current executable: 0.3.0)

## Summary

OpsQuest now introduces itself before a fresh profile's first mission and presents the Linux curriculum as four navigable learning worlds with explicit stage progress. Mission screens, hints, colors, completion guidance, and the first two world difficulty ramps were refined so exploration feels informative rather than punitive, while stable missions and outcome-based solutions remain intact.

## Delivered

- Added a concise one-time Linux quick start plus a replayable `opsquest guide` that explains objectives, outcome-based solutions, hints and XP, levels and ranks, world progression, terminal controls, and the boundary between lab and host commands.
- Added a world-aware curriculum map through `opsquest map` and the compatible `list` alias, including compact track-local world/stage positions, completion progress, optional stable IDs, and track-correct launch commands.
- Added `opsquest play --world N` and in-mission `world N` navigation. World play stops at its boundary; direct mission jumps and completed-mission replay remain unrestricted.
- Made selected and replayed missions continue in-process by default, added `play --once` for deliberate one-mission sessions with next-stage recommendations, and retained clear world-complete/next-world transitions during continuous play.
- Introduced a derived world model in `internal/mission`, including defensive copies, track-local placement, first-incomplete selection, and catalog rejection when one campaign would split into multiple worlds in the same track.
- Rebalanced the opening curriculum without renumbering missions: First Day is now Missions 1–5, and The Logpocalypse scales from beginner Missions 6–7 through intermediate Missions 8–9 to the advanced Mission 10 boss.
- Expanded the pipeline and Production Friday bosses to five progressively specific hints and made hint messages report the actual XP cost after the minimum-reward floor is applied.
- Simplified each mission introduction to three compact help lines, moved the full control guide behind `?`, labeled incident/objective/tool sections, and changed incomplete feedback from a warning to neutral progress.
- Made mission previews and profiles reflect persisted active hints and adjusted rewards, and made completed mission/replay screens state clearly that XP has already been claimed.
- Refined semantic color roles for page sections, worlds, objectives, progress, rewards, and achievements. Advanced difficulty is no longer rendered as an error, and long hint/achievement explanations are no longer colored as a single warning.
- Added a clearer world-based profile display and protected terminal output by rejecting control-bearing, non-printable, invalid, or overlong profile names while safely normalizing legacy/default names.
- Expanded focused, route-independent mission tests and the deterministic CLI smoke test for onboarding, maps, world jumps, world boundaries, profile safety, and new navigation.
- Updated repository instructions, the mission skill, README, and `initial_prompt.md` to describe the shipped world model and its maintenance rules.

## Compatibility and safety

- Existing mission IDs and global numbers remain stable. Existing `play`, `list`, direct mission selection, replay, profile, and in-mission navigation remain available; `map`, `guide`, world selection, and `play --once` are additive. Explicit mission selection now chooses the starting point for a continuous session instead of returning after success; scripts that require the old one-shot behavior can opt into it with `--once`.
- Mission setup, validators, rewards, and canonical outcomes are unchanged. Missions 4–5 changed world/campaign placement, Mission 6 changed displayed difficulty, and Missions 10/16 gained additional hints.
- The mission JSON schema and profile version remain unchanged. The profile JSON adds an optional `onboarded` marker; older profiles load without migration, existing progress suppresses redundant onboarding, and older binaries ignore the field when reading. If an older binary rewrites an otherwise pristine profile, only the marker may be lost and the quick start may appear again; gameplay progress is unaffected. Existing completion maps and XP are preserved, and existing in-progress hint counts remain valid with the expanded hint lists.
- Profile loading now removes unsafe legacy display characters, and new saves reject unsafe display names instead of persisting terminal control sequences.
- No production dependency was added. Linux commands still execute only in the in-memory teaching shell; Docker and Kubernetes gameplay were not expanded.

## Validation

| Command | Observed result |
| --- | --- |
| `GOCACHE=/tmp/opsquest-gocache go test ./internal/cli ./internal/game ./internal/mission ./internal/profile ./internal/ui` | PASS; focused CLI, session, mission, profile, and semantic-style tests passed. |
| `bash -n scripts/smoke-test.sh` | PASS; the expanded smoke script is valid Bash. |
| `./scripts/check-agent-docs.sh` | PASS; root instructions and all three repository skills validated. |
| `GOCACHE=/tmp/opsquest-gocache ./scripts/validate-missions.sh` | PASS; all packages and embedded canonical mission outcomes passed. |
| `GOCACHE=/tmp/opsquest-gocache ./scripts/smoke-test.sh` | PASS; onboarding, selected-mission continuation, explicit one-shot play, world map/jump, Linux and Docker discovery, profile, doctor, and in-mission navigation passed in an isolated profile home. |
| `GOCACHE=/tmp/opsquest-gocache make check-all` | PASS after the final continuation fix; agent docs, mission/all-package tests, vet, binary build, deterministic smoke test, and race detection all passed. |
| `git diff --check` | PASS; no whitespace errors were reported in the final change set. |

The real-Docker integration target was not run because Docker gameplay and its adapter were unchanged; the portable gate exercised the existing fake Docker contracts and Docker discovery paths. GoReleaser checks were not run because release configuration and packaging were unchanged. Go emitted non-fatal stat-cache warnings when the managed environment prevented writes to the default module cache; the requested temporary build cache was used and every command exited successfully.

## Remaining work

- Add a dedicated early file-reading stage and a smaller pipes/redirection exercise before the first pipeline boss rather than renumbering existing missions in this iteration.
- Consider declarative world-level focus text if external mission packs are introduced; current world placement is intentionally derived from existing campaign metadata.
- Validate the compact map and ANSI capability on native Windows terminals before making broader Windows UX claims.

## Final repository state

The `feature/ci-hardening` branch began the final continuation fix clean and now has six intentionally modified files: CLI orchestration and tests, deterministic smoke coverage, README, `initial_prompt.md`, and this iteration report. No generated `bin/` artifact is tracked, and no unrelated working-tree change was discarded.
