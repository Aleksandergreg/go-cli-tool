# OpsQuest technical documentation

This directory contains the living explanation of how OpsQuest works today. The root [README](../README.md) remains the player-facing introduction and command reference; the numbered iteration reports preserve delivery history.

## Start here

| If you want to understand… | Read… | Then inspect… |
| --- | --- | --- |
| The whole system and its package boundaries | [Architecture](architecture.md) | [`cmd/opsquest/main.go`](../cmd/opsquest/main.go) and [`internal/game/environment.go`](../internal/game/environment.go) |
| What each mission teaches and why it appears there | [Curriculum](curriculum.md) | [`internal/mission/data`](../internal/mission/data) and [`internal/mission/world.go`](../internal/mission/world.go) |
| How player commands remain isolated | [Sandbox and safety](sandbox-and-safety.md) | [`internal/sandbox`](../internal/sandbox) and [`internal/dockerlab`](../internal/dockerlab) |

The three guides are intentionally layered:

1. **Architecture** establishes the runtime and ownership model.
2. **Curriculum** explains the product and learning model built on that runtime.
3. **Sandbox and safety** goes deeper into parsing, virtual state, Docker isolation, and resource limits.

## Diagram sources

Editable sources and rendered exports live in [`diagrams/`](diagrams/):

- Mermaid (`.mmd`) is used for exact component, sequence, and execution flows that should change alongside code.
- Excalidraw (`.excalidraw`) is used for conceptual maps where spatial grouping and annotations communicate the idea faster.
- SVG exports make every diagram readable directly from the repository and embeddable in these guides.

When a diagram changes, update its editable source and SVG export together. The Markdown guides link to both so a reviewer can compare them.

## Historical documentation

Files named `iteration_N.md` record what was delivered and observed at a point in time. They are evidence and history, not the source of truth for current architecture. Plans under [`plans/`](plans/) and improvement notes under [`improvements/`](improvements/) may describe unfinished work; each file should state that status explicitly.
