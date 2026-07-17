# OpsQuest iteration {N}

Date: {YYYY-MM-DD}
Version: {version or unreleased}

## Summary

Describe the milestone's user-visible outcome in two or three sentences.

## Delivered

- Group shipped behavior by product area rather than by commit.
- Include repository tooling or documentation only when it materially affects delivery.
- Distinguish completed scope from intentionally deferred work.

## Compatibility and safety

- State effects on CLI behavior, mission schema, persisted profiles, dependencies, and sandbox isolation.
- State important formats or boundaries that deliberately remain unchanged.

## Validation

| Command | Observed result |
| --- | --- |
| `command actually run` | `PASS` or `FAIL`, with concise observed detail |

List a required check that was not run separately with its exact reason. Do not put an assumed result in the table.

## Remaining work

- Record sensible follow-up work outside this iteration.
- Note known risks or limitations that maintainers should carry forward.

## Final repository state

Summarize the final `git status --short --branch` output and identify any pre-existing or intentionally uncommitted changes.
