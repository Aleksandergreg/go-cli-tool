resource "github_workflow_repository_permissions" "opsquest" {
  repository                       = github_repository.opsquest.name
  default_workflow_permissions     = "read"
  can_approve_pull_request_reviews = true
}

resource "github_actions_repository_permissions" "opsquest" {
  repository           = github_repository.opsquest.name
  enabled              = true
  allowed_actions      = "selected"
  sha_pinning_required = true

  allowed_actions_config {
    github_owned_allowed = true
    verified_allowed     = false
    patterns_allowed = [
      "googleapis/release-please-action@*",
      "goreleaser/goreleaser-action@*",
      "opentofu/setup-opentofu@*",
    ]
  }
}
