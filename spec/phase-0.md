# meridian — phase 0: scaffold, store, registry

**Hand to the implementing LLM with:** spec 00, 01, 02, 03, and this file.

## Scope

Repository scaffold through a working registry: Go module, store with migrations,
`tracked_things` CRUD, the `track` command group, and `meridian init` seeding the
owner's real catalog. End state: `meridian init && meridian track list` shows the
real life of the user — courses, self-study, German, DSA patterns — each with its
decision rule.

## Tasks

1. `go.mod` (module `github.com/sadisticbrew/meridian`), Makefile (build/test/vet/fmt
   targets per spec 01), `cmd/meridian/main.go`.
2. `internal/store`: Open (WAL, busy_timeout), migration runner (embedded
   `migrations/001_init.sql` containing the full DDL from spec 02, `schema_version`
   tracking, transactional applies).
3. `internal/model`: `Thing` struct with JSON tags.
4. `internal/cli/track.go`: `add`, `list`, `archive`, `restore`, `rule`, `goal` —
   flags exactly as spec 03. Enforce: `--rule` required unless `--archived`; the
   metric-budget warning at >5 active non-archived non-pattern things.
5. `internal/cli/init.go`: `meridian init` — applies migrations, inserts seed rows
   below, idempotent (skip ids that exist; never overwrite existing rules/goals).
6. Subject resolution helper (`internal/cli/resolve.go`): exact id → unique suffix →
   exit 4 with candidates. Landed now because every later phase needs it; unit
   tests cover ambiguity.

## Seed catalog (exact rows — copy verbatim)

Courses (Sem 3, NIE Mysuru):

| id | name | goal | decision rule |
|---|---|---|---|
| `course/ddco` | Digital Design & Computer Organization | weekly_minutes 90 | 0 study hours this week → book one evening block before the next test |
| `course/dav` | Data Analytics & Visualization | weekly_minutes 60 | 0 study hours this week → book one evening block before the next test |
| `course/oop-java` | Object Oriented Programming with Java | weekly_minutes 60 | 0 study hours this week → book one evening block before the next test |
| `course/cdd` | Collaborative Development & DevOps | weekly_minutes 30 | home-turf subject — don't let it cannibalize DDCO evenings |

Self-study:

| id | name | goal | decision rule |
|---|---|---|---|
| `self-study/ostep` | OSTEP (audio study) | weekly_minutes 90 | book one audio-study block this week |
| `self-study/rust` | Rust | weekly_minutes 60 | pair practice with a -rs port; if it stalls two weeks, cut it |

Language:

| id | name | goal | decision rule |
|---|---|---|---|
| `language/german` | German (Nicos Weg) | weekly_minutes 90, occurrences_per_week 3 | 0 min this week → 15 min Nicos Weg tomorrow; B1 cuts PR 27→21 months |

DSA patterns (all with the same rule, no goals — they surface via the week "Focus"
line, not via red rules): `pattern/arrays-hashing`, `pattern/two-pointer`,
`pattern/sliding-window`, `pattern/stack`, `pattern/monotonic-stack`,
`pattern/binary-search`, `pattern/linked-list`, `pattern/trees`, `pattern/trie`,
`pattern/heap`, `pattern/backtracking`, `pattern/intervals`, `pattern/greedy`,
`pattern/dynamic-programming`, `pattern/unclassified`.
Rule: `reviews ≥ solves this week → re-drill that pattern's trigger cards`.

Archived (created with `--archived`):

| id | name | decision rule |
|---|---|---|
| `habit/instagram` | Instagram cycle | reinstall logged → note the trigger, don't spiral |

**No `project/*` rows are seeded.** The active-project slot is created when a
project actually starts, e.g. on Dec 13:
`meridian track add project/sonar --name "Sonar" --rule "weekends only — weekday hours mean something is wrong"`

## Acceptance criteria

- [ ] `make build && make test && make vet` green on a clean checkout.
- [ ] `meridian init` on a fresh `--db` creates schema + all seed rows; running it
      twice changes nothing (diff the `--json` of `track list`).
- [ ] `meridian track add project/test --name T --rule r` then `track list` shows it;
      `track archive project/test` then `track list` hides it, `--archived` shows it.
- [ ] `track add habit/whatever --name w` (no rule, not archived) fails with the
      anti-vanity error.
- [ ] Subject resolution: `ddco` resolves; `gra` (german + oop-java? no — pick a real
      ambiguity from the seed set) exits 4 listing candidates.
- [ ] Store tests cover CRUD + migration idempotency on temp DBs (spec 01 rules).

## Verification

```
make build && ./bin/meridian init --db /tmp/m.db && ./bin/meridian track list --db /tmp/m.db
make test && make vet && gofmt -l .
```

## Out of scope

Everything touching the `events` table (phase 1), `sync`, views, config file parsing
(no config needed yet), `--json` beyond what track tests require.
