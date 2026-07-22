---
description: Principles for authoring declarative OpsQuest missions with observable, solution-independent outcomes.
audience: mission authors and contributors
status: current
---

# Outcome-based mission design

A mission defines the initial world and the evidence of success. Engine behavior remains in Go; stories, setup, hints, rewards, and validation stay declarative in embedded JSON.

## Describe evidence, not a transcript

Good validation answers “what must now be true?” Examples include:

- a file exists at one path and no longer exists at another;
- file content, mode, or owner matches the intended result;
- a target process is stopped while a healthy process remains running;
- output contains all relevant paths and excludes a distractor;
- a report contains exactly the expected logical lines;
- an attempt-owned Docker container is running.

Avoid validators that require a particular command name, argument order, or intermediate state when another supported solution demonstrates the same understanding.

## Authoring questions

Before adding or moving a mission, answer:

1. **Prerequisite:** Which prior observations or command behaviors does it assume?
2. **New concept:** What is introduced rather than merely repeated?
3. **Evidence:** Which observable outcomes prove the objective?
4. **Alternatives:** Which equivalent supported solutions should remain valid?
5. **Reinforcement:** Where will the concept return with greater complexity?
6. **Recovery:** Can a player understand incomplete progress and restart safely?

## Guidance and difficulty

- Suggested commands name the intended tool family but do not constrain validation.
- One to five hints should progress from concept, to inspection strategy, to more concrete syntax.
- The explanation should connect the successful outcome to the operational lesson.
- Difficulty should reflect reasoning and composition, not missing documentation.
- Rewards should be consistent with neighboring missions and retain a meaningful minimum after hints.

## Required evidence in the repository

Every mission change needs canonical success coverage plus an incomplete or incorrect solution. Catalog loading must continue rejecting malformed fields, unsafe paths, conflicting setup, unsupported validators, and incompatible environment data.

Use the repository's `$add-mission` workflow and see [Mission content model](../technical/mission-content.md) for schema and compatibility details.
