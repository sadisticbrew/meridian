# meridian — spec 00: overview

`meridian` is a single-user, local-first, terminal-first tracking CLI. It answers one
question at a glance: **"am I actually spending time where my plan says it matters —
projects, coursework, German, DSA — and if not, what is the next concrete move?"**

Everything that does not serve that question is out of scope.

## Owner context (for the implementer)

The sole user is a 2nd-year CS undergrad (systems focus: Go, C, Python, learning Rust)
on CachyOS + Hyprland/Wayland, terminal-first and keyboard-driven. Long-range goal:
infrastructure/systems roles in Germany (EU Blue Card route, graduating May 2029) —
which is why German language, DSA interview prep, CGPA recovery, and systems portfolio
projects are the four load-bearing threads of his life. He is in a heavy semester with
weekends-only project time, and has a documented history of project burnout. The tool
must respect both facts: low friction, no nagging, honest numbers.

## Design philosophy — non-negotiable

These rules override feature requests, including the owner's own future ones:

1. **Signal vs noise.** A metric earns a headline slot only if it has a *decision
   rule*: a stored sentence answering "if this number is bad, what do I do
   differently?" Metrics without an answer are vanity and are either rejected or
   archived (stored, never surfaced in main views).
2. **Metric budget: 5 headline metrics.** Adding a 6th requires retiring one. The
   registry enforces this at the conceptual level: `track add` on an active
   non-archived thing beyond the budget should warn loudly.
3. **Wins-first rendering.** Every view leads with what was completed (minutes
   banked, lessons advanced, problems solved, first-try wins) and puts gaps *after*.
   The owner's own words: "walking past finished work without crediting myself" is
   the failure mode this prevents.
4. **No streaks, ever.** Streaks optimize for not-breaking-the-streak. No
   streak/chain/gamified-shame logic may appear anywhere, including the TUI and HTML
   export.
5. **Passive before manual.** If a metric can be collected without the owner typing,
   it must be (e.g. DSA solves arrive via the NeetCode GitHub sync). Manual logging
   is the fallback, never the default plan.
6. **LLMs never compute displayed numbers.** Every number shown is computed by SQL
   or Go from stored events. The LLM normalizer (spec 04) only converts freeform
   text into tags chosen from closed vocabularies. If quantification is unavailable,
   the entire tool still works — offline core, optional intelligence.
7. **Local-first.** Single SQLite file, XDG paths, no server, no accounts, no
   telemetry, no network requirement (the only network use is `git fetch` in the
   NeetCode collector, which degrades gracefully).
8. **Deterministic and replayable.** Collector syncs are idempotent (dedup keys);
   text normalization is replayable (raw text is kept verbatim and can be
   re-processed with a better prompt).

## The intended ritual

The tool is designed around a cadence, not constant checking:

- **Daily, ambient:** `meridian today` — a glance, ideally bound to a Hyprland
  scratchpad terminal keybind. Input expected on a normal day: zero or one command.
- **Per session:** `meridian start <subject>` / `meridian stop` around deep-work
  blocks. DSA solves require nothing — they appear via sync.
- **Weekly, Sunday:** `meridian week && meridian export html` — the review moment,
  with coffee. This is the only moment where red rules demand attention.

## Metric catalog (summary; exact seed rows in phase-0)

**Track now (the 5 headline metrics):**

| # | Metric | Kind of data | Why it survives the decision-rule test |
|---|--------|--------------|----------------------------------------|
| 1 | Deep-work minutes on active project | session | 0 min on a weekend → schedule a block; weekday over-runs → burnout-risk signal |
| 2 | Study minutes per Sem-3 course + self-study | session | subject at 0 hrs before a test → cram risk, reallocate evenings |
| 3 | German minutes + Nicos Weg lesson # | session + occurrence | 0 min in a week → schedule 15 min tomorrow; B1 cuts PR 27→21 months |
| 4 | DSA problems by pattern (solves, attempts, first-try rate) | occurrence (passive) | weak-pattern cluster → re-drill that pattern's trigger cards |
| 5 | Test/exam scores per course | milestone | score below CGPA-9.0 trajectory in a risk subject → adjust allocation |

**Track later (registry supports them; no views until they matter):** OSS engagement
sessions (Year 3), sleep proxy via suspend/resume journal, writing/blog sessions,
imported historical data.

**Rejected (banned from main views):** raw commit counts, LOC, GitHub streaks,
screen time, steps, manual sleep logging. A git-commit hook collector exists only as
an *archived* diagnostic (phase 4).

## Non-goals

- Multi-user, sync service, web app, mobile, browser extension.
- Notifications, reminders, nudges, idle detection of the "you haven't studied"
  kind. The dashboard never initiates contact.
- AI-generated insight prose ("your productivity trend suggests..."). The normalizer
  structures text; it does not editorialize.
- Tracking everything. Refusing to track a thing is a feature.

## Document map

| File | Contents |
|------|----------|
| `01-architecture.md` | stack, paths, package layout, conventions, testing rules |
| `02-data-model.md` | full SQLite DDL, event types, dedup/upsert, dump format |
| `03-cli-surface.md` | every command, flag, output shape, exit code |
| `04-plugins.md` | Collector/Provider interfaces, NeetCode collector, normalizer contract |
| `phase-0.md` … `phase-4.md` | self-contained build specs with acceptance criteria |

**For LLM implementers:** read `01`, `02`, `03`, `04`, then *only* the phase file
assigned to you. Phases are sequential. Implementing a later phase's feature early is
scope creep — each phase file has an explicit "out of scope" list; respect it.
