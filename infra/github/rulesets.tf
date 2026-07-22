resource "github_repository_ruleset" "main" {
  name        = "Protect the default branch"
  repository  = github_repository.opsquest.name
  target      = "branch"
  enforcement = "active"

  # Repository admins retain a reviewed, auditable recovery path without
  # regaining direct-push access to the protected branch.
  bypass_actors {
    actor_id    = 5
    actor_type  = "RepositoryRole"
    bypass_mode = "pull_request"
  }

  conditions {
    ref_name {
      include = ["~DEFAULT_BRANCH"]
      exclude = []
    }
  }

  rules {
    deletion                = true
    non_fast_forward        = true
    required_linear_history = true

    pull_request {
      allowed_merge_methods             = ["squash"]
      required_approving_review_count   = 0
      required_review_thread_resolution = true
    }

    required_status_checks {
      strict_required_status_checks_policy = true

      required_check {
        context        = "Local quality gate"
        integration_id = 15368
      }

      required_check {
        context        = "Secret leak scan"
        integration_id = 15368
      }
    }
  }
}

resource "github_repository_ruleset" "release_tags" {
  name        = "Protect release tags"
  repository  = github_repository.opsquest.name
  target      = "tag"
  enforcement = "active"

  conditions {
    ref_name {
      include = ["refs/tags/v*"]
      exclude = []
    }
  }

  # Release Please may create new version tags. Existing version tags cannot
  # be moved or deleted without first changing this reviewed policy.
  rules {
    deletion = true
    update   = true
  }
}
