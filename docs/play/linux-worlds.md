---
description: Overview of the four OpsQuest Linux worlds and their learning progression.
audience: players
status: current
---

# Linux worlds

The default track contains 19 missions across four worlds. Worlds guide the learning order but do not lock content; every mission remains directly playable and replayable.

| World | Missions | Focus |
| --- | ---: | --- |
| **First Day** | 1–5 | Orientation, directories, files, recursive search, and movement |
| **The Logpocalypse** | 6–10 | Permissions, environment variables, processes, archives, and a pipeline boss |
| **Production Friday** | 11–16 | Logs, aggregation, text transformation, ownership, disk usage, and a multi-fault incident |
| **The Automation Shift** | 18–20 | Modal editing, reusable scripts, executable modes, and child-shell scope |

Mission 17 belongs to the independent optional Docker track, so Linux resumes at global Mission 18. World and stage numbers are track-local even though mission numbers remain globally stable.

## Following or exploring the path

```console
$ opsquest play             # resume Linux progress
$ opsquest map              # see every world and stage
$ opsquest play --world 3   # stay within Production Friday
$ opsquest play 16          # begin at a global mission number
$ opsquest play linux-find-logs
```

Completing a world does not hide earlier practice. Use `--once` when you want one mission rather than continuous play.

The [Curriculum and mission map](../game/curriculum.md) lists the learning focus, suggested tools, difficulty, and observable outcomes for every current mission.
