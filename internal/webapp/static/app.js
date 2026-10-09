"use strict";

const byID = (id) => document.getElementById(id);
let latestState = "";
const elements = {
  waiting: byID("waiting"),
  layout: byID("mission-layout"),
  connection: byID("connection"),
  connectionLabel: byID("connection-label"),
  location: byID("location"),
  difficulty: byID("difficulty"),
  missionNumber: byID("mission-number"),
  title: byID("title"),
  reward: byID("reward"),
  story: byID("story"),
  objective: byID("objective"),
  statePill: byID("state-pill"),
  progressLabel: byID("progress-label"),
  outcomes: byID("outcomes"),
  commands: byID("commands"),
  hintCount: byID("hint-count"),
  hints: byID("hints"),
  hintInstruction: byID("hint-instruction"),
  completion: byID("completion"),
  completionTitle: byID("completion-title"),
  explanation: byID("explanation"),
  completionMeta: byID("completion-meta"),
};

function textNode(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  node.textContent = text;
  return node;
}

function setConnection(state, label) {
  elements.connection.dataset.state = state;
  elements.connectionLabel.textContent = label;
}

function locationLabel(snapshot) {
  const track = snapshot.track === "docker" ? "docker" : "linux";
  if (!snapshot.world_number) return track;
  return `${track} | world ${snapshot.world_number}/${snapshot.world_total}: ${snapshot.world_name} | stage ${snapshot.stage_number}/${snapshot.stage_total}`;
}

function renderOutcomes(snapshot) {
  elements.outcomes.replaceChildren();
  for (const outcome of snapshot.outcomes || []) {
    const row = textNode("li", `outcome${outcome.satisfied ? " complete" : ""}`, "");
    row.append(textNode("span", "check-box", outcome.satisfied ? "✓" : ""));
    row.append(textNode("span", "", outcome.description));
    elements.outcomes.append(row);
  }
}

function renderCommands(snapshot) {
  elements.commands.replaceChildren();
  for (const command of snapshot.suggested_commands || []) {
    elements.commands.append(textNode("code", "command-chip", command));
  }
}

function renderHints(snapshot) {
  elements.hints.replaceChildren();
  const revealed = snapshot.revealed_hints || [];
  for (let index = 0; index < snapshot.hint_count; index += 1) {
    const visible = index < revealed.length;
    const hint = textNode("div", `hint${visible ? "" : " locked"}`, "");
    const label = `hint ${index + 1} of ${snapshot.hint_count}`;
    hint.append(textNode("div", "comhead", visible ? label : `${label} [locked]`));
    if (visible) hint.append(textNode("div", "commtext", revealed[index]));
    elements.hints.append(hint);
  }
  elements.hintCount.textContent = `${snapshot.hints_used}/${snapshot.hint_count} hints`;
  elements.hintInstruction.classList.toggle("hidden", snapshot.hints_used >= snapshot.hint_count);
}

function addMeta(text) {
  elements.completionMeta.append(textNode("span", "", text));
}

function renderCompletion(snapshot) {
  const complete = snapshot.state === "completed";
  elements.completion.classList.toggle("hidden", !complete);
  if (!complete) return;
  elements.completionTitle.textContent = snapshot.first_completion
    ? `Objective confirmed · +${snapshot.xp_awarded} XP`
    : "Replay objective confirmed";
  elements.explanation.textContent = snapshot.explanation || "The observable mission outcome has been confirmed.";
  elements.completionMeta.replaceChildren();
  for (const command of snapshot.discovered_commands || []) addMeta(`new command: ${command}`);
  for (const achievement of snapshot.unlocked_achievements || []) addMeta(`achievement: ${achievement}`);
}

function render(event) {
  const snapshot = event && event.snapshot;
  if (!snapshot) return;
  latestState = snapshot.state;
  elements.waiting.classList.add("hidden");
  elements.layout.classList.remove("hidden");
  elements.location.textContent = locationLabel(snapshot);
  elements.difficulty.textContent = `(${snapshot.difficulty})`;
  elements.missionNumber.textContent = `${snapshot.number}.`;
  elements.title.textContent = snapshot.title;
  elements.reward.textContent = snapshot.replaying ? "xp claimed" : `${snapshot.reward_available} xp available`;
  elements.story.textContent = snapshot.story;
  elements.objective.textContent = snapshot.objective;
  elements.statePill.textContent = snapshot.state;
  elements.statePill.dataset.state = snapshot.state;

  const total = (snapshot.outcomes || []).length;
  const satisfied = snapshot.satisfied_outcomes || 0;
  elements.progressLabel.textContent = `${satisfied}/${total} checks`;

  renderOutcomes(snapshot);
  renderCommands(snapshot);
  renderHints(snapshot);
  renderCompletion(snapshot);
}

async function loadCurrentState() {
  try {
    const response = await fetch("/api/state", { cache: "no-store" });
    if (response.status === 204) return;
    if (!response.ok) throw new Error(`state request failed: ${response.status}`);
    render(await response.json());
  } catch (_error) {
    setConnection("offline", "cli unavailable");
  }
}

function connect() {
  const events = new EventSource("/api/events");
  events.addEventListener("open", () => setConnection("live", "cli connected"));
  events.addEventListener("snapshot", (message) => {
    try {
      render(JSON.parse(message.data));
    } catch (_error) {
      setConnection("offline", "invalid update");
    }
  });
  events.addEventListener("error", () => {
    const ended = latestState === "completed" || latestState === "paused";
    setConnection("offline", ended ? "cli session ended" : "reconnecting to cli");
  });
}

loadCurrentState();
connect();
