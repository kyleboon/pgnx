# Issue tracker: Beads (bd)

Issues and specs for this repo live in Beads, a local Dolt database under `.beads/`.
Use the `bd` CLI for all operations; run `bd prime` for the full workflow reference.
Do not use GitHub Issues for this repo even though the remote is on GitHub.

Issue IDs look like `pgnx-phase4` or `pgnx-engine.1` (children get a dotted suffix).

## Conventions

- **Create an issue**: `bd create "Title" -t task -p 2 -d "..."`. Use `--body-file -`
  with a heredoc for multi-line descriptions. Types: `bug|feature|task|epic|chore|decision|spike|story|milestone`.
  Priority is `0`–`4` (0 highest, default 2). `--acceptance`, `--design`, `--notes`
  carry the structured fields; `--spec-id` links a spec document.
- **Read an issue**: `bd show <id>`; `bd show <id> --json --include-comments` for machine-readable output.
- **List issues**: `bd list --status open --json`, with `-l <label>`, `-t <type>`, `-a <assignee>`,
  `--parent <id>`, `--no-assignee` as filters. `bd ready` lists unblocked, unassigned work.
- **Make an issue a child of a parent**: `bd create ... --parent <parent-id>`, or
  `bd update <child> --parent <parent-id>` afterwards. `bd show <parent> --children` lists them.
- **Comment on an issue**: `bd comments add <id> "..."` (or `-f file`). `bd comments <id>` lists them.
- **Apply / remove labels**: `bd label add <id> label1,label2` / `bd label remove <id> label`.
  `bd update <id> --add-label x --remove-label y` also works. Labels are free-form strings;
  nothing needs pre-creating. `bd label list-all` shows every label in use.
- **Assign / claim**: `bd update <id> --claim` (sets assignee to you and status to `in_progress`).
- **Close**: `bd close <id> -r "reason"`.
- **Specs**: design specs are Markdown files under `docs/superpowers/specs/`; the issue that
  tracks a spec should name the file in its description or `--spec-id`.

## Pull requests as a triage surface

**PRs as a request surface: no.** _(Set to `yes` if this repo treats external PRs as feature
requests; `/triage` reads this flag. Beads has no PR view, so a `yes` would use `gh pr` for the
PR side and `bd` for the resulting issue.)_

## When a skill says "publish to the issue tracker"

Create a Beads issue with `bd create`.

## When a skill says "fetch the relevant ticket"

Read it with `bd show <id>` as in **Read an issue** above.

## Wayfinding operations

Used by `/wayfinder`. The **map** is a single epic with **child** issues as tickets.

- **Map**: `bd create "..." -t epic -l wayfinder:map`, holding the Notes / Decisions-so-far / Fog
  body in its description. Update the body with `bd update <map> --body-file -`.
- **Child ticket**: `bd create "..." --parent <map> -l wayfinder:<type>`
  (`research`/`prototype`/`grilling`/`task`). Children inherit the parent's labels by default;
  pass `--no-inherit-labels` if that is unwanted.
- **Blocking**: Beads native dependencies. `bd dep <blocker> --blocks <blocked>`
  (same as `bd dep add <blocked> <blocker>`). `bd dep tree <map>` shows the graph;
  `bd dep cycles` checks for cycles. A ticket is unblocked when every blocker is closed.
- **Frontier query**: `bd list --parent <map> --status open --no-assignee --json`, then drop any
  that `bd dep list <id>` shows as still blocked (or use `bd ready --no-assignee` and keep only
  the map's children); first in map order wins.
- **Claim**: `bd update <id> --claim`, the session's first write.
- **Resolve**: `bd comments add <id> "<answer>"`, then `bd close <id> -r "..."`, then append a
  context pointer to the map's Decisions-so-far.
