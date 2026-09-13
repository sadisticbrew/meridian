# meridian — spec 03: CLI surface

Root command: `meridian`. Global flags: `--db PATH`, `--config PATH`, `--json`
(read commands). Help text style: lowercase, terse, no marketing.

Subject arguments everywhere accept a tracked-thing **id or unique suffix**:
`course/ddco`, `ddco`, `german` → `language/german`. Resolution: exact id → else
unique suffix match on the full id → else error exit 4 listing candidates.

---

## Registry: `meridian track`

```
meridian track add <kind>/<name> --name "Display Name" [flags]
  --rule TEXT        decision rule (REQUIRED unless --archived — anti-vanity is
                     enforced by the CLI itself)
  --weekly-min N     sets goal {"weekly_minutes": N}
  --per-week N       sets goal {"occurrences_per_week": N}
  --archived         create as archived (e.g. diagnostics, habits)
meridian track list [--all] [--archived] [--kind K]
meridian track archive <id>     # active=0, archived=1; events keep collecting
meridian track restore <id>
meridian track rule <id> "new rule text"
meridian track goal <id> --weekly-min N | --per-week N | --clear
```

`track add` warns when the count of active, non-archived headline things exceeds 5:
"metric budget exceeded — retire something or accept this is a vanity metric."

## Timer & logging: `start` / `stop` / `status` / `abort` / `log`

```
meridian start <subject> [--note TEXT]
  # errors (exit 1) if a session is already running, printing its subject + elapsed
meridian status        # "german — 32m" or "no running session" (exit 3)
meridian stop [--note TEXT]
  # writes the session event, prints ONE line: "1h10m on German — logged."
  # sessions < 60s are discarded with a message, not logged
meridian abort         # discards the running session, prints "discarded."
```

Timer state lives in the DB `state` table (key `active-session`) — a crashed or
closed terminal must never lose a running session. `stop` from a *new* terminal
closes it correctly.

```
meridian log <subject> --minutes N [--note TEXT] [--date YYYY-MM-DD] [--at HH:MM]
  # back-filled session; defaults: date=today, at=20:00 local (the Sunday catch-up case)
meridian log german --lesson N            # occurrence, value_num=N
meridian log <pattern> --problem SLUG --outcome solved|reviewed|struggled
meridian log <course> --score N --exam t1|t2|quiz1|quiz2|see [--max N]
meridian note "freeform text"             # type=note, raw_text
```

All manual events: `source=manual`, `dedup_key=NULL`.

## Reading: `today` / `week` / `subject` / `all`

### `meridian today`

Wins-first. Exact section order:

```
Today — Sun Sep 13
─────────────────────
▶ german — 32m (running)

Banked
  DDCO            65m   study
  German          40m   lesson 44 → 46
  DSA             2 solved, 2/2 first-try (car-fleet, daily-temperatures)

Gaps
  ⚠ OSTEP: nothing in 7 days — book one audio-study block this week
```

Rules: "Banked" lists today's sessions (minutes), German lesson delta, DSA solves
with first-try count. "Gaps" only fires on the 7-day-zero rule (spec 02). Running
session is the first line — immediate feedback that logging worked.

### `meridian week [--last]`

Current Monday-start week (`--last` = previous). Sections:

```
Week — Sep 07 → Sep 13
─────────────────────
Done
  Project         4h 05m
  Courses         5h 30m   (DDCO 3h, DAV 1h, OOP-Java 1h 30m)
  German          2h 10m, lessons 42 → 46
  DSA             9 solved, 6/9 first-try
  Scores          DSA t1: 21/25

Gaps
  ⚠ OSTEP: 0m / 90m — book one audio-study block this week

Focus next week
  monotonic-stack — 3 reviews, 1 struggle, 1 solve → re-drill trigger cards
```

"Focus next week": the pattern with the worst (reviewed + struggled) vs solved
balance in the window; printed only if one exists (a pattern with more solves than
reviews is healthy — no line).

### `meridian subject <id> [--days N]` (default 14)

Per-subject: sessions per day (compact bar of minutes), occurrence/milestone
history, current goal vs actual.

### `meridian all [--archived]`

Registry-wide summary. `--archived` is the ONLY place archived things render.

## Sync & normalizer

```
meridian sync [--only neetcode] [--timeout 30s]
  # runs collectors; per-collector line: "neetcode: 3 new, 1 updated"
  # network failure → warning line, NOT a fatal error; other collectors still run;
  # exit 1 only if the DB itself failed
meridian quantify [--pending] [--redo] [--dry-run] [--provider NAME] [--model NAME]
  # phase 4; prints per note: "note #12 → subject:course/ddco, minutes:45"
  # rejected tags are reported and the note stays quantified=0
```

## Output & export

```
meridian graph [minutes|dsa|german] [--days N]   # phase 3, terminal sparklines
meridian export html [--out FILE] [--days N]    # phase 3, self-contained HTML
meridian dump [--since TS] [--type T] [--subject S]   # NDJSON to stdout
meridian ingest -                                 # stdin NDJSON (round-trips dump;
                                                  # validates subject ids, rejects
                                                  # unknown with exit 1)
meridian tui                                      # phase 3
meridian init [--seed]                            # phase 0: create DB + seed registry
```

`init` is idempotent: on an existing DB it re-applies only missing seed rows
(matched by id) and never overwrites user-edited decision rules or goals.

## JSON output

Every read command (`today`, `week`, `subject`, `track list`, `dump`) accepts
`--json` and emits structured data with the same field names as the NDJSON dump.
Views are implemented as pure functions writing to `io.Writer` so the TUI (phase 3)
reuses them without re-deriving aggregation.
