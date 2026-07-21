# Technical documentation

This section describes the implemented system and its safety boundaries. It is written for maintainers and contributors; the code and tests remain authoritative.

## Reading order

1. [Architecture](architecture.md) maps packages, startup, mission attempts, environment contracts, persistence, and ownership of changes.
2. [Sandbox and safety](sandbox-and-safety.md) traces command execution, virtual state, quotas, Docker isolation, threats, and review checks.

Editable Mermaid and Excalidraw sources plus rendered SVGs live in [`diagrams/`](diagrams/). A guide links to the diagrams it owns so diagrams remain context, not a separate body of undocumented design.

The repository [agent guide](../../AGENTS.md) contains the package boundaries, project invariants, and validation gates used when changing the implementation.
