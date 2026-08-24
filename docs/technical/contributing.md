---
description: OpsQuest package boundaries, task-specific workflows, local validation, and documentation-site development.
audience: contributors and maintainers
status: current
---

# Contributing and quality gates

Start with the repository [`AGENTS.md`](https://github.com/Aleksandergreg/go-cli-tool/blob/main/AGENTS.md). It defines the safety invariants, package boundaries, authorized local work, and definition of done.

## Choose the owning package

| Change | Primary area |
| --- | --- |
| CLI command, flag, route, or presentation | `internal/cli` |
| Attempt controls, rewards, progression, or outcome evaluation | `internal/game` |
| Mission schema, catalog, worlds, or JSON content | `internal/mission` |
| Teaching-shell parsing, files, processes, archives, or commands | `internal/sandbox` |
| Optional Docker actions, fixtures, observation, or cleanup | `internal/dockerlab` |
| Local browser companion, pairing, HTTP projection, or assets | `internal/webapp` |
| Durable progress and migrations | `internal/profile` |
| Terminal color policy | `internal/ui` |

Use `$add-mission`, `$extend-sandbox-command`, or `$prepare-iteration` when the task matches one of those repository workflows.

## Local validation

| Scope | Command |
| --- | --- |
| Focused Go package | `go test ./internal/PACKAGE` |
| Mission catalog and canonical outcomes | `make validate-missions` |
| Agent instructions and skill structure | `make check-agent-docs` |
| Deterministic CLI path | `make smoke-test` |
| Documentation consistency | `make check-docs` |
| Ordinary repository gate | `make check` |
| Comprehensive gate with race detection | `make check-all` |
| Real Docker lifecycle | `make docker-integration` |
| Real OrbStack lifecycle | `make orbstack-integration` |
| Hosted documentation | `make docs-check` |
| GitHub repository governance | `make tofu-check` |

Run `make check-all` for release-sized, persistence, parser, sandbox, or concurrency-sensitive work. Docker adapter changes also run the real integration target when prerequisites are available. `make docker-integration` uses the active Docker context; `make orbstack-integration` explicitly selects OrbStack's `orbstack` context.

## Work on this site

Zensical requires Python 3.10 or newer. Use a project virtual environment so the pinned alpha version does not affect other Python tools:

```console
$ python3 -m venv .venv
$ . .venv/bin/activate
$ python -m pip install -r requirements-docs.txt
$ make docs-serve
```

Before committing documentation changes:

```console
$ make check-docs
$ make docs-check
```

The dependency-free check validates navigation ownership, page metadata,
mission counts, Go requirements, repository/site boundaries, and learning
diagram facts. The strict Zensical build validates public pages and anchors and
writes the static site to ignored `site/`. If a suitable Python is not
installed, the official pinned container can run the same site check:

```console
$ docker run --rm -v "$PWD:/docs" zensical/zensical:0.0.51 build --clean --strict
```

The Pages workflow runs the same Make target on documentation pull requests. Merges to `main` upload the static artifact and deploy through the protected `github-pages` environment.

## Documentation lifecycle

- Put player instructions and the current mission map in `docs/play/`.
- Put learning and system explanations in `docs/game/` or `docs/technical/`.
- Put contributor workflows and compatibility contracts in `docs/technical/`.
- Put only unfinished work in `docs/roadmap/` with an explicit status.
- Put implemented decisions in `project/decisions/`.
- Put point-in-time reports in `project/history/iterations/`; they are
  repository records, not public site pages.
- Keep infrastructure runbooks beside the infrastructure they operate.
- List every public Markdown page explicitly in `mkdocs.yml`.
- Update an editable diagram source and its matching SVG together.

Review docs claims in the same pull request as the behavior they describe. Cross-link shared explanations instead of maintaining copies.
