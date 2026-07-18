---
name: add-mission
description: "Use when adding, modifying, renumbering, or reviewing OpsQuest mission JSON and its campaign coverage. Do not use for sandbox command semantics unless mission work also requires an engine change."
---

# Add or modify an OpsQuest mission

Read [references/mission-checklist.md](references/mission-checklist.md) before editing mission content. It records the current JSON schema, supported outcome conditions, and count-sensitive files.

## Workflow

1. Inspect the current state.
   - Read `git status` and preserve unrelated changes.
   - Read the missions immediately before and after the intended catalog position in `internal/mission/data/`.
   - Read `internal/mission/mission.go`, `internal/mission/catalog.go`, `internal/game/validator.go`, `internal/mission/catalog_test.go`, and `internal/game/missions_test.go`.
   - Check the track-local world/campaign order, stage placement, difficulty and XP progression, existing validators, and test patterns before choosing an implementation.

2. State the learning objective before writing JSON.
   - Identify the Linux or Docker concept being practiced, the observable successful state or output, and the misconception an incorrect-solution test should catch.
   - Decide whether multiple legitimate command routes exist. The validator must leave those routes open.

3. Model the lab declaratively.
   - For a simulated mission, put directories, files, virtual processes, environment values, and virtual archives in `setup`. For a Docker mission, select the Docker track/environment and declare only digest-pinned images and logical container fixtures in `docker`.
   - Express success with existing `validation.all` conditions whenever they can describe the outcome accurately.
   - Add or extend a Go validator and environment observer only when the existing condition types cannot express the learning outcome; keep the mission JSON free of executable setup or command-history requirements.
   - Do not relax catalog, sandbox, or validator behavior to accommodate malformed content.

4. Keep content consistent.
   - Use a zero-padded numeric filename, a contiguous stable `number`, a lowercase hyphenated `id`, and the exact existing campaign name when joining a world. Keep every campaign contiguous within its track.
   - Make the story and objective describe the incident and outcome without prescribing the solution.
   - Curate `suggested_commands` as unique base command names supported by the mission environment. Keep this free orientation broader than one canonical route, but do not include flags, arguments, or an exact solution.
   - Order hints from conceptual nudge to increasingly concrete syntax. Write an explanation that teaches why the solution works.
   - Calibrate `difficulty`, `xp`, and `hint_penalty` against adjacent stages; keep each world's learning ramp and boss placement coherent.

5. Prove the mission behavior.
   - Add the canonical command sequence to the table in `internal/game/missions_test.go` so every catalog entry has a working outcome.
   - Add at least one alternative valid solution test when more than one meaningful route exists.
   - Add an incomplete or incorrect solution test that demonstrates the validator does not award completion early.
   - Add focused schema or loader coverage in `internal/mission` when introducing a new field or validation condition.

6. Update catalog-facing documentation.
   - Adjust hard-coded mission counts, world/stage descriptions, map expectations, smoke assertions, and iteration material when the catalog size or organization changes.
   - Update command documentation only if the mission also introduces supported simulated-shell or Docker-lab behavior.

7. Validate in increasing scope.
   - Run `go test ./internal/mission ./internal/game` first, adding `./internal/dockerlab` when Docker fixtures, observations, or command behavior are involved.
   - Run `make validate-missions`, then `make check` after the focused tests pass.
   - Run `make check-all` for release-sized work or changes to schema, validators, parser behavior, persistence, or isolation.

In the handoff, name the learning objective, canonical and alternative routes tested, rejected incomplete route, documentation/count updates, and exact command results.
