#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
PUBLIC_ROOT="${REPO_ROOT}/docs"

failures=0

fail() {
  printf 'docs: %s\n' "$1" >&2
  failures=$((failures + 1))
}

require_text() {
  local file="$1"
  local expected="$2"
  if ! grep -Fq -- "${expected}" "${file}"; then
    fail "${file#"${REPO_ROOT}/"} is missing expected text: ${expected}"
  fi
}

require_regex() {
  local file="$1"
  local expected="$2"
  if ! grep -Eq -- "${expected}" "${file}"; then
    fail "${file#"${REPO_ROOT}/"} does not match expected pattern: ${expected}"
  fi
}

for page in $(find "${PUBLIC_ROOT}" -type f -name '*.md' -print | sort); do
  if [[ "$(sed -n '1p' "${page}")" != "---" ]]; then
    fail "${page#"${REPO_ROOT}/"} must begin with YAML frontmatter"
    continue
  fi

  closing_line="$(awk 'NR > 1 && $0 == "---" { print NR; exit }' "${page}")"
  if [[ -z "${closing_line}" ]]; then
    fail "${page#"${REPO_ROOT}/"} has no closing frontmatter delimiter"
    continue
  fi

  frontmatter="$(sed -n "2,$((closing_line - 1))p" "${page}")"
  for field in description audience status; do
    if ! printf '%s\n' "${frontmatter}" | grep -Eq "^${field}:[[:space:]]*[^[:space:]]"; then
      fail "${page#"${REPO_ROOT}/"} needs a non-empty ${field} field"
    fi
  done
done

temp_dir="$(mktemp -d "${TMPDIR:-/tmp}/opsquest-docs.XXXXXX")"
cleanup() {
  rm -rf -- "${temp_dir}"
}
trap cleanup EXIT

find "${PUBLIC_ROOT}" -type f -name '*.md' -print |
  sed "s#^${PUBLIC_ROOT}/##" |
  sort -u >"${temp_dir}/public-pages"

awk '
  {
    line = $0
    sub(/^[[:space:]]*-[[:space:]]*/, "", line)
    separator = index(line, ": ")
    if (separator > 0) {
      line = substr(line, separator + 2)
    }
    if (line ~ /^https?:\/\//) {
      next
    }
    if (line ~ /^[A-Za-z0-9_.\/-]+\.md$/) {
      print line
    }
  }
' "${REPO_ROOT}/mkdocs.yml" | sort -u >"${temp_dir}/nav-pages"

while IFS= read -r page; do
  [[ -n "${page}" ]] && fail "public page is missing from mkdocs.yml navigation: ${page}"
done < <(comm -23 "${temp_dir}/public-pages" "${temp_dir}/nav-pages")

while IFS= read -r page; do
  [[ -n "${page}" ]] && fail "mkdocs.yml references a missing public page: ${page}"
done < <(comm -13 "${temp_dir}/public-pages" "${temp_dir}/nav-pages")

for forbidden in \
  "${PUBLIC_ROOT}/history" \
  "${PUBLIC_ROOT}/technical/repository-governance.md" \
  "${PUBLIC_ROOT}/roadmap/hosted-documentation.md"; do
  if [[ -e "${forbidden}" ]]; then
    fail "${forbidden#"${REPO_ROOT}/"} must remain outside the public docs tree"
  fi
done

for required in \
  "${REPO_ROOT}/project/history/README.md" \
  "${REPO_ROOT}/project/decisions/zensical-site.md" \
  "${REPO_ROOT}/infra/github/GOVERNANCE.md"; do
  if [[ ! -s "${required}" ]]; then
    fail "required repository-only document is missing: ${required#"${REPO_ROOT}/"}"
  fi
done

if [[ -e "${REPO_ROOT}/initial_prompt.md" ]]; then
  fail "initial_prompt.md duplicates canonical documentation and must stay retired"
fi
if grep -Fq 'initial_prompt.md' "${REPO_ROOT}/release-please-config.json"; then
  fail "release-please-config.json still references retired initial_prompt.md"
fi

go_version="$(awk '$1 == "go" { print $2; exit }' "${REPO_ROOT}/go.mod")"
go_series="$(printf '%s\n' "${go_version}" | awk -F. '{ print $1 "." $2 }')"
toolchain="$(awk '$1 == "toolchain" { print $2; exit }' "${REPO_ROOT}/go.mod")"

