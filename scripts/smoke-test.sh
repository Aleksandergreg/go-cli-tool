#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
TMP_PARENT="$(cd -- "${TMPDIR:-/tmp}" && pwd -P)"
if [[ "${TMP_PARENT}" == "/" ]]; then
  SMOKE_TEMPLATE="/opsquest-smoke.XXXXXX"
else
  SMOKE_TEMPLATE="${TMP_PARENT}/opsquest-smoke.XXXXXX"
fi
SMOKE_ROOT="$(mktemp -d "${SMOKE_TEMPLATE}")"
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

assert_no_ansi() {
  local label="$1"
  local output="$2"
  if [[ "${output}" == *$'\033'* ]]; then
    printf 'smoke-test: %s contained ANSI escape bytes in captured non-terminal output\n' "${label}" >&2
    printf 'escaped output: %q\n' "${output}" >&2
    exit 1
  fi
}

run_opsquest() {
  env OPSQUEST_HOME="${PROFILE_HOME}" OPSQUEST_PLAYER="smoke-operator" "${BINARY}" "$@"
}

cd "${REPO_ROOT}"
go build -o "${BINARY}" ./cmd/opsquest

help_output="$(run_opsquest help)"
assert_no_ansi "help" "${help_output}"
assert_contains "help" "${help_output}" "OpsQuest — learn operations"
assert_contains "help" "${help_output}" "opsquest doctor"

list_output="$(run_opsquest list)"
assert_no_ansi "list" "${list_output}"
assert_contains "list" "${list_output}" "LINUX CAMPAIGN"
assert_contains "list" "${list_output}" "0/16 missions complete"

show_output="$(run_opsquest show 1)"
assert_no_ansi "show" "${show_output}"
assert_contains "show" "${show_output}" "MISSION 01: Where Am I?"
assert_contains "show" "${show_output}" "Outcome checks: 1"

profile_output="$(run_opsquest profile --name "Smoke Operator")"
assert_no_ansi "profile initialization" "${profile_output}"
assert_contains "profile initialization" "${profile_output}" "Profile name updated."
assert_contains "profile initialization" "${profile_output}" "Operator: Smoke Operator"
[[ -s "${PROFILE_HOME}/profile.json" ]] || fail "profile was not created under the temporary OPSQUEST_HOME"

doctor_output="$(run_opsquest doctor)"
assert_no_ansi "doctor" "${doctor_output}"
assert_contains "doctor" "${doctor_output}" "embedded catalog: 16 missions"
assert_contains "doctor" "${doctor_output}" "profile path: ${PROFILE_HOME}/profile.json"
assert_contains "doctor" "${doctor_output}" "sandbox: in-memory; host command execution disabled"

play_output="$(printf 'pwd\nopsquest list --completed\nplay 3\nquit\n' | run_opsquest play)"
assert_no_ansi "scripted mission" "${play_output}"
assert_contains "scripted mission" "${play_output}" "MISSION 01: Where Am I?"
assert_contains "scripted mission" "${play_output}" "✓ Mission complete!"
assert_contains "scripted mission" "${play_output}" "+40 XP"
assert_contains "continuous play" "${play_output}" "Continuing to Mission 02: Configuration Crawl"
assert_contains "in-mission list" "${play_output}" "1/16 missions complete"
assert_contains "in-mission switch" "${play_output}" "Switching to Mission 03: A Place for Everything"
assert_contains "in-mission switch" "${play_output}" "MISSION 03: A Place for Everything"

final_profile="$(run_opsquest profile)"
assert_no_ansi "completed profile" "${final_profile}"
assert_contains "completed profile" "${final_profile}" "Missions completed: 1"
assert_contains "completed profile" "${final_profile}" "40 XP"

printf 'smoke-test: help, list, show, profile, doctor, and continuous in-mission navigation passed\n'
