---
description: Complete map of current OpsQuest tracks, worlds, missions, suggested tools, and observable outcomes.
audience: players, educators, and mission authors
status: current
---

# OpsQuest curriculum

OpsQuest teaches operational problem-solving through incidents rather than isolated command drills. Missions provide a disposable environment, describe an observable goal, and accept any supported command sequence that produces the required outcome.

## Learning journey

![OpsQuest learning journey](diagrams/learning-journey.svg)

Editable source: [`learning-journey.excalidraw`](diagrams/learning-journey.excalidraw)

The curriculum has two independent tracks:

- **Linux:** 19 missions across four ordered worlds. Bare `opsquest play` follows this track.
- **Docker:** one optional Foundations mission in its first world. Docker readiness never blocks Linux play.

Mission numbers are stable and global across both tracks, which is why the Docker mission is number 17 while Linux World 4 resumes at number 18. World and stage positions are derived separately inside each track.

The [learning philosophy](learning-philosophy.md) explains the incident loop and feedback model. [Outcome-based mission design](mission-design.md) describes how validators preserve equivalent solutions.

## Current mission map

The “tools” column records suggested commands, not a mandatory solution. Outcome names are the declarative validator types exercised by the mission.

| Global # | Track position | Difficulty | Primary learning focus | Suggested tools | Observable outcomes |
| ---: | --- | --- | --- | --- | --- |
| 1 | Linux W1.1 | Beginner | Identify the current directory | `pwd` | Output equals |
| 2 | Linux W1.2 | Beginner | Inspect and navigate a directory tree | `pwd`, `ls`, `cd` | Current directory equals |
| 3 | Linux W1.3 | Beginner | Create directories and files | `ls`, `mkdir`, `touch` | Directory and file exist |
| 4 | Linux W1.4 | Beginner | Search recursively and filter matching files | `find`, `grep` | Output contains all required paths and excludes noise |
| 5 | Linux W1.5 | Beginner | Move a release artifact without losing content | `ls`, `mv`, `cp`, `rm` | Destination content equals; source is missing |
| 6 | Linux W2.1 | Beginner | Inspect and repair executable permissions | `ls`, `stat`, `chmod` | File mode equals |
| 7 | Linux W2.2 | Beginner | Inspect and export environment variables | `env`, `echo`, `export` | Environment value equals |
| 8 | Linux W2.3 | Intermediate | Inspect processes and stop the faulty one | `ps`, `kill` | Target stopped; healthy process still running |
| 9 | Linux W2.4 | Intermediate | Inspect and extract an archive | `ls`, `tar` | Extracted content contains and equals expected data |
| 10 | Linux W2.5 | Advanced | Build a multi-stage log-processing pipeline | `cat`, `grep`, `awk`, `sort`, `uniq` | Report file content equals |
| 11 | Linux W3.1 | Beginner | Select the newest log lines | `cat`, `tail` | Output equals |
| 12 | Linux W3.2 | Intermediate | Sort and aggregate repeated alerts | `cat`, `sort`, `uniq` | Report lines equal |
| 13 | Linux W3.3 | Intermediate | Repair configuration text while preserving mode | `cat`, `sed`, `vi` | Content and mode equal |
| 14 | Linux W3.4 | Intermediate | Correct ownership and permissions together | `stat`, `chown`, `chmod` | Owner and mode equal |
| 15 | Linux W3.5 | Intermediate | Compare disk usage and identify the right target | `ls`, `du`, `sort`, `tail` | Output includes target and excludes distractor |
| 16 | Linux W3.6 | Advanced | Resolve a multi-fault production incident | `tar`, `sed`, `vi`, `chmod`, `ps`, `kill` | Content, mode, and process-state checks |
| 17 | Docker W1.1 | Beginner | List mission containers and start a stopped service | `docker` | Owned container count equals; target is running |
| 18 | Linux W4.1 | Beginner | Make a focused configuration edit with a modal editor | `cat`, `vi` | Content and mode equal |
| 19 | Linux W4.2 | Intermediate | Repair, permit, and run a reusable report script | `cat`, `less`, `vi`, `sed`, `chmod`, `sh` | Report content/lines and script mode |
| 20 | Linux W4.3 | Advanced | Reason about child-shell directory and environment scope | `cat`, `less`, `vi`, `sed`, `chmod`, `sh`, `pwd`, `env` | Parent scope preserved; report and script state correct |

## World progression

### Linux World 1: First Day

The opening world establishes orientation and safe filesystem manipulation. It moves from observing location, through navigation and creation, to recursive search and a small state-changing incident. All five missions are beginner difficulty.

### Linux World 2: The Logpocalypse

This world broadens the state model: permissions, environment, processes, and archives. Missions 6–7 remain beginner-friendly, Missions 8–9 combine inspection with mutation, and Mission 10 is the first advanced boss and sustained pipeline exercise.

### Linux World 3: Production Friday

The third world reinforces text inspection and transformation while adding ownership and disk-usage reasoning. Its six stages culminate in a multi-fault incident that combines archive extraction, configuration repair, permissions, and process control.

### Linux World 4: The Automation Shift

The final Linux world moves from one-off commands to reusable automation. It introduces the modal editor, executable virtual scripts, and the difference between child-shell scope and durable virtual state.

### Docker World 1: It Works on My Machine

Docker Foundations begins with the smallest useful lifecycle: discover the attempt's containers and start the intended stopped service. Logical aliases hide real container names and IDs, keeping the lesson focused on observable container state.

## Curriculum evolution

The current map makes deliberate expansion opportunities visible: there is no dedicated early file-reading stage, the first substantial pipeline appears in an advanced boss, and Docker currently has only its opening lifecycle mission. Those are curriculum decisions to address with new declarative missions rather than by weakening existing validators.

Read [Progression and rewards](progression.md) for route and persistence behavior, and [Outcome-based mission design](mission-design.md) for the authoring checklist.

Mission content and its canonical success/incomplete coverage live in [`internal/mission/data`](https://github.com/Aleksandergreg/go-cli-tool/tree/main/internal/mission/data) and [`internal/game/missions_test.go`](https://github.com/Aleksandergreg/go-cli-tool/blob/main/internal/game/missions_test.go). Use the repository's `$add-mission` skill when changing that content.
