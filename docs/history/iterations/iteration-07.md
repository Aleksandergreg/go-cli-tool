---
description: Historical delivery record for OpsQuest iteration 7.
audience: contributors and maintainers
status: historical
search:
  exclude: true
---

# OpsQuest iteration 7

> Historical delivery record. See the [documentation map](../../README.md) for current behavior.

Implemented all five improvements.

- Filesystem moves/copies now preserve invariants: same-path moves cannot delete files, moving CWD/ancestors is rejected, `cp -r .` works, and virtual `/` cannot be copied.
- Added deterministic quotas for files, aggregate storage, archives, command output/input, environment state, entries, paths, and ownership metadata. Recursive operations preflight before mutation.
- Blank lines now survive `head`, `tail`, `grep`, `sort`, `uniq`, `awk`, and `cut`.
- Tar metadata is published only after successful archive creation; failed replacements preserve existing metadata and backing files.
- Linux Completionist now checks every current catalog mission, ignoring stale completion IDs.
- Added bounded output construction and `sed` amplification preflight.

Key files include [filesystem.go](https://github.com/Aleksandergreg/go-cli-tool/blob/main/internal/sandbox/filesystem.go#L115), [output.go](https://github.com/Aleksandergreg/go-cli-tool/blob/main/internal/sandbox/output.go#L12), [commands_system.go](https://github.com/Aleksandergreg/go-cli-tool/blob/main/internal/sandbox/commands_system.go#L188), [commands_transform.go](https://github.com/Aleksandergreg/go-cli-tool/blob/main/internal/sandbox/commands_transform.go#L184), and [progression.go](https://github.com/Aleksandergreg/go-cli-tool/blob/main/internal/game/progression.go#L10). Documentation was updated in [README.md](https://github.com/Aleksandergreg/go-cli-tool/blob/main/README.md#L151) and `initial_prompt.md`.

Validation passed:

- Focused sandbox, completionist, quota, archive, and blank-line tests
- `go test ./...`
- Final `make check-all`, including agent docs, mission validation, vet, build, smoke test, and `go test -race ./...`
- `git diff --check`

No checks were skipped. Git status contains 15 modified and 3 new files, all intentional for this work; no unrelated changes were found. Existing persisted achievement unlocks remain monotonic and are not revoked.
