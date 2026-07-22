locals {
  github_owner    = "Aleksandergreg"
  repository_name = "go-cli-tool"
}

# Authentication is intentionally omitted. The provider reads a short-lived
# runtime credential from GITHUB_TOKEN, GitHub App environment variables, or
# the GitHub CLI credential store. No credential belongs in OpenTofu code.
provider "github" {
  owner = local.github_owner
}
