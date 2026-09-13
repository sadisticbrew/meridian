# AGENTS.md — meridian

`meridian` is a local-first, terminal-first personal tracking CLI (single SQLite
file, pure-Go). The full design lives in `spec/`; this file is the standing law for
every agent session and every subagent brief.

## Spec is law

- Before implementing anything, read `spec/00-overview.md` through
  `spec/04-plugins.md` plus the assigned `spec/phase-N.md`.
- The phase file's "Out of scope" list is binding. Building ahead is a defect.
- Deviations and resolved ambiguities MUST appear in the final report — silent
  deviation is failure. When the spec is ambiguous, take the simpler reading and
  flag it.
- Only two sanctioned spec-amendment points exist (defined in `spec/phase-4.md`);
  otherwise never edit files under `spec/`.

## Hard constraints

- Go ≥ 1.24, module `github.com/sadisticbrew/meridian`. Single binary, no cgo.
- Locked dependencies: `spf13/cobra`, `modernc.org/sqlite`, `BurntSushi/toml`,
  `guptarohit/asciigraph` (phase 3), `charmbracelet/bubbletea` + `lipgloss`
  (phase 3). No others without updating `spec/01-architecture.md` first.
- SQLite only through `modernc.org/sqlite` — never `mattn/go-sqlite3` (cgo).
- Product law (spec 00): no streaks anywhere; LLM normalizer only ever selects
  tags from closed vocabularies; every displayed number is computed by SQL/Go,
  never by a model; archived things never render in `today`/`week`.

## Code conventions

- No `log.Fatal`, no `os.Exit` outside `cmd/`. No panics in `internal/`. Errors
  are returned up.
- Timestamps: UTC RFC 3339 in storage, local time in display, Monday-start weeks.
- Tracked-thing ids: `<kind>/<name>` kebab-case (`course/ddco`).
- Comments sparse — only non-obvious *why*.
- Views are pure functions writing to `io.Writer`, taking a `now time.Time`
  parameter; never call `time.Now()` inside views (golden tests depend on it).
- Store access only via `internal/store` methods; no package outside it issues
  DDL or raw INSERT/UPDATE. Migrations are append-only; never edit an applied
  migration file.

## Testing rules

- Table-driven by default. Store tests use DBs in `t.TempDir()`, never the
  user's DB. Collector tests build fixture git repos in tempdirs with
  `GIT_AUTHOR_DATE`/`GIT_COMMITTER_DATE` — no network in tests.
- View tests are golden-file (`internal/views/testdata/`); update golden files
  deliberately, never to make a test pass.
- A phase is done only when `make build && make test && make vet` is green and
  `gofmt -l .` is empty. This exact sequence is the commit gate.

## Git protocol

- Conventional commits: `feat:`, `fix:`, `test:`, `docs:`, `chore:`.
- Branch `feat/phase-N`. The orchestrator (main session) is the ONLY committer;
  subagents never run git write commands.
- Never amend, never force-push, never rewrite history.
- Commit only on a green gate, one commit per passed acceptance-criterion item.

## Subagent protocol

- Implementation and test-fix loops go to subagents on the cheap model
  (DeepSeek V4.1 Flash), one bounded task per subagent, briefed with spec
  file+heading references and file paths — never pasted spec bodies.
- Every subagent reports: files changed, exact test command output, and any
  spec ambiguity it hit. Ambiguities get resolved by the orchestrator, not
  guessed by the subagent.
- Retry rule: a subagent failing twice on the same bug stops the loop — the
  orchestrator fixes it surgically itself or reports the problem. Retry spirals
  are the number-one way to burn budget.
