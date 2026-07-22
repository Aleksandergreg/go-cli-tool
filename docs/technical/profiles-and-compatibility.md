---
description: Durable OpsQuest profile state, atomic persistence, migration behavior, and compatibility-sensitive identifiers.
audience: contributors and maintainers
status: current
---

# Profiles and compatibility

Mission attempts are disposable; player progress is durable. `internal/profile` is the only normal application component that writes persistent state.

## Stored state

The versioned profile records:

- display name and onboarding state;
- total XP;
- completion reward, hint count, and timestamp by stable mission ID;
- successful command practice counts;
- progressive hints for incomplete missions;
- achievement unlock timestamps.

The profile does not contain a mission's virtual filesystem, working directory, environment, processes, archives, or command history.

## Location and overrides

By default, `profile.json` lives under the platform user configuration directory in an `opsquest` folder.

- `OPSQUEST_HOME` selects another profile directory.
- `OPSQUEST_PLAYER` supplies the initial display name for a new profile.

Display names accept up to 40 printable Unicode characters. New invalid values are rejected; legacy values are normalized when loaded so terminal controls cannot be rendered.

## Atomic storage

Saving validates the live profile, clones map-backed state, normalizes compatibility fields, and writes indented JSON to an owner-only temporary file in the destination directory. The file is synced, closed, and atomically renamed over `profile.json`.

Reset removes only that profile path. Player commands inside a mission cannot address it.

## Version behavior

The current profile schema is versioned independently from missions. Older supported profiles are normalized by adding missing maps and cleaning invalid legacy state. A profile written by a newer schema version is rejected instead of being partially interpreted.

Additive fields still require backwards-compatibility reasoning: an older executable may ignore a field and remove it if it later rewrites the profile. Destructive or semantic changes require an explicit migration and tests.

## Sensitive identifiers

Treat these as compatibility contracts:

- profile schema version and JSON field meaning;
- stable mission IDs used as map keys;
- completion and hint semantics;
- rank and reward rules when changing visible progression;
- achievement IDs stored in profiles.

The implementation and migration tests live in [`internal/profile`](https://github.com/Aleksandergreg/go-cli-tool/tree/main/internal/profile).
