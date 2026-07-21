# OpsQuest iteration 12

> Historical delivery record. See the [documentation map](../../README.md) for current behavior.

Implemented Release Please, GoReleaser, and CodeQL without changing gameplay, mission data, profiles, or production Go dependencies.

### What changed

- [release.yml](../../../.github/workflows/release.yml) maintains release PRs and attaches GoReleaser artifacts after release creation.
- [release-please-config.json](../../../release-please-config.json) bootstraps from version 0.3.0 and excludes older history.
- [.release-please-manifest.json](../../../.release-please-manifest.json) records the current release baseline.
- [CHANGELOG.md](../../../CHANGELOG.md) is now Release Please-managed.
- [.goreleaser.yml](../../../.goreleaser.yml) builds:
  - macOS amd64/arm64
  - Linux amd64/arm64
  - Windows amd64
  - SHA-256 checksums and archives containing README, changelog, Beer-Ware license, and third-party notices
- [codeql.yml](../../../.github/workflows/codeql.yml) runs manual Go analysis through `make build` on PRs, `main`, weekly, and manually.
- [version.go](../../../internal/buildinfo/version.go) is now the authoritative executable version source.
- [Makefile](../../../Makefile) adds `release-check` and `release-snapshot`.
- Existing Actions were upgraded to `checkout@v6` and `setup-go@v6`.
- README, agent guidance, `initial_prompt.md`, the CI/CD roadmap, and [iteration 11](iteration-11.md) document the new process.

Release Please and GoReleaser intentionally share one workflow because events created using the default repository token do not start another workflow. [Release Please action guidance](https://github.com/googleapis/release-please-action)

### Validation observed

- Focused CLI/buildinfo tests: passed.
- Release Please JSON and workflow/GoReleaser YAML parsing: passed.
- `make check-agent-docs`: passed.
- `actionlint` 1.7.12 across all workflows: passed.
- `make release-check` with GoReleaser 2.17.0: passed.
- `make release-snapshot`: passed; five platform archives generated.
- SHA-256 verification: all five archives passed.
- Packaged macOS binary: reported `OpsQuest 0.3.0`.
- `make check-all`: passed all tests, mission validation, vet, build, smoke testing, and race detection.
- `./bin/opsquest version`: reported `OpsQuest 0.3.0`.
- `git diff --check`: passed.

The initial focused test could not write to the sandboxed default Go cache; rerunning with a temporary `/tmp` cache passed. An initial checksum command was run from the wrong directory; rerunning from `dist/` verified every archive.

Hosted Release Please publication and CodeQL SARIF upload were not run because they require GitHub Actions context and repository permissions. No release was published. Docker integration was not run because Docker behavior was unchanged and remains outside the portable quality gate.

### Repository setup still needed

- Allow GitHub Actions to create pull requests.
- Disable CodeQL default setup if it is currently enabled; the new workflow is the repository-managed advanced setup. [GitHub CodeQL setup types](https://docs.github.com/en/code-security/concepts/code-scanning/setup-types)
- Approve checks for repository-token-created release PRs if GitHub requests it.
- Use Conventional Commit squash titles such as `fix:` and `feat:`.

Current Git status: `feature/ci-hardening`, with 10 intentionally modified and 8 new files from this implementation. Generated `bin/` and `dist/` artifacts are ignored.
