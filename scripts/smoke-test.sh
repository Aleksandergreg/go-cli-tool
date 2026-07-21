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
FRESH_PROFILE_HOME="${SMOKE_ROOT}/fresh-profile"
ROUTE_PROFILE_HOME="${SMOKE_ROOT}/route-profile"
BINARY="${SMOKE_ROOT}/opsquest"
EMPTY_BIN="${SMOKE_ROOT}/empty-bin"
mkdir -p "${EMPTY_BIN}"

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

assert_not_contains() {
  local label="$1"
  local output="$2"
  local unexpected="$3"
  if [[ "${output}" == *"${unexpected}"* ]]; then
    printf 'smoke-test: %s unexpectedly contained %q\n--- output ---\n%s\n' "${label}" "${unexpected}" "${output}" >&2
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
  env PATH="${EMPTY_BIN}" OPSQUEST_HOME="${PROFILE_HOME}" OPSQUEST_PLAYER="smoke-operator" "${BINARY}" "$@"
}

run_fresh_opsquest() {
	env PATH="${EMPTY_BIN}" OPSQUEST_HOME="${FRESH_PROFILE_HOME}" OPSQUEST_PLAYER="fresh-operator" "${BINARY}" "$@"
}

run_route_opsquest() {
	env PATH="${EMPTY_BIN}" OPSQUEST_HOME="${ROUTE_PROFILE_HOME}" OPSQUEST_PLAYER="route-operator" "${BINARY}" "$@"
}

cd "${REPO_ROOT}"
go build -o "${BINARY}" ./cmd/opsquest

help_output="$(run_opsquest help)"
assert_no_ansi "help" "${help_output}"
assert_contains "help" "${help_output}" "OpsQuest — learn operations"
assert_contains "help" "${help_output}" "opsquest doctor"

fresh_play_output="$(printf 'quit\n' | run_fresh_opsquest play 1)"
assert_no_ansi "fresh play" "${fresh_play_output}"
assert_contains "fresh play" "${fresh_play_output}" "WELCOME TO OPSQUEST"
returning_play_output="$(printf 'quit\n' | run_fresh_opsquest play 1)"
assert_not_contains "returning play" "${returning_play_output}" "WELCOME TO OPSQUEST"

selected_play_output="$(printf 'pwd\nquit\n' | run_fresh_opsquest play 1)"
assert_contains "selected continuous play" "${selected_play_output}" "Continuing to Mission 02: Configuration Crawl"
assert_contains "selected continuous play" "${selected_play_output}" "MISSION 02: Configuration Crawl"

once_output="$(printf 'pwd\n' | run_fresh_opsquest play --once 1)"
assert_contains "one-shot play" "${once_output}" "NEXT RECOMMENDED"
assert_not_contains "one-shot play" "${once_output}" "MISSION 02: Configuration Crawl"

printf 'pwd\n' | run_route_opsquest play --once 1 >/dev/null
replay_hints_output="$(printf 'hint\nhint\nhint\nquit\n' | run_route_opsquest play 1)"
assert_contains "replay hints" "${replay_hints_output}" "Hint 1/2 (no XP cost):"
assert_contains "replay hints" "${replay_hints_output}" "Hint 2/2 (no XP cost):"
assert_contains "replay hints" "${replay_hints_output}" "No more hints."
selected_route_output="$(printf 'mkdir -p reports/daily\ntouch reports/daily/summary.txt\nquit\n' | run_route_opsquest play 3)"
assert_contains "selected route" "${selected_route_output}" "Continuing to Mission 04: The Missing Log File"
assert_contains "selected route" "${selected_route_output}" "MISSION 04: The Missing Log File"
assert_not_contains "selected route" "${selected_route_output}" "Continuing to Mission 02"

guide_output="$(run_opsquest guide)"
assert_no_ansi "guide" "${guide_output}"
assert_contains "guide" "${guide_output}" "WELCOME TO OPSQUEST"
assert_contains "guide" "${guide_output}" "final outcome is what counts"
assert_contains "guide" "${guide_output}" "never reach your host shell or files"

list_output="$(run_opsquest map)"
assert_no_ansi "map" "${list_output}"
assert_contains "map" "${list_output}" "LINUX CAMPAIGN"
assert_contains "map" "${list_output}" "WORLD 1/4 · First Day"
assert_contains "map" "${list_output}" "Stage 5/5 · #05"
assert_contains "map" "${list_output}" "0/19 missions complete"

