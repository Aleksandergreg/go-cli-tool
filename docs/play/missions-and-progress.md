---
description: Understand OpsQuest objectives, controls, routes, hints, rewards, achievements, and saved progress.
audience: players
status: current
---

# Missions, hints, and progress

Every mission gives you a disposable environment and two pieces of direction:

- **Incident:** the fictional operational situation.
- **Objective:** the result the environment must show when you are done.

OpsQuest validates outcomes, not a prescribed command history. If two supported
approaches leave the environment in the same correct state, both can pass.

## Explore, act, observe

After each successful command, the game checks command output and environment
state. Partial progress is reported without revealing one mandatory solution.

Inside a mission:

| Control | Purpose |
| --- | --- |
| `objective` | Repeat the objective and suggested tool names |
| `status` | Show satisfied and missing observable outcomes |
| `hint` | Reveal the next progressive hint and reduce available XP |
| `restart` | Rebuild the disposable environment from its original setup |
| `help` | List teaching-shell commands |
| `help COMMAND` | Show focused syntax and examples |
| `?` | Show the mission and terminal-control guide |
| `quit` | Leave the current attempt |

The command guide, objective reminder, and `help COMMAND` are free. Progressive
hints trade some XP for increasingly specific guidance. Hint progress remains
recorded while a mission is incomplete, so quitting cannot erase the cost.

## Choose a route

- Bare `opsquest play` resumes the first incomplete Linux mission.
- `opsquest play --track docker` selects the first incomplete Docker mission.
- `opsquest play --world N` stays within one Linux world.
- A global mission number or stable ID begins there and follows that track.
- `--once` returns after one completed mission.
- `--web` moves mission guidance and live outcome progress into a local browser companion while commands remain in the terminal.
- Completed missions remain replayable and never award duplicate XP.

Inside a mission, `play N` means Stage N of the current world. A stable ID from
`map --ids` can jump across worlds. `next`, `previous`, `map`, `list`, and
`world N` navigate without returning to the top-level shell.

Listing progress preserves the active attempt. Switching missions or worlds
starts fresh disposable state while retaining saved profile progress.

## Completion and rewards

When every observable condition passes, OpsQuest closes the active environment
before recording completion. The first completion awards hint-adjusted XP,
records practiced commands, and may unlock achievements. Replays retain the
original completion and reward.

The displayed level is derived from total XP. Ranks progress through:

1. Intern
2. Operator
3. Junior Sysadmin
4. Sysadmin
5. SRE
6. Senior SRE

Achievements recognize the first fix, a three-command pipeline, ten practiced
commands, five hint-free completions, an advanced incident, and completion of
the Linux track.

## What persists

The local profile stores XP, ranks, completions, practiced commands, incomplete
hint progress, onboarding state, and achievements. Virtual files, directories,
processes, archives, environment variables, command history, and working
directory belong only to one attempt.

Completions use stable mission IDs rather than display positions. The mission
map can therefore grow without invalidating existing completions, provided
those IDs remain compatible.

See [Profiles and compatibility](../technical/profiles-and-compatibility.md) for
storage and migration behavior.
