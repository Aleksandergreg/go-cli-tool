---
description: The learning principles behind OpsQuest incidents, feedback, hints, and safe experimentation.
audience: players, educators, and mission authors
status: current
---

# Learning philosophy

OpsQuest treats the terminal as a problem-solving environment. A mission should teach the player to observe state, choose a tool, evaluate the result, and adjust—not simply type a command shown moments earlier.

## The learning loop

Every mission follows the same six-part loop:

1. **Incident:** give the task a memorable operational context.
2. **Objective:** describe the result rather than a command recipe.
3. **Explore:** let the player inspect the environment and consult free guidance.
4. **Act:** execute one or more commands inside the isolated attempt.
5. **Observe:** evaluate output and environment state after successful commands.
6. **Reflect:** report progress, explain the incident, and record practiced tools and rewards.

## Productive freedom

Suggested commands establish the intended vocabulary, but they are not a whitelist for one mission. If two supported approaches create the same observable result, both should pass. This creates room for experimentation while keeping the environment small enough to teach clearly.

`status` reports which outcome checks remain without revealing a canonical command sequence. A free command guide and `help COMMAND` support syntax recall. Progressive hints are deliberately separate: they become more specific and trade some XP for help.

## Safe failure

Linux missions run entirely in memory, so experimentation cannot modify host files or processes. Restarting rebuilds the original mission setup. Optional Docker missions use exact, attempt-owned resources with a narrow teaching grammar.

This safety model supports learning: players can try a command, inspect its effect, and recover without needing a disposable virtual machine of their own.

## Curriculum shape

A world should introduce concepts in increasing combinations:

- establish observation before mutation;
- introduce one important state domain at a time;
- reinforce an earlier tool in a more complex incident;
- culminate in a boss that combines previously practiced concepts;
- avoid making difficulty depend only on obscure flags.

The [current curriculum](curriculum.md) makes the sequence visible. [Mission design](mission-design.md) turns these principles into authoring checks.
