---
description: Maintainer-oriented map of OpsQuest architecture, safety, mission content, persistence, and contribution workflows.
audience: contributors and maintainers
status: current
---

# Technical documentation

OpsQuest separates product flow, declarative mission content, isolated execution, and durable player progress. These guides describe the implemented system; code and tests remain authoritative.

## Reading order

1. [Architecture](architecture.md) maps packages, startup, mission attempts, and environment contracts.
2. [Sandbox and safety](sandbox-and-safety.md) traces command execution, virtual state, quotas, Docker isolation, and threat controls.
3. [Mission content model](mission-content.md) explains embedded JSON, catalog integrity, validators, and world derivation.
4. [Profiles and compatibility](profiles-and-compatibility.md) covers durable progress, atomic storage, and sensitive identifiers.
5. [Contributing and quality gates](contributing.md) maps common changes to repository workflows and validation.

Editable Mermaid and Excalidraw sources plus rendered SVGs live in [`diagrams/`](diagrams/). Each guide links to the diagrams it owns so visual material retains its implementation context.
