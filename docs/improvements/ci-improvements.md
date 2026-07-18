Existing duplicate runs remain visible historically, but the next PR update should produce only the (pull_request) check.

  ## Recommended CI/CD roadmap

  1. Release Please—recommended next, after version cleanup.

     OpsQuest currently hardcodes 0.3.0 in internal/cli/app.go, has no tags, and no CHANGELOG.md. First establish one authoritative version source, then bootstrap Release Please at 0.3.0 with a deliberate starting
     commit. Release Please supports Go repositories and generates release PRs, tags, changelogs, and GitHub Releases from Conventional Commits. Release Please (https://github.com/googleapis/release-please)

     Adopt squash merges with PR titles such as:

     feat: add Docker log missions
     fix: prevent duplicate CI runs
     docs: explain installation

     One caveat: resources created with the default GITHUB_TOKEN may not automatically trigger downstream workflows. Either build release assets in the same workflow or use a narrowly scoped GitHub App/PAT. Release
     Please action authentication (https://github.com/googleapis/release-please-action)

  2. Release binaries with GoReleaser.

     Build archives for:
      - macOS arm64 and amd64
      - Linux arm64 and amd64
      - Optionally Windows amd64

     Include LICENSE, THIRD_PARTY_NOTICES.md, and SHA-256 checksums. GoReleaser handles cross-platform archives and GitHub Release uploads with relatively little custom scripting. GoReleaser quick start
     (https://www.goreleaser.com/getting-started/quick-start/)

  3. Add Dependabot with low noise.

     Configure monthly or grouped weekly updates for:
      - gomod
      - github-actions

     Use a low open-PR limit and avoid auto-merge initially. Dependabot can maintain both Go modules and Action references. GitHub Dependabot documentation
     (https://docs.github.com/en/code-security/how-tos/secure-your-supply-chain/secure-your-dependencies/auto-update-actions)

  4. Enable CodeQL default setup.

     This is lower-maintenance than adding another workflow file and scans Go on PRs, default-branch pushes, and a schedule. CodeQL default setup
     (https://docs.github.com/en/code-security/concepts/code-scanning/setup-types)

  5. Add govulncheck.

     A weekly/manual govulncheck ./... provides Go-specific, call-aware vulnerability detection. I would initially keep it separate from the required PR gate until its behavior is established. Official Go
     vulnerability tooling (https://go.dev/doc/tutorial/govulncheck)

  6. Add real Docker lifecycle testing separately.

     Run make docker-integration through workflow_dispatch and perhaps weekly scheduling. Keep it outside required PR CI because it needs a daemon, network image availability, and creates more potential flakiness.

  7. Add release provenance later.

     Once downloadable binaries exist, GitHub artifact attestations can provide verifiable build provenance. They are useful for releases, but unnecessary for routine PR binaries. GitHub artifact attestations
     (https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations)

  Validation completed:

  - Workflow YAML parsed successfully
  - make check-all passed, including tests, vet, build, smoke testing, and race detection
  - git diff --check passed
  - actionlint was not run because it is not installed

  Final status: branch fix/ci-running-twice, with only .github/workflows/ci.yml and README.md intentionally modified.


