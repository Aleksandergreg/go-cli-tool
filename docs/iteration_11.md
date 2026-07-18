# OpsQuest iteration 11

Date: 2026-07-18
Version: unreleased; executable baseline remains 0.3.0

## Summary

OpsQuest now has a repository-managed release and security pipeline. Release Please prepares semantic-version release pull requests, GoReleaser creates checked cross-platform archives, and CodeQL analyzes the real Go build path without changing gameplay or requiring Docker.

## Delivered

- Bootstrapped Release Please at the existing 0.3.0 baseline and excluded earlier non-conventional history with an explicit commit boundary.
- Centralized the executable version in `internal/buildinfo.Version`; release pull requests update that constant, the manifest, changelog, and the version statement in `initial_prompt.md`.
- Added a serialized release workflow that creates or updates the release pull request and, after that pull request is merged, creates the tag/release and attaches GoReleaser artifacts in the same job.
- Added reproducible GoReleaser builds for macOS and Linux on amd64/arm64 and Windows on amd64, with SHA-256 checksums and the README, changelog, Beer-Ware license, and third-party notices in every archive.
- Added repository-managed CodeQL advanced setup for pull requests, `main`, a weekly schedule, and manual dispatch. Manual build mode reuses `make build`.
- Added `make release-check` and `make release-snapshot`, ignored generated `dist/` output, upgraded the existing checkout/setup-go actions to their current supported majors, and documented the operational release workflow and remaining CI/CD roadmap.

## Compatibility and safety

- CLI commands and output remain compatible; `opsquest version` still reports 0.3.0 through the new centralized constant.
- Mission schema, catalog content, profile format, sandbox behavior, Docker adapter behavior, and host-isolation boundaries are unchanged.
- No production Go dependency was added or changed. GoReleaser and actionlint were installed only into a temporary validation directory and are not part of `go.mod`.
- Release automation uses the repository token and least-privilege job permissions. It does not add package-manager publication, Docker/Kubernetes gameplay, host command execution, or automatic deployment.
- The first release after this bootstrap is derived only from conventional commits after `2fbb97c7d421ab60a6471e2ff6079ed6d7092d40`; before 1.0, breaking changes intentionally produce a minor bump.

## Validation

| Command | Observed result |
| --- | --- |
| `env GOCACHE=/tmp/opsquest-release-gocache go test ./internal/buildinfo ./internal/cli` | PASS; the new version source and CLI integration compiled and the CLI package tests passed. |
| Ruby JSON and YAML parsing for both Release Please files, `.goreleaser.yml`, and all workflow files | PASS; every file parsed successfully. |
| `make check-agent-docs` | PASS; `AGENTS.md` and all three repository skills validated. |
| `/tmp/opsquest-release-tools/actionlint -color` | PASS; all GitHub Actions workflows passed static workflow validation with actionlint 1.7.12. |
| `env GORELEASER=/tmp/opsquest-release-tools/goreleaser make release-check` | PASS; GoReleaser 2.17.0 validated one configuration file. |
| `env GORELEASER=/tmp/opsquest-release-tools/goreleaser GOCACHE=/tmp/opsquest-release-gocache GOMODCACHE=/tmp/opsquest-release-project-gomodcache GOPROXY=file:///Users/aleksandergregersen/go/pkg/mod/cache/download GOSUMDB=off make release-snapshot` | PASS; five binaries and archives were built without publishing. |
| `shasum -a 256 -c opsquest_0.0.0-SNAPSHOT-2fbb97c_checksums.txt` from `dist/` | PASS; all five release archive checksums verified. |
| `./dist/opsquest_darwin_arm64_v8.0/opsquest version` | PASS; the packaged native binary reported `OpsQuest 0.3.0`. |
| `env GOCACHE=/tmp/opsquest-release-gocache GOMODCACHE=/tmp/opsquest-release-project-gomodcache GOPROXY=file:///Users/aleksandergregersen/go/pkg/mod/cache/download GOSUMDB=off make check-all` | PASS; agent docs, mission validation/all Go tests, vet, build, isolated smoke test, and race detection all passed. |
| `./bin/opsquest version` | PASS; the normal release build reported `OpsQuest 0.3.0`. |
| `git diff --check` | PASS; no whitespace errors. |

The first focused `go test ./internal/buildinfo ./internal/cli` attempt without a temporary `GOCACHE` could not access the sandboxed macOS Go build cache. The same test passed with the task-specific `/tmp` cache shown above. An initial checksum invocation from the repository root could not resolve the checksum file's `dist/`-relative entries; running it from `dist/` verified every archive.

The hosted Release Please, GitHub Release upload, and CodeQL SARIF upload were not run locally because they require GitHub Actions event context and repository permissions. No publishing command was run. The real Docker lifecycle target was not run because no Docker gameplay or adapter behavior changed and it is intentionally outside the portable `make check-all` gate.

## Remaining work

- Enable the repository setting that allows GitHub Actions to create pull requests. Approve release-PR checks when GitHub marks repository-token-created pull requests as approval-required, or introduce a narrowly scoped GitHub App only if unattended checks are needed.
- Ensure CodeQL default setup is disabled before the new advanced workflow runs, then observe its first hosted analysis and tune only if it produces actionable evidence.
- Observe the first real release end to end. The archives are currently unsigned and macOS binaries are not notarized.
- Consider grouped Dependabot updates, `govulncheck`, immutable action SHA pins, and artifact attestations as separate follow-up work after this pipeline has operated successfully.

## Final repository state

The final review is on `feature/ci-hardening`. The working tree contains only intentional release, CodeQL, buildinfo, Makefile, harness, and documentation changes from this iteration; generated `bin/` and `dist/` artifacts remain ignored.
