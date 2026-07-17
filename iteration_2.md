OpsQuest is now expanded to v0.2.0 with a clean working tree.

Highlights:

- 16 Linux missions across three campaigns
- New “Production Friday” multi-step boss incident
- Added `sed`, `tr`, `du`, `stat`, history, numeric sort, richer grep, and command-specific help
- Six learning-focused achievements
- Mission `status` and `restart` controls
- Mission previews, campaign filters, profile naming, and `doctor` diagnostics
- Fixed quoted globbing, pipeline redirection, archive traversal, stale archive metadata, unsafe copies/moves, nested command mastery, `wc -l`, `tar` parsing, and CLI help behavior
- Strict mission schema and setup validation

Documentation is updated in [README.md](/Users/aleksandergregersen/privat/go-cli-tool/README.md).

Verification completed:

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- Release build
- Full scripted 16-mission campaign: 1,230 XP, Senior SRE, 6/6 achievements

Run it with:

```console
go run ./cmd/opsquest play
```
