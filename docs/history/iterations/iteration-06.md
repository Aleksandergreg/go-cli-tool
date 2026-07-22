---
description: Historical delivery record for OpsQuest iteration 6.
audience: contributors and maintainers
status: historical
search:
  exclude: true
---

# OpsQuest iteration 6

> Historical audit. See the [documentation map](../../README.md) for current behavior.

Yes—there are worthwhile improvements, but I would not broadly rewrite the project. The current package structure is sensible for 16 missions. The best path is to fix several concrete correctness/resource issues, then introduce a narrow environment boundary before Docker work.

No files were changed during this audit.

## Fix first

1. Virtual `mv` has two correctness bugs.

   - Moving a regular file onto the same resolved path deletes it. The destination is removed before descendants are collected in [filesystem.go](https://github.com/Aleksandergreg/go-cli-tool/blob/main/internal/sandbox/filesystem.go#L280).
   - The current-directory protection is mistakenly in `cp` at [commands_files.go](https://github.com/Aleksandergreg/go-cli-tool/blob/main/internal/sandbox/commands_files.go#L193), while `mv` has no protection. Therefore `cp -r . /out` is rejected, but `mv . /out` can leave `CWD`, `PWD`, and `OLDPWD` pointing at nonexistent paths.

   Move the guard to `mv`, handle identical source/destination explicitly, and retain an explicit root-copy rejection.

2. Script resource limits can be bypassed.

   The 1 MiB script-output limit is checked after a line finishes in [commands_script.go](https://github.com/Aleksandergreg/go-cli-tool/blob/main/internal/sandbox/commands_script.go#L117), but redirected output is already written and cleared in [shell.go](https://github.com/Aleksandergreg/go-cli-tool/blob/main/internal/sandbox/shell.go#L140). `cat x >> x` can therefore double a virtual file repeatedly until the Go process runs out of memory.

   Add centralized limits for:

   - Maximum single-file size
   - Total virtual-filesystem bytes
   - Entry count
   - Pipeline/intermediate output

   Enforce them atomically in `WriteFile` and `AppendFile`.

3. Completionist can unlock prematurely.

   [session.go](https://github.com/Aleksandergreg/go-cli-tool/blob/main/internal/game/session.go#L226) compares `len(Player.Completed)` with the current mission count. Removed or renamed mission IDs still count. The catalog-aware reconciliation in [app.go](https://github.com/Aleksandergreg/go-cli-tool/blob/main/internal/cli/app.go#L64) does this correctly.

   Centralize achievement evaluation in one progression component using only catalog-known mission IDs.

4. Interactive and scripted shell syntax disagree.

   Scripts explicitly reject `;`, `&&`, substitutions, functions, and control syntax, but the interactive lexer treats much of this as ordinary filename/argument text. For example, `touch a; touch b` can create surprising filenames rather than reporting unsupported syntax.

   Move quote-aware syntax validation into the lexer/parser and use it for both interactive and script execution. This removes duplicate grammar logic and improves teaching consistency.

5. Blank-line processing is incorrect.

   [textLines](https://github.com/Aleksandergreg/go-cli-tool/blob/main/internal/sandbox/commands_text.go#L499) treats a file containing exactly `"\n"` as having no lines. This affects `head`, `grep '^$'`, `sort`, `uniq`, `awk`, and `cut`.

6. Failed `tar -c` can leave phantom archive metadata.

   [commands_system.go](https://github.com/Aleksandergreg/go-cli-tool/blob/main/internal/sandbox/commands_system.go#L241) publishes archive metadata before successfully creating the backing virtual file. Preflight and write first, then publish metadata.

## Architectural refactors

The most valuable architectural change before Docker is a narrow lab boundary. Currently:

- `Session` constructs a concrete sandbox in [session.go](https://github.com/Aleksandergreg/go-cli-tool/blob/main/internal/game/session.go#L40).
- Terminal completion accepts `*sandbox.Sandbox` in [input.go](https://github.com/Aleksandergreg/go-cli-tool/blob/main/internal/game/input.go#L52).
- Validators read sandbox internals directly in [validator.go](https://github.com/Aleksandergreg/go-cli-tool/blob/main/internal/game/validator.go#L106).

When Docker work begins, introduce small execution and observation interfaces plus an environment factory. Avoid designing a giant generic interface before the Docker requirements are concrete.

Other useful refactors:

- Replace the dispatcher, command-name list, and manual map in [shell.go](https://github.com/Aleksandergreg/go-cli-tool/blob/main/internal/sandbox/shell.go#L325) with one command registry. It can describe help, script safety, stdin support, and interactive behavior.
- Centralize filesystem mutation through `Sandbox` wrappers so archive metadata cannot drift when adding editors, transforms, or commands.
- Make campaigns first-class objects rather than free-form mission strings. This will matter once Linux, Docker, and Kubernetes have different environment types and prerequisites.
- Replace stringly typed outcome switches with constants or a condition registry. Adding a validator currently requires coordinated edits in catalog validation, evaluation, and descriptions.
- Split `input.go` into terminal decoding, completion, and reader files; split `vi.go` into editor state, key decoding, and rendering. This is organizational, not a performance requirement.

## Performance improvements worth doing

- Save the profile once on a completion command. It is currently synchronously written at [session.go](https://github.com/Aleksandergreg/go-cli-tool/blob/main/internal/game/session.go#L197) and again at line 230.
- Expand pipeline stages once into an execution plan. Vi preflight, script preflight, and execution currently repeat expansion, including virtual glob scans.
- Collapse `du` into one filesystem traversal.
- Add benchmarks before replacing the flat filesystem with a tree or introducing a rope/gap buffer for `vi`. Current missions contain very few paths, so those rewrites are not justified yet.

Profile persistence also needs explicit migrations and invariant validation rather than simply bumping every old version in [profile.go](https://github.com/Aleksandergreg/go-cli-tool/blob/main/internal/profile/profile.go#L62). Concurrent OpsQuest processes are currently last-writer-wins.

## Testing gaps

The canonical mission test indexes solutions by mission number and does not assert that an entry exists. A missing solution can pass if a mission accidentally starts completed. Use stable mission IDs and assert:

- Every mission has a non-empty canonical solution.
- Every mission starts incomplete.
- Appropriate alternative solutions succeed.
- Representative incorrect/incomplete solutions fail.

I would also add filesystem invariant tests and fuzz tests for lexing, parsing, path resolution, copy/move, and archive metadata.

One smaller CLI bug: in-mission `list --campaign "First Day"` cannot work because [missionNavigationFields](https://github.com/Aleksandergreg/go-cli-tool/blob/main/internal/game/session.go#L257) uses `strings.Fields`, which ignores shell quoting.

## Validation observed

- `go test -cover ./...` passed.
- `go vet ./...` passed.
- Sandbox focused tests passed.
- Coverage: CLI 57.4%, game 70.2%, mission 73.3%, profile 53.7%, sandbox 64.4%.
- Git remained clean.

My recommended next implementation batch is: filesystem move invariants, sandbox resource quotas, blank-line handling, transactional archive metadata, and completionist correctness.
