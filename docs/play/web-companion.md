---
description: Run the local OpsQuest browser companion while commands remain in the teaching terminal.
audience: players
status: current
---

# Web mission companion

The optional web companion separates guidance from command entry. OpsQuest
continues to execute and validate every command in the CLI-owned teaching
environment, while a local browser page shows the incident, objective,
suggested tools, progressive hints, live outcome checks, rewards, and the
completion explanation.

Start any Linux or Docker route with `--web`:

```console
$ opsquest play --web
$ opsquest play --web --once 5
$ opsquest play --web --track docker
```

OpsQuest prints a one-time URL such as
`http://127.0.0.1:49152/pair?token=...`. Open it in a browser before entering
commands. The URL exchanges its token for a browser-session cookie and cannot
be paired a second time.

## Split-screen play

Keep the terminal and browser visible together:

- Enter lab commands, mission navigation, and controls in the terminal.
- Read the incident, objective, suggested tools, and outcome checklist in the browser.
- Type `hint` in the terminal to reveal the next hint in the browser. The usual XP penalty and persisted hint progress still apply.
- Type `status` to force a fresh outcome observation. Successful lab commands also refresh progress automatically.
- Type `restart` to rebuild the disposable environment and reset its outcome checks.

In companion mode the terminal intentionally omits the mission narrative,
hint text, detailed progress checklist, and completion explanation. Raw command
output remains there because it is both player feedback and, for some
missions, validation input.

## Lifetime and reconnection

The companion exists only while that `opsquest play --web` process is active.
Reloading or reconnecting from the paired browser restores the latest complete
snapshot without replaying commands. Continuous play reuses the same page as
the route advances to another mission. Quitting or interrupting the CLI closes
the local server; the already-rendered page may remain visible but no longer
receives updates.

## Safety boundary

The server binds only to an ephemeral IPv4 loopback port. It uses one-time
pairing, an HTTP-only same-site cookie, exact Host and Origin checks, restrictive
browser security headers, and no permissive CORS policy.

The browser receives a presentation projection rather than the mission's raw
setup or validation structures. It has no API for submitting command text,
selecting Docker resources, mutating the profile, or declaring completion.
`game.Session` still evaluates observable outcomes, closes the active
environment, and saves the profile before publishing a completed state.

The companion adds no hosted service, account, external network requirement,
or third-party production dependency.
