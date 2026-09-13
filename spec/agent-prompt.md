# Per-phase implementation prompt (for GLM-5.3-Flash sessions)

Only line 1 and the `{{PHASE_NOTES}}` block change between phases. The static law
is in the repo-root `AGENTS.md`, which opencode auto-loads every session — do not
repeat it in the prompt.

Replace `{{DEEPSEEK_ID}}` with the exact model id from `opencode models` output.

---

```text
Implement phase 4 of the meridian spec.

SPEC: read spec/00-overview.md, spec/01-architecture.md, spec/02-data-model.md,
spec/03-cli-surface.md, spec/04-plugins.md, then spec/phase-{{N}}.md — the phase
file is your assignment; the others are background law. AGENTS.md applies
throughout.

MODE:
- You are the orchestrator. You read and reason about spec; you do NOT write
  code yourself except single-line surgical fixes.
- Delegate every implementation task and every test-fix loop to a subagent
  using model opencode-go/deepseek-v4.1-flash (cheap containment — a flailing subagent is
  discarded at no real cost; you retry with a tighter brief, you do not inherit
  its mess).

SUBAGENT PROTOCOL:
- One subagent = one bounded task: e.g. "implement internal/store/migrations.go
  from the DDL in spec/02-data-model.md", "make TestX pass, here is the failure
  output".
- Brief each subagent with: exact spec section references (file + heading), file
  paths, and a pointer to AGENTS.md. Never paste whole spec files into a brief.
- Require every subagent to report: files changed, exact test command results,
  and any spec ambiguity it hit. If it found an ambiguity, you resolve it —
  never let the subagent guess.
- Subagents NEVER run git write commands; their work stays in the working tree.
  You are the only committer.
- Retry rule (AGENTS.md): a subagent failing twice on the same bug stops the
  loop — fix it surgically yourself or report it to me.

GIT PROTOCOL:
- Work on branch feat/phase-{{N}} (phase 0 only: git init first, commit spec/,
  AGENTS.md, and this file as the initial commit on main).
- Commit only after `make build && make test && make vet && gofmt -l .` is
  clean, one commit per passed acceptance-criterion item:
  `feat(phase-{{N}}): <AC item from spec/phase-{{N}}.md>`

COMPLETION GATE (in order, no skipping):
1. Every acceptance-criteria checkbox in spec/phase-{{N}}.md verified by
   running the actual command — not by reading code.
2. The phase's "Out of scope" list re-read; confirm you built none of it.
3. make build && make test && make vet && gofmt -l . all clean.
4. Merge feat/phase-{{N}} into main, print a final report:
   - AC checklist with pass/fail per item
   - every deviation from the spec, one-line justification each
   - every spec ambiguity you resolved and how
   (Deviations and ambiguities reported silently are failures; I review, you
   don't hide.)

SPECIAL NOTES FOR THIS PHASE:
**Phase 4:** Record the verified `opencode run` invocation and sleep-proxy
journal findings back into spec/04-plugins.md — the two sanctioned spec-amendment
points. API keys only from env; the redaction test is mandatory.
```

---

## Per-phase `{{PHASE_NOTES}}` blocks

**Phase 0:** `git init` first; initial commit of `spec/` + `AGENTS.md` + this
file on main. No `project/*` seed rows — the seed catalog in spec/phase-0.md is
verbatim.

**Phase 1:** Highest flake-risk: timer state must survive process death (state
table, spec 02). Views take a `now time.Time` parameter — golden tests depend on
it. Do not register a `sync` command.

**Phase 2:** The slug→pattern map generation is a subagent task writing
`data/neetcode150-patterns.json` — flag it for my review before merge; do not
merge the map without me. Collector tests use fixture git repos in tempdirs,
never the network.

**Phase 3:** Build order is law: `graph` → `export html` → `tui`. HTML: zero JS,
zero external URLs, < 300 KB — golden test enforces. TUI is read-only this
phase.

**Phase 4:** Record the verified `opencode run` invocation and sleep-proxy
journal findings back into spec/04-plugins.md — the two sanctioned spec-amendment
points. API keys only from env; the redaction test is mandatory.

## Session hygiene (for the human, not the prompt)

1. `opencode models | grep -i deepseek` → note the exact id for `{{DEEPSEEK_ID}}`.
2. One fresh opencode session per phase, main model GLM-5.3-Flash. Never carry a
   prior phase's session forward.
3. When the final report lands, spend five minutes on the deviations/ambiguities
   section — that's your review gate. Approve the merge only then.
