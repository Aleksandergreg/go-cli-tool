---
description: Historical delivery record for OpsQuest iteration 14.
audience: contributors and maintainers
status: historical
search:
  exclude: true
---

# OpsQuest iteration 14

> Historical delivery record. See the [documentation map](../../README.md) for current behavior.

Date: 2026-07-22
Version: Unreleased (current executable: 0.4.0)

## Summary

OpsQuest now has a Zensical documentation site organized around player, game-design, technical, and project audiences. A strict GitHub Actions build validates pull requests, while merges to `main` produce and deploy a GitHub Pages artifact.

## Delivered

- Added a pinned, Material-compatible Zensical configuration with explicit navigation, repository links, local search, Mermaid support, and strict internal-link and anchor validation.
- Added `docs-build`, `docs-check`, and `docs-serve` Make targets plus an isolated generated-site directory, giving contributors and CI one shared documentation interface.
- Added a player route for quick start, mission flow, controls, Linux worlds, and the optional Docker Foundations lab.
- Added game-design guides for learning philosophy, outcome-based mission design, progression, rewards, and the complete curriculum map.
- Added technical guides for architecture, sandbox boundaries, declarative mission content, profile compatibility, and contribution quality gates.
- Reduced the root README to the repository landing path and moved long-form material into the hosted information architecture without duplicating authoritative explanations.
- Added lifecycle and audience metadata to current, roadmap, and historical material; iteration reports remain available through a quiet archive and are excluded from search.
- Added a least-privilege GitHub Pages workflow that validates documentation on pull requests and deploys only from `main` through the protected `github-pages` environment.
- Converted repository-source references to explicit GitHub URLs so they remain valid from generated Pages routes.

## Compatibility and safety

- CLI commands, flags, terminal presentation, mission behavior, Docker behavior, and release version metadata are unchanged.
- The mission JSON schema, embedded catalog, validators, profile schema, and persisted profile version are unchanged; no migration is required.
- No third-party production dependency was added. Zensical 0.0.51 is an exactly pinned documentation-only build dependency running in Python 3.10 or newer.
- Linux player input remains confined to the in-memory teaching shell, and Docker input remains confined to the typed, labeled lab boundary. The documentation workflow does not execute player input or broaden runtime permissions.
- Existing Markdown remains the content source. The `mkdocs.yml` layout is deliberately compatible with Material for MkDocs so a generator fallback does not require a content migration.

## Validation

| Command | Observed result |
| --- | --- |
| `make docs-check ZENSICAL=/tmp/opsquest-doc-venv/bin/zensical` | PASS; Zensical 0.0.51 on Python 3.13.14 completed a clean strict build with no link or anchor issues. |
| `/tmp/opsquest-release-tools/actionlint .github/workflows/docs.yml` | PASS; the GitHub Pages workflow produced no diagnostics under actionlint 1.7.12. |
| `/tmp/opsquest-doc-venv/bin/python -c 'import yaml; ...'` | PASS; `mkdocs.yml` and the Pages workflow parsed as YAML. |
| `make check-all` | PASS after granting the managed environment access to Go's normal caches; agent docs, mission/all-package tests, vet, build, smoke test, and race detection passed. |
| `git diff --check` | PASS; no whitespace errors were reported. |
| `gh run watch 29906838299 --exit-status --interval 5` | PASS; the `main` workflow built the strict Zensical site, configured Pages, uploaded the artifact, and completed the Pages deployment. |
| `curl -L --fail --silent --show-error --head https://aleksandergreg.github.io/go-cli-tool/` | PASS; the public HTTPS endpoint returned HTTP 200 with the deployed OpsQuest page. |

The first sandboxed `make check-all` attempt could not update Go's module/stat cache and therefore could not compile the race packages; rerunning the same gate with normal Go cache access passed. The first post-merge Pages run built the site but exposed a missing `pages: read` permission on the build job; the least-privilege follow-up passed `actionlint`, the normal repository gate, the pull-request documentation build, and the replacement deployment. The real-Docker integration target was not run because runtime and Docker behavior were unchanged. GoReleaser checks were not run because release configuration and packaging were unchanged.

## Remaining work

- Reassess the pinned Zensical version after real maintenance use; retain Material for MkDocs as the low-migration fallback while Zensical is alpha.
- Consider a custom domain, analytics, or a more branded interactive front end only after there is a concrete product need.

## Final repository state

The rollout merged through [PR #14](https://github.com/Aleksandergreg/go-cli-tool/pull/14), with the build-token permission correction merged through [PR #15](https://github.com/Aleksandergreg/go-cli-tool/pull/15). Generated `site/` and `bin/` output remains ignored, and no unrelated working-tree change was discarded.
