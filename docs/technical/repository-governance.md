---
description: Declarative GitHub settings, rulesets, credential boundaries, OpenTofu state, and the reviewed apply procedure.
audience: maintainers
status: current
---

# Repository governance

OpsQuest keeps GitHub repository policy in OpenTofu under
[`infra/github`](https://github.com/Aleksandergreg/go-cli-tool/tree/main/infra/github).
Committing that configuration does not change GitHub by itself. A maintainer
must authenticate, inspect an OpenTofu plan, and apply it from a trusted
operator environment.

## Managed controls

The configuration owns the following settings:

- squash-only pull-request merges, auto-merge, update-branch suggestions, and
  automatic deletion of merged head branches;
- a default-branch ruleset that blocks deletion and force pushes, requires a
  pull request, requires linear history and resolved conversations, and
  requires the GitHub Actions `Local quality gate` from the GitHub Actions app
  for non-admins; repository admins retain a full break-glass bypass;
- an immutable-after-creation `v*` release-tag policy that remains compatible
  with Release Please creating new tags;
- read-only default workflow-token permissions, an explicit allowlist for
  third-party actions, mandatory immutable action SHA references, and no
  persisted checkout credentials in subsequent workflow steps;
- an independent required Gitleaks job that scans full Git history on pull
  requests, `main`, a weekly schedule, and manual dispatch;
- Dependabot vulnerability alerts, security updates, and bounded weekly Go,
  GitHub Actions, and OpenTofu update pull requests.

The ruleset requires zero approving reviews because this is currently a solo
maintainer repository. Requiring one approval would make the owner unable to
merge their own pull request. Increase the count when an independent reviewer
is consistently available.

CodeQL is deliberately not a required check yet. Keep its advanced workflow
enabled and make it required only after its hosted analysis is reliably green;
do not enable CodeQL default setup alongside the repository workflow.

## Credential boundary

No API token, application private key, backend credential, or secret variable
belongs in Git, an OpenTofu variable-value file, a saved plan, or a command
line argument.

For a local bootstrap, prefer the GitHub CLI credential store. A repository-
scoped, expiring fine-grained personal access token supplied through the
`GITHUB_TOKEN` environment variable is the fallback. It needs repository
Administration read/write permission for settings and rulesets. Unset the
environment variable when the operation is complete.

For unattended administration, use a dedicated GitHub App installed only on
this repository. Grant only the endpoint permissions required by the plan,
keep its private key in a secrets manager, and mint short-lived installation
tokens in the trusted runner. Run automated administration from a separate,
private administration repository or a protected infrastructure workspace
rather than giving a workflow in the managed repository its own administration
key.

The current resources need repository Administration read/write access. When
using GitHub App authentication, provider 6.13 also needs repository Contents
read/write access to read and update merge settings; this is a provider/API
constraint, not permission for an apply job to modify source code. Do not grant
access to Actions secrets or unrelated repositories.

The GitHub provider accepts credentials at runtime and does not need a `token`
argument in HCL. The checked-in provider block intentionally contains only the
repository owner.

Local `.env` files are ignored as a guardrail, but ignore rules are not a
secret store. Application or deployment secrets belong in a password manager,
the operating-system keychain, or GitHub Actions environment secrets. Prefer
OIDC federation over stored cloud credentials whenever a future deployment
platform supports it.

## OpenTofu state

OpenTofu state and saved plans can reveal repository metadata and resource
identifiers. They are treated as sensitive even though provider credentials
are not written into state.

The local ignore file excludes `.terraform/`, state, plans, variable-value
files, crash logs, and override files. That is sufficient for formatting and
schema validation, but not for a shared or automated apply. Before the first
real apply, configure a remote backend that provides encryption at rest,
locking, version history, access control, and audit logging. Keep backend
credentials in the backend platform's credential mechanism rather than in
`-backend-config` values or checked-in files.

Commit `.terraform.lock.hcl`. It records the reviewed provider version and
package checksums; it is not a secret.

## Validate without credentials

OpenTofu validation initializes providers without contacting the managed
GitHub repository:

```console
$ make tofu-check
```

The required `Local quality gate` performs the same check in GitHub Actions
before running the comprehensive Go gate. Pull-request workflows receive no
repository-administration credential.

The `Secret leak scan` downloads a fixed Gitleaks release, verifies its pinned
SHA-256 before execution, and reports findings with the credential fully
redacted. It does not upload a report artifact or pass the scanner a GitHub API
token. The complete history checkout ensures that deleting a secret in a later
commit does not hide the original exposure.

## First plan and apply

Prerequisites are OpenTofu 1.12.x, an encrypted locking backend, and a GitHub
plan that supports rulesets for a private repository.

1. Merge this configuration and both required workflows to `main` before
   activating the rulesets. Otherwise a required check may not exist yet.
2. Inspect existing rulesets in GitHub before applying. Rulesets layer, so an
   unmanaged rule can make the effective policy more restrictive than this
   configuration.
3. Authenticate in a trusted shell without placing a token in shell history.
4. Initialize the configured remote backend.
5. Create and inspect a saved plan.
6. Apply exactly that reviewed plan.

```console
$ tofu -chdir=infra/github init -reconfigure
$ tofu -chdir=infra/github plan -out=opsquest.tfplan
$ tofu -chdir=infra/github show opsquest.tfplan
$ tofu -chdir=infra/github apply opsquest.tfplan
```

The declarative import block adopts the existing repository on the first
apply. A correct first plan must show an import, not repository replacement or
visibility change. Stop if it proposes deletion, repository recreation, or an
unrelated feature change.

After applying, open a test pull request and confirm that `Local quality gate`
and `Secret leak scan` are required, squash merge is the only merge method, and
the merged head branch is deleted. Confirm that non-admins cannot push directly
or force push to `main`, while a repository admin can deliberately bypass the
ruleset.

## Recovery and release automation

Repository administrators have an always-available bypass for this solo
repository. It permits direct pushes, force pushes, and other updates that the
default-branch ruleset would otherwise reject; all non-admins remain subject to
the full ruleset. Prefer the normal pull-request path, treat bypass as
break-glass, record why it was used, and use `--force-with-lease` instead of
`--force` when rewriting remote history.

Release Please uses the built-in repository token. GitHub may place workflows
triggered by its release pull request in an approval-required state. Approve
those runs manually before merging. If unattended release pull requests become
necessary, replace that token with a short-lived token from a narrowly scoped
GitHub App; do not add a long-lived personal token to the workflow.

Apply governance changes manually after reviewing `tofu plan`. CI should
format and validate this configuration, but it must not apply changes from a
pull request or expose an administration credential to untrusted code.

## If a secret is exposed

Treat every committed or logged credential as compromised. Revoke or rotate it
first, review provider audit logs for misuse, then remove it from the current
tree and, where appropriate, rewrite history. Removing the text without
rotating the credential is not remediation. Record the incident without
copying the secret into an issue, pull request, artifact, or chat transcript.
