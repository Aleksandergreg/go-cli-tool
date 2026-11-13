# GitHub repository governance

This directory declares OpsQuest repository settings, Actions permissions,
security controls, and branch/tag rulesets with OpenTofu. It intentionally
contains no provider credential, backend credential, variable-value file, or
state.

Read the [repository governance runbook](GOVERNANCE.md)
before planning or applying changes. In particular:

- authenticate at runtime through `GITHUB_TOKEN`, GitHub App environment
  variables, or the GitHub CLI credential store;
- configure an encrypted remote backend with state locking before the first
  shared or automated apply;
- run `make tofu-check` without credentials for formatting and schema
  validation;
- inspect a saved plan before applying it manually;
- never run an automatic apply from an untrusted pull-request workflow.

The dependency lock file is committed. OpenTofu working data, plans, state,
and variable-value files are ignored locally as a second line of defense.
