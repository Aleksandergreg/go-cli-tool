---
description: Delivered OpsQuest release, security, and documentation automation plus possible follow-up improvements.
audience: contributors and maintainers
status: partially delivered
---

# CI/CD improvements

Status: the release and security foundation is implemented; the second section tracks possible follow-up work.

## Implemented

- The required CI quality gate runs once for a pull request and once after its merge to `main`; feature-branch pushes no longer duplicate the pull-request run.
- Release Please maintains a version/changelog pull request from Conventional Commits, then creates `vX.Y.Z` tags and GitHub releases. The 0.3.0 bootstrap deliberately excludes older repository history.
- GoReleaser attaches macOS, Linux, and Windows archives plus SHA-256 checksums to the Release Please release. It packages `LICENSE` and `THIRD_PARTY_NOTICES.md`, and has non-publishing local validation targets.
- Repository-managed CodeQL advanced setup analyzes the same `make build` path on pull requests, `main`, a weekly schedule, and manual dispatch.
- A dedicated documentation workflow strictly builds Zensical content on pull requests and deploys the static artifact to GitHub Pages after relevant merges to `main`.
- Repository settings, Actions permissions, security controls, and branch/tag rulesets are declared in pinned OpenTofu configuration with an explicit credential, state, review, and break-glass model. Remote activation remains a reviewed operator apply rather than a pull-request side effect.
- Dependabot groups weekly Go module, GitHub Actions, and OpenTofu updates with bounded pull-request volume and no automatic merging.
- Every workflow action is pinned to an immutable commit; Dependabot maintains those references, and the declared Actions policy requires SHA pins.
- An independent required job scans full Git history for committed credentials with a checksum-verified Gitleaks binary, fully redacted findings, and no report artifacts or scanner API token.

The release flow deliberately keeps Release Please and GoReleaser in one workflow. GitHub does not start a second workflow from a tag or release created with the default `GITHUB_TOKEN`, so a separate GoReleaser workflow would silently miss normal releases.

## Sensible next steps

1. Add a scheduled and manually dispatched `govulncheck ./...` job. Keep it advisory until the repository has established how toolchain and database updates affect reliability.
2. Add GitHub artifact attestations for released archives once the release workflow has completed at least one real release.
3. Add opt-in scheduled Docker lifecycle testing only when the pinned fixture image and daemon behavior are reliable enough for hosted CI; keep it outside the portable required gate.
4. Evaluate signed checksums, macOS signing/notarization, and a Homebrew tap after there is evidence of external binary distribution. These add credentials and maintenance cost, so they should follow actual demand.

Do not enable CodeQL default setup while `.github/workflows/codeql.yml` is active. It is an advanced configuration, and simultaneous setup can block or duplicate analysis uploads.
