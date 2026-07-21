# CI/CD improvements

Status: the release and security foundation is implemented; the second section tracks possible follow-up work.

## Implemented

- The required CI quality gate runs once for a pull request and once after its merge to `main`; feature-branch pushes no longer duplicate the pull-request run.
- Release Please maintains a version/changelog pull request from Conventional Commits, then creates `vX.Y.Z` tags and GitHub releases. The 0.3.0 bootstrap deliberately excludes older repository history.
- GoReleaser attaches macOS, Linux, and Windows archives plus SHA-256 checksums to the Release Please release. It packages `LICENSE` and `THIRD_PARTY_NOTICES.md`, and has non-publishing local validation targets.
- Repository-managed CodeQL advanced setup analyzes the same `make build` path on pull requests, `main`, a weekly schedule, and manual dispatch.

The release flow deliberately keeps Release Please and GoReleaser in one workflow. GitHub does not start a second workflow from a tag or release created with the default `GITHUB_TOKEN`, so a separate GoReleaser workflow would silently miss normal releases.

## Sensible next steps

1. Add low-noise Dependabot groups for `gomod` and `github-actions`, with a small open-pull-request limit and no automatic merging.
2. Add a scheduled and manually dispatched `govulncheck ./...` job. Keep it advisory until the repository has established how toolchain and database updates affect reliability.
3. Pin third-party actions to full commit SHAs and let Dependabot maintain those pins, reducing exposure to mutable major-version tags.
4. Add GitHub artifact attestations for released archives once the release workflow has completed at least one real release.
5. Add opt-in scheduled Docker lifecycle testing only when the pinned fixture image and daemon behavior are reliable enough for hosted CI; keep it outside the portable required gate.
6. Evaluate signed checksums, macOS signing/notarization, and a Homebrew tap after there is evidence of external binary distribution. These add credentials and maintenance cost, so they should follow actual demand.

Do not enable CodeQL default setup while `.github/workflows/codeql.yml` is active. It is an advanced configuration, and simultaneous setup can block or duplicate analysis uploads.
