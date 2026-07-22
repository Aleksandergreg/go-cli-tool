---
description: Understand OpsQuest incidents, observable objectives, hints, status checks, and mission routes.
audience: players
status: current
---

# How missions work

Every mission gives you an isolated environment and two pieces of direction:

- **Incident:** the fictional operational situation.
- **Objective:** the result the environment must show when you are done.

OpsQuest validates outcomes, not a prescribed command history. A mission that asks you to move a file can accept `mv` or an equivalent supported `cp` followed by `rm` if the final file content and paths are correct.

## Explore, act, observe

After each successful command, the game checks output and environment state. If only part of the objective is complete, it reports how many checks pass without revealing a mandatory recipe.

Inside a mission:

| Control | Purpose |
| --- | --- |
| `objective` | Repeat the objective and suggested tool names |
| `status` | Show satisfied and missing observable outcomes |
| `hint` | Reveal the next progressive hint and reduce available XP |
| `restart` | Rebuild the mission environment from its original setup |
| `help` | List teaching-shell commands |
| `help COMMAND` | Show focused syntax and examples |
| `?` | Show the full mission and terminal-control guide |
| `quit` | Leave the current attempt |

The free command guide and `help COMMAND` do not reduce XP. Progressive hints do; hint progress remains recorded for an incomplete mission so quitting does not erase the cost.

## Choosing a route

- Bare `opsquest play` resumes the first incomplete Linux mission.
- `opsquest play --world N` begins the first incomplete stage in one Linux world and stops at its boundary.
- A global number or stable mission ID begins there and continues in track order.
- `--once` returns after one completed mission.
- Completed missions remain replayable and never award duplicate XP.

Inside a mission, `play N` means Stage N of the current world. Use a stable ID from `map --ids` when jumping across worlds. `next`, `previous`, `map`, `list`, and `world N` let you navigate without returning to the top-level shell.

Listing progress preserves the current attempt. Switching missions or worlds starts the selected mission with fresh disposable state while retaining profile progress.

## Completion

When every condition passes, OpsQuest closes the active environment before recording completion. The first completion awards hint-adjusted XP, records practiced commands, and may unlock achievements. Replays preserve the original completion and reward.

Read [Progression and rewards](../game/progression.md) for ranks, XP, achievements, and persistence.
