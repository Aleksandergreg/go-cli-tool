resource "github_repository" "opsquest" {
  name         = local.repository_name
  description  = "Duolingo meets a terminal sandbox: a Go CLI game that teaches Linux and Docker operations through story-driven incidents in a safe, in-memory shell."
  homepage_url = "https://aleksandergreg.github.io/go-cli-tool/"
  topics       = ["go", "cli", "terminal-game", "linux", "docker", "devops", "sre", "education", "learn-linux", "sandbox"]
  visibility   = "public"

  has_discussions = true
  has_issues      = true
  has_projects    = true
  has_wiki        = true

  allow_auto_merge            = true
  allow_merge_commit          = false
  allow_rebase_merge          = false
  allow_squash_merge          = true
  allow_update_branch         = true
  delete_branch_on_merge      = true
  squash_merge_commit_message = "PR_BODY"
  squash_merge_commit_title   = "PR_TITLE"

  lifecycle {
    prevent_destroy = true
  }
}

# The repository predates this configuration. Keeping the import declarative
# prevents a first apply from attempting to create a replacement repository.
import {
  to = github_repository.opsquest
  id = "go-cli-tool"
}
