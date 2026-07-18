The next natural step is **v0.3: Docker Foundations**, delivered as a small vertical slice rather than the entire Docker roadmap at once. This matches the explicit direction in [initial_prompt.md](/Users/aleksandergregersen/privat/go-cli-tool/initial_prompt.md:230).

### First implementation milestone

Build the environment boundary and one Docker mission end-to-end:

1. Introduce small execution and observation interfaces plus an environment factory.
2. Wrap the existing in-memory sandbox without changing Linux gameplay.
3. Add an environment type to missions, defaulting existing content to `simulated`.
4. Detect Docker availability through `opsquest doctor`.
5. Implement one disposable Docker mission with reliable teardown.
6. Validate the container’s observable state, independent of the exact commands used.

This follows the architectural recommendation already recorded in [iteration_6.md](/Users/aleksandergregersen/privat/go-cli-tool/docs/iteration_6.md:47).

### Proposed first campaign: “It Works on My Machine”

After the vertical slice proves reliable, expand it to roughly five missions:

- **Container Census** — distinguish images from running and stopped containers.
- **The Silent Crash** — diagnose a failed container using `ps` and `logs`.
- **Wrong Environment** — recreate a service with the correct environment variable.
- **Port Problems** — publish container port 8080 on host port 3000.
- **Emergency Shell** — inspect a running container with `exec` and repair the incident.

Supported teaching commands could initially be limited to `docker images`, `ps`, `run`, `logs`, `exec`, `stop`, and `rm`.

### Important boundaries

- Docker remains optional; all Linux missions work without it.
- Player input is parsed and allowlisted by OpsQuest, never passed to a host shell.
- Every resource receives unique OpsQuest labels and resource limits.
- Success, restart, quit, interruption, and failure clean up only OpsQuest-owned resources.
- No privileged containers, host bind mounts, or Docker socket exposure inside labs.
- Dockerfiles, volumes, networking, Compose, and Kubernetes remain follow-up iterations.

The first PR should therefore be: **environment abstraction + Docker capability detection + one real mission + lifecycle tests**. That gives genuine new gameplay while testing the architecture and safety model before committing to a complete Docker campaign.

A scripting-focused Linux mini-campaign would also be a sensible later content addition, since `vi` and `sh` are implemented but currently have no dedicated missions. No files were changed during this assessment.