docker_list_output="$(run_opsquest list --track docker)"
assert_no_ansi "docker list" "${docker_list_output}"
assert_contains "docker list" "${docker_list_output}" "DOCKER LABS"
assert_contains "docker list" "${docker_list_output}" "Container Census"
assert_contains "docker list" "${docker_list_output}" "0/1 missions complete"
assert_contains "docker list" "${docker_list_output}" "Continue: opsquest play --track docker"
assert_contains "docker list" "${docker_list_output}" "Jump: opsquest play --track docker --world N"

show_output="$(run_opsquest show 1)"
assert_no_ansi "show" "${show_output}"
assert_contains "show" "${show_output}" "MISSION 01: Where Am I?"
assert_contains "show" "${show_output}" "World 1/4: First Day · Stage 1/5"
assert_contains "show" "${show_output}" "Outcome checks: 1"
assert_contains "show" "${show_output}" "Commands you may need to solve this level:"
assert_contains "show" "${show_output}" "  pwd"

docker_show_output="$(run_opsquest show 17)"
assert_no_ansi "docker show" "${docker_show_output}"
assert_contains "docker show" "${docker_show_output}" "MISSION 17: Container Census"
assert_contains "docker show" "${docker_show_output}" "Hints: 0 used · 3 remaining · 3 total"
assert_contains "docker show" "${docker_show_output}" "  docker"

profile_output="$(run_opsquest profile --name "Smoke Operator")"
assert_no_ansi "profile initialization" "${profile_output}"
assert_contains "profile initialization" "${profile_output}" "Profile name updated."
assert_contains "profile initialization" "${profile_output}" "Operator: Smoke Operator"
[[ -s "${PROFILE_HOME}/profile.json" ]] || fail "profile was not created under the temporary OPSQUEST_HOME"

doctor_output="$(run_opsquest doctor)"
assert_no_ansi "doctor" "${doctor_output}"
assert_contains "doctor" "${doctor_output}" "embedded catalog: 20 missions"
assert_contains "doctor" "${doctor_output}" "profile path: ${PROFILE_HOME}/profile.json"
assert_contains "doctor" "${doctor_output}" "Linux labs: in-memory; no host shell or filesystem access"
assert_contains "doctor" "${doctor_output}" "docker labs:"
assert_contains "doctor" "${doctor_output}" "docker executable not found in PATH"

play_output="$(printf 'pwd\nopsquest list --completed\nplay 3\nquit\n' | run_opsquest play)"
assert_no_ansi "scripted mission" "${play_output}"
assert_contains "scripted mission" "${play_output}" "MISSION 01: Where Am I?"
assert_contains "scripted mission" "${play_output}" "Commands you may need to solve this level:"
assert_contains "scripted mission" "${play_output}" "✓ Mission complete!"
assert_contains "scripted mission" "${play_output}" "+40 XP"
assert_contains "continuous play" "${play_output}" "Continuing to Mission 02: Configuration Crawl"
assert_contains "in-mission list" "${play_output}" "1/19 missions complete"
assert_contains "in-mission switch" "${play_output}" "Switching to Mission 03: A Place for Everything"
assert_contains "in-mission switch" "${play_output}" "MISSION 03: A Place for Everything"

world_output="$(printf 'play 3\nquit\n' | run_opsquest play --world 2)"
assert_no_ansi "world jump" "${world_output}"
assert_contains "world jump" "${world_output}" "MISSION 06: Permission to Deploy"
assert_contains "world jump" "${world_output}" "World 2/4: The Logpocalypse · Stage 1/5"
assert_contains "world stage jump" "${world_output}" "Switching to Mission 08: The Runaway Worker"
assert_contains "world stage jump" "${world_output}" "World 2/4: The Logpocalypse · Stage 3/5"
assert_not_contains "world stage jump" "${world_output}" "MISSION 03: A Place for Everything"

final_profile="$(run_opsquest profile)"
assert_no_ansi "completed profile" "${final_profile}"
assert_contains "completed profile" "${final_profile}" "Missions completed: 1"
assert_contains "completed profile" "${final_profile}" "40 XP"

printf 'smoke-test: onboarding, replay hints, sticky selected routes, continuous and one-shot play, worlds, world-local stages, Linux and Docker discovery, profile, doctor, and in-mission navigation passed\n'
