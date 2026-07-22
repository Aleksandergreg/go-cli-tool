---
description: Historical delivery record for OpsQuest iteration 1.
audience: contributors and maintainers
status: historical
search:
  exclude: true
---

# OpsQuest iteration 1

> Historical delivery record. See the [documentation map](../../README.md) for current behavior.

OpsQuest v0.1 is now fully playable.

Implemented:

- Ten Linux missions with stories, hints, explanations, and XP
- Safe in-memory terminal sandbox—player commands never reach the host shell
- Outcome-based validation for files, output, permissions, processes, archives, and environment variables
- Pipelines, quoting, variables, globbing, and redirection
- Persistent profiles, ranks, command mastery, hint penalties, replay protection, and reset
- `play`, `list`, `profile`, `commands`, and `reset`
- Dependency-free declarative JSON missions
- Complete documentation in [README.md](https://github.com/Aleksandergreg/go-cli-tool/blob/main/README.md)

Run it with:

```console
go run ./cmd/opsquest play
```

Verification passed:

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- Production build
- Scripted completion of all ten missions, including the `find -exec grep` exercise and final pipeline boss mission.
