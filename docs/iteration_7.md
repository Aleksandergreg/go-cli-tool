Implemented all five improvements.

- Filesystem moves/copies now preserve invariants: same-path moves cannot delete files, moving CWD/ancestors is rejected, `cp -r .` works, and virtual `/` cannot be copied.
- Added deterministic quotas for files, aggregate storage, archives, command output/input, environment state, entries, paths, and ownership metadata. Recursive operations preflight before mutation.
- Blank lines now survive `head`, `tail`, `grep`, `sort`, `uniq`, `awk`, and `cut`.
- Tar metadata is published only after successful archive creation; failed replacements preserve existing metadata and backing files.
- Linux Completionist now checks every current catalog mission, ignoring stale completion IDs.
- Added bounded output construction and `sed` amplification preflight.

Key files include [filesystem.go](/Users/aleksandergregersen/privat/go-cli-tool/internal/sandbox/filesystem.go:115), [output.go](/Users/aleksandergregersen/privat/go-cli-tool/internal/sandbox/output.go:12), [commands_system.go](/Users/aleksandergregersen/privat/go-cli-tool/internal/sandbox/commands_system.go:188), [commands_transform.go](/Users/aleksandergregersen/privat/go-cli-tool/internal/sandbox/commands_transform.go:184), and [progression.go](/Users/aleksandergregersen/privat/go-cli-tool/internal/game/progression.go:10). Documentation was updated in [README.md](/Users/aleksandergregersen/privat/go-cli-tool/README.md:151) and `initial_prompt.md`.

Validation passed:

- Focused sandbox, completionist, quota, archive, and blank-line tests
- `go test ./...`
- Final `make check-all`, including agent docs, mission validation, vet, build, smoke test, and `go test -race ./...`
- `git diff --check`

No checks were skipped. Git status contains 15 modified and 3 new files, all intentional for this work; no unrelated changes were found. Existing persisted achievement unlocks remain monotonic and are not revoked.