if [[ -z "${go_version}" ]]; then
  fail "go.mod has no Go version"
fi
if [[ "${toolchain}" != go"${go_series}".* ]]; then
  fail "go.mod toolchain ${toolchain:-<missing>} does not match Go ${go_series}"
fi

for file in \
  "${REPO_ROOT}/AGENTS.md" \
  "${REPO_ROOT}/README.md" \
  "${PUBLIC_ROOT}/play/quick-start.md"; do
  require_text "${file}" "Go ${go_series}"
done

mission_total=0
docker_count=0
docker_numbers=()
while IFS= read -r mission_file; do
  mission_total=$((mission_total + 1))
  if grep -Fq '"track": "docker"' "${mission_file}"; then
    docker_count=$((docker_count + 1))
    mission_number="$(sed -nE 's/^[[:space:]]*"number":[[:space:]]*([0-9]+),?/\1/p' "${mission_file}")"
    docker_numbers+=("${mission_number}")
  fi
done < <(find "${REPO_ROOT}/internal/mission/data" -type f -name '*.json' -print | sort)
linux_count=$((mission_total - docker_count))

require_text "${REPO_ROOT}/README.md" "${linux_count} Linux missions"
require_text "${REPO_ROOT}/README.md" "${docker_count} optional"
require_regex "${REPO_ROOT}/AGENTS.md" "${linux_count} .*Linux missions"
require_text "${REPO_ROOT}/AGENTS.md" "${docker_count} optional"
require_text "${PUBLIC_ROOT}/README.md" "${linux_count} Linux missions"
require_text "${PUBLIC_ROOT}/README.md" "${docker_count} optional"
require_text "${PUBLIC_ROOT}/play/mission-map.md" "**Linux:** ${linux_count} missions"
require_text "${PUBLIC_ROOT}/play/mission-map.md" "**Docker:** ${docker_count} optional"

diagram_source="${PUBLIC_ROOT}/play/diagrams/learning-journey.excalidraw"
diagram_svg="${PUBLIC_ROOT}/play/diagrams/learning-journey.svg"
require_text "${diagram_source}" "LINUX TRACK · ${linux_count} MISSIONS"
require_text "${diagram_svg}" "LINUX TRACK · ${linux_count} MISSIONS"
require_text "${diagram_source}" "${docker_count} bounded lifecycle and diagnostic missions"
require_text "${diagram_svg}" "${docker_count} bounded missions"

if ((${#docker_numbers[@]} > 0)); then
  docker_min="$(printf '%s\n' "${docker_numbers[@]}" | sort -n | head -n 1)"
  docker_max="$(printf '%s\n' "${docker_numbers[@]}" | sort -n | tail -n 1)"
  docker_range="Missions ${docker_min}–${docker_max}"
  require_text "${diagram_source}" "${docker_range}"
  require_text "${diagram_svg}" "${docker_range}"
fi

deprecated_pattern='docs/history|docs/technical/repository-governance|docs/roadmap/hosted-documentation|play/how-missions-work|play/linux-worlds|game/curriculum|game/progression|game/mission-design|technical/mission-content|roadmap/ci-cd|roadmap/docker-foundations'
scan_paths=(
  "${REPO_ROOT}/AGENTS.md"
  "${REPO_ROOT}/README.md"
  "${REPO_ROOT}/mkdocs.yml"
  "${PUBLIC_ROOT}"
  "${REPO_ROOT}/.agents"
  "${REPO_ROOT}/infra/github/README.md"
  "${REPO_ROOT}/project/decisions"
)
if grep -RInE -- "${deprecated_pattern}" "${scan_paths[@]}" >/dev/null; then
  grep -RInE -- "${deprecated_pattern}" "${scan_paths[@]}" >&2 || true
  fail "current documentation references a retired path"
fi

if ((failures > 0)); then
  printf 'docs: failed with %d problem(s)\n' "${failures}" >&2
  exit 1
fi

printf 'docs: validated %d public pages, %d Linux missions, %d Docker missions, and Go %s\n' \
  "$(wc -l <"${temp_dir}/public-pages" | tr -d ' ')" \
  "${linux_count}" \
  "${docker_count}" \
  "${go_version}"
