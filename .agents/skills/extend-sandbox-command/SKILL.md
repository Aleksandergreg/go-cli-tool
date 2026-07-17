---
name: extend-sandbox-command
description: "Use when adding a simulated shell command or changing parsing, flags, path handling, pipelines, redirection, or command errors in internal/sandbox. Do not use for mission-only content or real host command execution."
---

# Extend the simulated shell

Read [references/shell-semantics.md](references/shell-semantics.md) before changing parser or command behavior. OpsQuest implements a deliberate teaching subset, not a transparent proxy to the operating system.

## Workflow

1. Map the existing behavior.
   - Inspect `internal/sandbox/shell.go`, `sandbox.go`, `filesystem.go`, the relevant `commands_*.go`, and `shell_test.go`.
   - Trace lexing, parsing, word expansion, stage execution, dispatcher registration, virtual path resolution, and error wrapping.
   - Compare a semantically similar command and read its focused manual entry. Check missions and canonical solutions that already depend on the affected behavior.

2. Define the supported teaching subset explicitly.
   - Before implementation, list accepted operands and flags, stdin/file behavior, output format, state mutations, error cases, and deliberately unsupported real-world features.
   - Implement only the semantics needed to teach the concept accurately. Return a clear unsupported or usage error for omitted behavior instead of silently approximating dangerous or misleading semantics.

3. Preserve shell consistency and isolation.
   - Keep every path operation in `FileSystem` through `Sandbox.Resolve`; keep processes and archives in their virtual maps.
   - Never invoke `os/exec`, a host shell, host filesystem APIs, or host process APIs for player commands.
   - Respect current quote-aware globbing, variable expansion, pipeline stdin/stdout, stage-local redirection, and wrapped command errors where they apply.
   - If parser semantics must change, test their interaction with paths, quoting, pipelines, input/output/append redirection, variables, globbing, comments, and malformed input.

4. Implement through existing boundaries.
   - Put command behavior in the closest `commands_*.go` file or a narrowly named new file.
   - Register the command in `Sandbox.run` and in the no-argument command list.
   - Add or update its `commandManuals` entry so `help COMMAND` and `man COMMAND` describe the actual subset.
   - Keep reusable virtual-filesystem behavior in `filesystem.go`; do not bypass it inside a command handler.

5. Add tests before relying on the command in content.
   - Cover successful direct use and stdin/pipeline use where applicable.
   - Cover path forms, quoting or globbing, redirection, and state changes relevant to the command.
   - Cover boundary values, unknown flags, missing operands/files, invalid input, and explicitly unsupported behavior.
   - Add a mission-level integration or canonical-solution test when a mission depends on the new behavior.
   - Retain or strengthen a host-isolation assertion for any change near dispatch, path access, processes, or parsing.

6. Update user-visible command documentation.
   - Keep `shellHelp`, `commandManuals`, README's supported-command list, mission hints, and examples consistent with the implementation.
   - Record material unsupported behavior in focused help or README when a learner could reasonably mistake the subset for full shell compatibility.

7. Validate in increasing scope.
   - Run `go test ./internal/sandbox` first.
   - Run `go test ./internal/game ./internal/mission` when mission behavior is affected.
   - Run `make check-all` for parser, dispatcher, filesystem, process, archive, or isolation changes; otherwise run at least `make check`.

Report the supported subset, intentional omissions, safety impact, user-facing help changes, mission dependencies, and exact validation results.
