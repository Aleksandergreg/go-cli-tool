#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
SMOKE_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/opsquest-smoke.XXXXXX")"
PROFILE_HOME="${SMOKE_ROOT}/profile"
BINARY="${SMOKE_ROOT}/opsquest"

cleanup() {
  rm -rf -- "${SMOKE_ROOT}"
}
trap cleanup EXIT

fail() {
  printf 'smoke-test: %s\n' "$1" >&2
  exit 1
}

assert_contains() {
  local label="$1"
  local output="$2"
  local expected="$3"
  if [[ "${output}" != *"${expected}"* ]]; then
    printf 'smoke-test: %s did not contain %q\n--- output ---\n%s\n' "${label}" "${expected}" "${output}" >&2
    exit 1
  fi
}

run_opsquest() {
  env OPSQUEST_HOME="${PROFILE_HOME}" OPSQUEST_PLAYER="smoke-operator" "${BINARY}" "$@"
}

cd "${REPO_ROOT}"
go build -o "${BINARY}" ./cmd/opsquest

help_output="$(run_opsquest help)"
assert_contains "help" "${help_output}" "OpsQuest — learn operations"
assert_contains "help" "${help_output}" "opsquest doctor"

list_output="$(run_opsquest list)"
assert_contains "list" "${list_output}" "LINUX CAMPAIGN"
assert_contains "list" "${list_output}" "0/16 missions complete"

show_output="$(run_opsquest show 1)"
assert_contains "show" "${show_output}" "MISSION 01: Where Am I?"
assert_contains "show" "${show_output}" "Outcome checks: 1"

profile_output="$(run_opsquest profile --name "Smoke Operator")"
assert_contains "profile initialization" "${profile_output}" "Profile name updated."
assert_contains "profile initialization" "${profile_output}" "Operator: Smoke Operator"
[[ -s "${PROFILE_HOME}/profile.json" ]] || fail "profile was not created under the temporary OPSQUEST_HOME"

doctor_output="$(run_opsquest doctor)"
assert_contains "doctor" "${doctor_output}" "embedded catalog: 16 missions"
assert_contains "doctor" "${doctor_output}" "profile path: ${PROFILE_HOME}/profile.json"
assert_contains "doctor" "${doctor_output}" "sandbox: in-memory; host command execution disabled"

play_output="$(printf 'pwd\n' | run_opsquest play 1)"
assert_contains "scripted mission" "${play_output}" "MISSION 01: Where Am I?"
assert_contains "scripted mission" "${play_output}" "✓ Mission complete!"
assert_contains "scripted mission" "${play_output}" "+40 XP"

final_profile="$(run_opsquest profile)"
assert_contains "completed profile" "${final_profile}" "Missions completed: 1"
assert_contains "completed profile" "${final_profile}" "40 XP"

printf 'smoke-test: help, list, show, profile, doctor, and scripted mission passed\n'
