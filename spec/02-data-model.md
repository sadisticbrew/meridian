# meridian — spec 02: data model

The entire future-proofing story rests on two tables: a **registry** of tracked
things and a single **append-only event store**. Adding a new tracked thing is one
registry row. Adding a new data source is a collector (spec 04). No table-per-metric
ever.

## DDL (migration 001)

```sql
CREATE TABLE IF NOT EXISTS tracked_things (
  id            TEXT PRIMARY KEY,   -- 'course/ddco', 'pattern/monotonic-stack'
  kind          TEXT NOT NULL,      -- project|course|self-study|language|pattern|habit
  display_name  TEXT NOT NULL,
  active        INTEGER NOT NULL DEFAULT 1,
  archived      INTEGER NOT NULL DEFAULT 0,
  decision_rule TEXT,              -- printed when the metric is red
  goal_json     TEXT,              -- thresholds, see below
  created_at    TEXT NOT NULL      -- UTC RFC 3339
);

CREATE TABLE IF NOT EXISTS events (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  ts           TEXT NOT NULL,      -- UTC RFC 3339: when the event HAPPENED
  source       TEXT NOT NULL,      -- manual|neetcode|hook|import|quantify
  type         TEXT NOT NULL,      -- session|occurrence|milestone|note|sample
  subject      TEXT,               -- tracked_things.id, NULL allowed for notes
  value_num    REAL,
  value_text   TEXT,
  payload_json TEXT,               -- type-specific, see below
  dedup_key    TEXT UNIQUE,        -- collectors MUST set; manual events leave NULL
  raw_text     TEXT,               -- notes: verbatim, immutable
  quantified   INTEGER NOT NULL DEFAULT 0,
  created_at   TEXT NOT NULL      -- UTC RFC 3339: insertion time
);
CREATE INDEX IF NOT EXISTS idx_events_ts          ON events(ts);
CREATE INDEX IF NOT EXISTS idx_events_subject_ts  ON events(subject, ts);
CREATE INDEX IF NOT EXISTS idx_events_type        ON events(type);

CREATE TABLE IF NOT EXISTS state (
  key   TEXT PRIMARY KEY,          -- e.g. 'active-session'
  value TEXT NOT NULL             -- JSON
);
```

Notes on deliberate choices:

- SQLite `UNIQUE` allows multiple NULLs, so manual events (dedup_key NULL) are
  unaffected by the uniqueness constraint.
- `ts` (event time) vs `created_at` (insertion time) are separate: retroactive
  logging and history imports must not fake insertion time, and views group by
  `ts`.
- `archived` things still collect events (hooks, imports) but are never rendered in
  main views; `active = 0` means "not currently pursuing, show nothing."

## Event types

### `session` — a timed block of focused work

- `subject`: required.
- `payload`: `{"started_at": "...", "ended_at": "...", "minutes": 65, "tags": []}`
  (`"minutes"` is the floor of the duration; `"manual": true` present when
  back-filled via `log --minutes` instead of the timer).
- `ts` = `started_at`.
- Written by `start`/`stop` (timer) or `log --minutes` (back-fill).
- Sessions shorter than 60 seconds are **discarded** at `stop` time with a message
  ("discarded 40s session — too short to be signal").

### `occurrence` — a point event

- `subject`: required.
- German lesson: subject `language/german`, `value_num` = absolute lesson number
  (e.g. 44), `ts` = when logged. Latest-in-window wins when displaying "current
  lesson".
- DSA manual review/struggle: subject `pattern/<id>`, `payload`
  `{"problem": "car-fleet", "outcome": "reviewed|struggled|solved"}`.
- DSA passive solve (source `neetcode`): subject `pattern/<id>` or
  `pattern/unclassified`, `payload`
  `{"slug": "car-fleet", "attempts": 2, "language": "py", "topic_folder": "..."}`,
  `dedup_key` = `neetcode/<slug>`. **Upsert semantics:** if the dedup key exists and
  the new payload has a higher `attempts` count, update `payload_json` (an existing
  solve event is never duplicated or re-announced — only enriched).
- Hook/import occurrences: whatever the collector says; archived subjects only.

### `milestone` — an exam or score event

- `subject`: a `course/*` thing. `value_num` = marks obtained.
- `payload`: `{"exam": "t1", "max": 25}` — exam is one of `t1|t2|quiz1|quiz2|see`.
- Low volume by design (~30 events/year across all subjects).

### `note` — freeform text awaiting normalization

- `raw_text`: the verbatim note (immutable — normalizer output lives in
  `payload_json`, never overwrites it).
- `subject`: NULL initially; the normalizer may set it? **No** — normalizer output
  goes only into `payload_json`; derived events carry the subject. `subject` stays
  NULL for notes.
- After `quantify`: `payload_json` =
  `{"tags": {...}, "fields": {...}}`, `quantified = 1`. Schema in spec 04.

### `sample` — a periodic numeric measurement (future-proofing slot)

- `value_num` = the measurement (e.g. sleep hours from the phase-4 proxy).
- `subject`: e.g. `sleep/proxy` (an archived thing).
- Nothing in phases 0–3 reads `sample` events; the type exists so phase 4 and later
  need zero schema changes.

## `goal_json` and red rules

Per tracked thing, a JSON object of thresholds. Only two keys have meaning today:

```json
{"weekly_minutes": 90}        // session subjects: red if this week's minutes < target
{"occurrences_per_week": 3}  // occurrence subjects: red if count this week < target
```

Evaluation semantics (exact, so implementers don't improvise):

- **`week` view:** for each active, non-archived subject with a goal, compute the
  current Monday-start week's aggregate. If below target, print under the "Gaps"
  section: `⚠ <display>: <actual> / <target> — <decision_rule>`.
- **`today` view:** red only when a subject with a goal has had **zero events in
  the last 7 days** — a daily nag would violate the philosophy. Footer line format:
  `⚠ <display>: nothing in 7 days — <decision_rule>`.
- Subjects without goals never render red — no goal, no guilt line.

## Dump format (NDJSON)

`meridian dump` emits one JSON object per line, field names identical to the
`events` columns, plus `"subject_name"` resolved from the registry:

```json
{"id":41,"ts":"2026-09-13T09:12:00Z","source":"manual","type":"session","subject":"course/ddco","value_num":null,"value_text":null,"payload":{"started_at":"...","ended_at":"...","minutes":65,"manual":true},"raw_text":null,"quantified":0,"subject_name":"Digital Design & Computer Organization","created_at":"2026-09-13T18:32:11Z"}
```

Rules: NDJSON only (no pretty-printing), UTC in dump regardless of display
timezone, key order irrelevant. `dump` must round-trip: `meridian ingest` accepts its
own dump output.

## Queries the store must expose (method names, signatures up to implementer)

- `SubjectsActive()`, `Thing(id)`, `UpsertThing(...)`, `ArchiveThing(id, bool)`
- `AddEvent(e)`, `UpsertByDedup(e)` — the collector upsert
- `RunningSession()` / `SetRunning()` / `ClearRunning()` — state table wrappers
- `SessionsForWindow(subject, from, to)`, `MinutesForWindow(subject, from, to)`
- `OccurrencesForWindow(subject, from, to)`, `LatestOccurrence(subject)`
- `EventsForWindow(subject, from, to)`
- `Notes(onlyUnquantified bool)`, `MarkQuantified(id, payload)`
- `MilestonesForSubject(subject)`

All window queries are half-open `[from, to)` on `ts`.
