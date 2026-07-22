---
description: OpsQuest routes, hints, XP, ranks, achievements, replay, and persistent player progress.
audience: players and game contributors
status: current
---

# Progression and rewards

Worlds recommend an order without locking the player into it. Stable mission IDs and global numbers support direct practice, while track-local world and stage positions keep the curriculum readable.

## Route behavior

- Bare `play` selects the first incomplete Linux mission.
- `play --track docker` selects the first incomplete Docker mission.
- `play --world N` stays within one track-local world.
- A top-level mission number or stable ID begins there and follows that track's order.
- Inside a mission, `play STAGE` stays in the current world; a stable ID is the explicit cross-world route.
- `--once` stops after one completion.
- Completed missions remain directly replayable.

The profile records completions by stable mission ID, not displayed world position. Curriculum presentation can therefore evolve without losing completions, provided IDs and public global numbers remain compatible.

## Hints and XP

Missions define a base XP reward and a per-hint penalty. Progressive hints persist while a mission is incomplete, preventing quit-and-retry from erasing their cost. Free command guidance, the objective reminder, and `help COMMAND` do not affect rewards.

Only the first completion awards XP. Replaying a completed mission retains the original reward and clears hints used during that replay instead of modifying history.

## Levels and ranks

The displayed level is derived from total XP. Rank thresholds currently progress through:

1. Intern
2. Operator
3. Junior Sysadmin
4. Sysadmin
5. SRE
6. Senior SRE

## Achievements

Achievements reward learning behavior:

- complete the first fix;
- build a successful three-command pipeline;
- practice ten different commands;
- complete five missions without hints;
- complete an advanced incident;
- finish every Linux mission.

## Persistence boundary

XP, completions, command counts, hints, onboarding state, and achievements persist in the local profile. Attempt state—virtual files, directories, processes, archives, environment variables, history, and working directory—does not.

See [Profiles and compatibility](../technical/profiles-and-compatibility.md) for the persisted model and atomic storage behavior.
