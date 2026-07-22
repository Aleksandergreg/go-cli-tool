output "repository_url" {
  description = "Managed repository URL."
  value       = github_repository.opsquest.html_url
}

output "default_branch_ruleset_id" {
  description = "GitHub identifier for the default-branch ruleset."
  value       = github_repository_ruleset.main.ruleset_id
}

output "release_tag_ruleset_id" {
  description = "GitHub identifier for the release-tag ruleset."
  value       = github_repository_ruleset.release_tags.ruleset_id
}
