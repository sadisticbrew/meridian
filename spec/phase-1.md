# meridian — phase 1: events, timer, manual logging, core views

**Hand to the implementing LLM with:** spec 00–03 and this file. Phase 0 must be
merged and green first.

## Scope

The daily loop, end to end: start/stop timer, manual `log`, notes, and the `today` /
`week` views with wins-first rendering and red rules. After this phase the owner can
run a full real day: morning glance, evening study blocks, Sunday catch-up — and see
an honest, decision-rule-annotated summary. This is the phase that must ship in one
weekend; keep it that small.

## Tasks

1. **Store event API** (spec 02): `AddEvent`, window queries, running-session state
   (`state` table key `active-session`, value
   `{"subject":"course/ddco","started_at":"...","note":""}`).
2. **Timer commands** (`internal/cli/session.go`):
   - `start` — refuses if a session runs (prints its subject + elapsed).
   - `stop` — floor minutes from `started_at`; **< 60s → discard with message**;
     prints exactly one line: `1h10m on DDCO — logged.`
   - `status`, `abort` per spec 03.
   - Critical property: timer state is in the DB, so `start` in one terminal and
     `stop` in another works, and a crashed terminal loses nothing.
3. **Manual logging** (`internal/cli/log.go`): `log <subject> --minutes N`
   (back-fill, `--date`/`--at` defaults today/20:00 local), `log german --lesson N`,
   `log <pattern> --problem S --outcome ...`, `log <course> --score N --exam E
   [--max N]`, and `note "text"` (raw note, subject NULL, quantified 0).
4. **Views** (`internal/views/`, pure `io.Writer` functions — the TUI in phase 3
   reuses them):
   - `today`: exact layout per spec 03 (running line, "Banked", "Gaps" with the
     7-day-zero rule).
   - `week`: "Done" (project/course/German/DSA sections — DSA section in this phase
     renders only manual solves; passive ones arrive in phase 2), "Gaps" (goal
     shortfalls, exact format from spec 02), "Focus next week" line from manual
     review/struggle events.
   - `subject <id>`: 14-day compact minute bars + occurrence history.
   - Week boundaries: Monday-start, local time.
5. **`dump`**: NDJSON per spec 02, `--since/--type/--subject` filters.
6. **`--json`** on `today`, `week`, `subject`.

## Acceptance criteria

- [ ] Simulated day (script against a temp DB): `start ddco` → `stop` → `log german
      --lesson 44` → `note "struggled on k-maps"` → `today` shows all of it in
      wins-first order.
- [ ] Crash test: `start ddco`, kill the process (no `stop`), new process runs
      `status` → shows the running session; `stop` closes it with correct minutes.
- [ ] `stop` at 40s discards with a message and writes no event.
- [ ] `start` twice → second refuses (exit 1) and reports the running session.
- [ ] Red rules: with `language/german` at 0 events for 8 days, `today` shows the
      German gap line with its decision rule; with a 90-minute week goal unmet,
      `week` shows `⚠ German: 40m / 90m — ...`.
- [ ] Golden-file tests for `today` and `week` (fixture DB, fixed local-time
      injection — views take a `now time.Time` parameter; never call `time.Now()`
      inside views).
- [ ] `dump | wc -l` equals event count; JSON round-trips (unmarshal each line).
- [ ] `make build && make test && make vet && gofmt -l .` all clean.

## Verification

```
make build && make test && make vet
./bin/meridian start ddco --db /tmp/m.db && ./bin/meridian stop --db /tmp/m.db
./bin/meridian today --db /tmp/m.db
```

## Out of scope

`sync` (phase 2 — do not even register the command), `quantify` (phase 4 — `note`
stores, nothing processes), `ingest`, graphs, `export html`, TUI (phase 3), config
file. Notes are write-only in this phase; that is intentional.
