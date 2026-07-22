---
description: OpsQuest mission JSON, embedded catalog loading, validation vocabulary, worlds, and compatibility rules.
audience: mission authors and contributors
status: current
---

# Mission content model

Mission content is declarative JSON embedded into the OpsQuest binary from `internal/mission/data`. Go code owns parsing, validation, execution, and outcome observation; mission files describe one exercise.

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
- unpinned Docker images, invalid aliases, or oversized Docker setup;
- a campaign that reappears as separate worlds in one track.

Catalog access is indexed by ID and number. Returned missions and worlds are deep copies so adapters cannot mutate embedded content.

## Observable validation

Conditions cover output, working directory, path existence, file content and logical lines, modes, owners, process state, environment values, and bounded Docker container state. The game layer compares output conditions; the active environment observes state conditions through the shared `Environment` contract.

Validators describe evidence, not the command that must produce it. See [Outcome-based mission design](../game/mission-design.md) for the product rule.

## Worlds and placement

Within each track, every contiguous campaign becomes an ordered world. Stage positions are derived at catalog load rather than persisted in JSON. Linux and Docker therefore have independent world numbering while mission numbers remain global.

Compatibility-sensitive mission data includes:

- stable mission IDs used by profile completions and hints;
- global mission numbers used by public top-level navigation;
- track and campaign order used to derive worlds;
- condition names and allowed fields;
- environment/setup pairing.

Changing one requires explicit compatibility reasoning and canonical success plus incomplete-solution coverage.

The implementation lives in [`internal/mission`](https://github.com/Aleksandergreg/go-cli-tool/tree/main/internal/mission). Use `$add-mission` for repository-specific authoring steps.
