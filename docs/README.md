# OpsQuest documentation

This is the repository-level map for OpsQuest documentation. The root [README](../README.md) remains the player-facing introduction and command reference; material here explains the game design, implementation, future direction, and delivery history in progressively more detail.

## Choose a section

| Section | Use it for | Authority |
| --- | --- | --- |
| [Game and learning](game/README.md) | The learning loop, worlds, missions, progression, and curriculum decisions | Current product documentation |
| [Technical](technical/README.md) | Architecture, runtime behavior, sandbox isolation, Docker boundaries, and implementation diagrams | Current technical documentation; code and tests win if they disagree |
| [Roadmap](roadmap/README.md) | Proposed work, unfinished campaign increments, and improvement options | Forward-looking; each page must state its status |
| [Delivery history](history/README.md) | What a completed iteration delivered and which checks were observed at that time | Historical evidence, not current behavior |

For a first technical pass, read [Architecture](technical/architecture.md), then [Sandbox and safety](technical/sandbox-and-safety.md). For the product and learning model, start with the [Curriculum](game/curriculum.md).

## Placement rules

Use the document's purpose—not its file type or creation date—to decide where it belongs:

1. Put player experience, teaching strategy, mission sequencing, and game-system explanations in `game/`.
2. Put descriptions of implemented code, runtime flows, contracts, and safety properties in `technical/`.
3. Put proposals and unfinished work in `roadmap/`, with an explicit status near the top.
4. Put point-in-time delivery reports in `history/iterations/`; use zero-padded filenames so they sort chronologically.
5. Keep editable diagram sources and their rendered SVGs beside the section that owns them, under that section's `diagrams/` directory.

Current guides should describe what OpsQuest does now. A delivered proposal should be folded into the appropriate current guide; its roadmap page may remain as rationale, but must be marked delivered. Historical reports should not be rewritten into the source of truth.

## Diagram convention

Mermaid (`.mmd`) is used for exact component, sequence, and execution flows that should change alongside code. Excalidraw (`.excalidraw`) is used for conceptual maps where spatial grouping helps. Each editable source has a repository-viewable SVG export with the same basename; update and review the pair together.

The [hosted documentation proposal](roadmap/hosted-documentation.md) applies this same information architecture to a possible public site.
