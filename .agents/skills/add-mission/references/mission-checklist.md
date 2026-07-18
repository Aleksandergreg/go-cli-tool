# Mission checklist

## Current content model

Missions are embedded JSON files under `internal/mission/data/`. `internal/mission/catalog.go` decodes them with unknown fields rejected, validates each item, sorts by `number`, and requires a contiguous catalog beginning at 1.

Every mission supplies:

- Identity and placement: `id`, `number`, `title`, `campaign`, `difficulty`.
- An effective learning `track` and `environment`; omitted values preserve the legacy `linux` and `simulated` defaults.
- Teaching content: `story`, `objective`, `hints`, `explanation`.
- Simulated labs: an absolute clean `start_dir` plus declarative `setup` containing `directories`, `files`, `processes`, `environment`, and `archives` as needed.
- Docker labs: `track: docker`, `environment: docker`, and a declarative `docker` setup containing digest-pinned image references and logical `running` or `stopped` container fixtures. They do not define simulated setup or a start directory.
- Declarative `validation.all`: one or more observable conditions.
- `rewards`: positive `xp` and non-negative `hint_penalty`.

Setup paths are absolute and clean. Archive entry paths are relative, clean, and confined to the virtual archive. Modes are octal strings. Environment names follow shell variable naming, PIDs are positive and unique, and one setup path cannot represent conflicting entry kinds.

## Supported validation conditions

Use the smallest combination that proves the outcome without encoding a command route:

- Output: `output_equals`, `output_contains`, `output_contains_all`, `output_not_contains`.
- Navigation: `cwd_equals`.
- Virtual paths: `file_exists`, `dir_exists`, `path_missing`.
- Virtual file state: `file_content_equals`, `file_content_contains`, `file_lines_equal`, `file_mode_equals`, `file_owner_equals`.
- Virtual processes: `process_stopped`, `process_running`.
- Virtual environment: `env_equals` with `NAME=value`.
- Docker state: `docker_container_running` with a declared logical container alias and `docker_container_count_equals` with a non-negative count.

Output-only validation is appropriate when producing the exact useful output is the lesson. Prefer filesystem, process, or environment state when the operational result should survive the final command. Combine positive and negative conditions when distractors could otherwise produce a false completion.

## Coverage checklist

- The learning objective and success outcome are written down before implementation.
- Filename, number, ID, campaign, difficulty, XP, and hint penalty fit adjacent missions.
- Story and objective avoid giving away one exact command.
- Hints progress from concept through the relevant tool or option to concrete help; explanation teaches the underlying operations idea.
- Setup contains no host path, host process, executable hook, mutable Docker tag, privileged option, or player-supplied engine argument.
- Validation accepts every legitimate route and rejects incomplete or collateral outcomes.
- `TestEveryMissionHasAWorkingOutcome` has a canonical solution entry.
- A meaningful alternative route is tested when one exists.
- An incomplete or incorrect route is tested.
- `TestEmbeddedCatalog` count, README counts/campaigns, `scripts/smoke-test.sh` assertions, and iteration references are updated when needed.
- `go test ./internal/mission ./internal/game`, `make validate-missions`, and the relevant repository quality gate have been observed.
