resource "github_repository_vulnerability_alerts" "opsquest" {
  repository = github_repository.opsquest.name
  enabled    = true
}

resource "github_repository_dependabot_security_updates" "opsquest" {
  repository = github_repository.opsquest.name
  enabled    = true

  depends_on = [github_repository_vulnerability_alerts.opsquest]
}
