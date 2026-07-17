Implemented.

Inside a mission you can now run:

```console
list --completed
play 3
next
previous
```

The `opsquest` prefix also works:

```console
opsquest list --completed
opsquest play 3
```

Listing preserves the current sandbox. Switching missions resets the current temporary sandbox but keeps profile progress, XP, hints, and mastery.

Terminal editing now supports:

- Left/Right and Up/Down
- Home/End and Ctrl-A/E
- Option/Ctrl-Left/Right word movement
- Command-Left/Right when the terminal emits standard modifier sequences
- Backspace, forward Delete, Option-Backspace, and Ctrl-W
- Tab completion and bracketed-paste isolation

Validation passed:

- `go test ./internal/game ./internal/cli`
- `make smoke-test`
- `make check-all`, including race detection
- `git diff --check`

Seven intentional files are modified; no unrelated changes were touched.

Your currently running OpsQuest process contains the old code. Run `quit`, then restart it:

```console
go run ./cmd/opsquest play
```
