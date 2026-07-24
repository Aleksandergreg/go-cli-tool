---
description: Author declarative OpsQuest missions with observable outcomes, strict catalog validation, and compatibility-safe identifiers.
audience: mission authors and contributors
status: current
---

# Mission authoring

Mission content is declarative JSON embedded into the OpsQuest binary from `internal/mission/data`. Go code owns parsing, validation, execution, and outcome observation; mission files describe one exercise.

## Author around evidence

A mission defines its initial world and the evidence of success. Validation
should answer “what must now be true?” rather than “which command did the
player type?”

Good evidence includes:

- a file exists at one path and no longer exists at another;
- file content, mode, or owner matches the intended result;
- a target process is stopped while a healthy process remains running;
- output contains all relevant paths and excludes a distractor;
- a report contains exactly the required logical lines;
- an attempt-owned Docker container is in the required state.

Avoid requiring one command name, argument order, or intermediate state when
another supported solution demonstrates the same understanding.

Before adding or moving a mission, answer:

1. Which prior observations or command behaviors does it assume?
2. What concept is introduced rather than merely repeated?
3. Which observable outcomes prove the objective?
4. Which equivalent supported solutions should remain valid?
5. Where will the concept return with greater complexity?
6. Can a player understand incomplete progress and restart safely?

## Mission shape

Each mission defines:

```text
identity: stable ID, global number, track, environment, campaign
narrative: title, difficulty, story, objective, explanation
guidance: suggested command names and one to five progressive hints
setup: virtual directories, files, processes, environment, and archives
       or bounded Docker image and container fixtures
validation: one or more observable outcome conditions
rewards: base XP and per-hint penalty
```

Linux remains the default track and `simulated` remains the default environment for compatibility with earlier mission files. Docker missions must use the Docker track and cannot mix simulated setup with Docker fixtures.

## Strict catalog loading

`mission.LoadCatalog` loads every embedded JSON file before the CLI starts. It rejects:

- unknown JSON fields or multiple JSON values;
- malformed or duplicate stable IDs and global numbers;
- non-contiguous global numbering;
- unsupported tracks, environments, or difficulties;
- missing narrative, guidance, setup, validation, or reward data;
- unsafe, conflicting, or inconsistent virtual paths and state;
- validators with missing, extra, or incompatible fields;
- unpinned Docker images, invalid aliases, oversized Docker setup, or incomplete diagnostic fixtures;
- a campaign that reappears as separate worlds in one track.

Catalog access is indexed by ID and number. Returned missions and worlds are deep copies so adapters cannot mutate embedded content.

## Observable validation

Conditions cover output, working directory, path existence, file content and logical lines, modes, owners, process state, environment values, and bounded Docker container state. Docker fixtures may additionally declare a bounded log and exit code together for a stopped one-shot diagnostic job; container conditions can require either running or stopped state. The game layer compares output conditions; the active environment observes state conditions through the shared `Environment` contract.

Suggested commands identify the intended tool family but do not constrain
validation. One to five hints should progress from concept, through inspection
strategy, toward concrete syntax. Difficulty should reflect reasoning and
composition rather than missing documentation.

## Worlds and placement

Within each track, every contiguous campaign becomes an ordered world. Stage positions are derived at catalog load rather than persisted in JSON. Linux and Docker therefore have independent world numbering while mission numbers remain global.

Compatibility-sensitive mission data includes:

- stable mission IDs used by profile completions and hints;
- global mission numbers used by public top-level navigation;
- track and campaign order used to derive worlds;
- condition names and allowed fields;
- environment/setup pairing.

Changing one requires explicit compatibility reasoning and canonical success plus incomplete-solution coverage.

## Required repository evidence

Every mission change needs canonical success coverage plus an incomplete or
incorrect solution. Catalog tests must continue rejecting malformed fields,
unsafe paths, conflicting setup, unsupported validators, and incompatible
environment data.

The implementation lives in
[`internal/mission`](https://github.com/Aleksandergreg/go-cli-tool/tree/main/internal/mission).
Use the repository's `$add-mission` workflow for the complete authoring and
validation sequence.
