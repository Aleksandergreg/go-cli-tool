---
description: Overview of the four OpsQuest Linux worlds and their learning progression.
audience: players
status: current
---

# Linux worlds

The default track contains 23 missions across four worlds. Worlds guide the learning order but do not lock content; every mission remains directly playable and replayable.

| World | Missions | Focus |
| --- | ---: | --- |
| **First Day** | 1–6 | Orientation, navigation, file reading, creation, recursive search, and movement |
| **The Logpocalypse** | 7–12 | Permissions, environment variables, log previews, processes, archives, and a pipeline boss |
| **Production Friday** | 13–19 | Logs, error counting, aggregation, text transformation, ownership, disk usage, and a multi-fault incident |
| **The Automation Shift** | 26–29 | Running supplied scripts, modal editing, reusable automation, and child-shell scope |

Missions 20–25 belong to the independent optional Docker track, so Linux resumes at global Mission 26. World and stage numbers are track-local. Saved completions use stable mission IDs, so the expanded display numbering does not invalidate existing profiles.

## Following or exploring the path

```console
$ opsquest play             # resume Linux progress
$ opsquest map              # see every world and stage
$ opsquest play --world 3   # stay within Production Friday
$ opsquest play 19          # begin at a global mission number
$ opsquest play linux-find-logs
```

Completing a world does not hide earlier practice. Use `--once` when you want one mission rather than continuous play.

The [Curriculum and mission map](../game/curriculum.md) lists the learning focus, suggested tools, difficulty, and observable outcomes for every current mission.
