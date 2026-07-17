#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
SKILLS_ROOT="${REPO_ROOT}/.agents/skills"

failures=0
skill_count=0

fail() {
  printf 'agent-docs: %s\n' "$1" >&2
  failures=$((failures + 1))
}

require_nonempty_file() {
  if [[ ! -s "$1" ]]; then
    fail "required file is missing or empty: ${1#"${REPO_ROOT}/"}"
  fi
}

require_nonempty_file "${REPO_ROOT}/AGENTS.md"

if [[ ! -d "${SKILLS_ROOT}" ]]; then
  fail "required directory is missing: .agents/skills"
else
  for required_skill in add-mission extend-sandbox-command prepare-iteration; do
    if [[ ! -d "${SKILLS_ROOT}/${required_skill}" ]]; then
      fail "required skill directory is missing: .agents/skills/${required_skill}"
    fi
  done

  for skill_dir in "${SKILLS_ROOT}"/*; do
    [[ -d "${skill_dir}" ]] || continue
    skill_count=$((skill_count + 1))
    directory_name="${skill_dir##*/}"
    skill_file="${skill_dir}/SKILL.md"

    if [[ ! -s "${skill_file}" ]]; then
      fail "${directory_name}/SKILL.md is missing or empty"
      continue
    fi
    if [[ "$(sed -n '1p' "${skill_file}")" != "---" ]]; then
      fail "${directory_name}/SKILL.md must begin with YAML frontmatter"
      continue
    fi

    closing_line="$(awk 'NR > 1 && $0 == "---" { print NR; exit }' "${skill_file}")"
    if [[ -z "${closing_line}" ]]; then
      fail "${directory_name}/SKILL.md has no closing frontmatter delimiter"
      continue
    fi

    frontmatter="$(sed -n "2,$((closing_line - 1))p" "${skill_file}")"
    declared_name="$(printf '%s\n' "${frontmatter}" | sed -n 's/^name:[[:space:]]*//p')"
    description="$(printf '%s\n' "${frontmatter}" | sed -n 's/^description:[[:space:]]*//p')"
    if [[ -z "${declared_name}" ]]; then
      fail "${directory_name}/SKILL.md frontmatter needs a non-empty name"
    elif [[ "${declared_name}" != "${directory_name}" ]]; then
      fail "skill name ${declared_name} does not match directory ${directory_name}"
    fi
    if [[ -z "${description}" ]]; then
      fail "${directory_name}/SKILL.md frontmatter needs a non-empty description"
    fi
  done
fi

if ((skill_count == 0)); then
  fail ".agents/skills contains no skill directories"
fi

require_nonempty_file "${SKILLS_ROOT}/add-mission/references/mission-checklist.md"
require_nonempty_file "${SKILLS_ROOT}/extend-sandbox-command/references/shell-semantics.md"
require_nonempty_file "${SKILLS_ROOT}/prepare-iteration/references/iteration-template.md"

marker_pattern='TO''DO|T''BD|FIX''ME|PLACE''HOLDER|CHANGE''ME|REPLACE''_ME'
scan_paths=("${REPO_ROOT}/AGENTS.md" "${REPO_ROOT}/.agents/skills" "${REPO_ROOT}/scripts")
if [[ -e "${REPO_ROOT}/.github/workflows/ci.yml" ]]; then
  scan_paths+=("${REPO_ROOT}/.github/workflows/ci.yml")
fi
if grep -RInE -- "${marker_pattern}" "${scan_paths[@]}" >/dev/null; then
  grep -RInE -- "${marker_pattern}" "${scan_paths[@]}" >&2 || true
  fail "unfinished marker found in the agent harness"
fi

if ((failures > 0)); then
  printf 'agent-docs: failed with %d problem(s)\n' "${failures}" >&2
  exit 1
fi

printf 'agent-docs: validated AGENTS.md and %d skill(s)\n' "${skill_count}"
