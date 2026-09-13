# meridian — phase 2: NeetCode collector, sync, DSA/German/score views

**Hand to the implementing LLM with:** spec 00–04 and this file. Phases 0–1 merged
and green first.

## Scope

The phase that makes the dashboard feel alive: DSA solves flow in passively from
`github.com/sadisticbrew/neetcode-submissions`, and the DSA / German / score views
become the leading indicators the whole design promised (pattern breakdown,
first-try rate, weak-pattern focus line). After this phase the owner's *only*
mandatory daily input is the session timer around study blocks.

## Tasks

### 1. `data/neetcode150-patterns.json`

Flat map of every NeetCode 150 problem slug → pattern id (the registry's
`pattern/*` ids, phase 0 seed). This is a **generated artifact with a human-review
gate**:

- Generation is a one-time LLM-assisted task against the official NeetCode 150
  list, using the owner's trigger-card taxonomy. The generated file is committed
  and the owner reviews the diff before merge — the review is part of the phase,
  not optional.
- Validation test (mandatory, fails CI): every value in the map must be an existing
  `pattern/*` tracked thing; every slug unique.

### 2. Collector `internal/collectors/neetcode.go`

Implement exactly per spec 04: mirror clone/fetch at
`~/.local/share/meridian/mirror/neetcode`, first-add walk for solve timestamps,
attempts from max `submission-N` index at HEAD, language from extension, pattern
lookup with `pattern/unclassified` fallback, upsert on
`dedup_key = "neetcode/<slug>"` that only enriches attempts. Git errors are
returned, never fatal to the command.

Fixture-based tests build a fake NeetCode repo in `t.TempDir()` (`git init`,
committed `Data Structures & Algorithms/car-fleet/submission-0.py` with
`GIT_AUTHOR_DATE` controlled), point the collector at it via config/flag, and
assert:

- first sync → `Result{New: 1}`, event has correct ts/attempts/pattern.
- second sync, no changes → `New: 0, Updated: 0`, still one event.
- a new `submission-1.py` commit → `Updated: 1`, attempts=2 in the same event,
  `ts` unchanged.
- repo URL unreachable → error surfaced as "skipped", exit 0 from `sync` overall.

### 3. `sync` command

`internal/cli/sync.go` per spec 03: iterates the collector registry (only
`neetcode` exists this phase), prints `neetcode: 3 new, 1 updated (attempts +1 on
car-fleet)`, `--only`, `--timeout`. Network failure prints the skip line, exit 0;
only DB failures exit 1.

### 4. DSA view upgrades

- `week`: DSA line becomes `9 solved, 6/9 first-try` (solves from both passive and
  manual `--outcome solved` events; first-try = passive events with attempts=1).
- "Focus next week" now computes across **all** pattern occurrences (passive solves
  count toward health; manual reviews/struggles toward the focus signal): choose
  the pattern with max (reviewed + struggled − solved) > 0.
- `subject pattern/<id>`: per-problem table (slug, attempts, outcome, date) for the
  last 14 days.
- `pattern/unclassified` solves are listed in `week`'s DSA line only when nonzero,
  with a hint: `3 unclassified — add them to data/neetcode150-patterns.json`.

### 5. German + milestone rendering

- `today`/`week` German lines show lesson delta (`42 → 46`) via latest occurrence
  in window vs latest before window.
- `week` "Scores" line lists this week's milestones (`DSA t1: 21/25`).
- `subject course/<id>` shows milestone history table.

## Acceptance criteria

- [ ] Sync twice against the fixture repo → zero duplicate events (query event
      count by dedup_key).
- [ ] Attempts upsert updates payload only; `ts`, `created_at`, `source` immutable.
- [ ] Offline: `sync` with an unreachable URL prints the skip line, exit 0.
- [ ] Patterns map validation test passes; a deliberately bad value fails it.
- [ ] With fixture solves + a manual review, `week` prints the Focus line for the
      reviewed pattern.
- [ ] First-try rate correct on a mixed fixture (2× attempts=1, 1× attempts=3 →
      `2/3 first-try`).
- [ ] Golden-file updates for `week`/`subject` views; `make test && make vet` green.

## Verification

```
make test && make vet
./bin/meridian sync --db /tmp/m.db && ./bin/meridian sync --db /tmp/m.db   # second run: 0 new
./bin/meridian week --db /tmp/m.db
```

## Out of scope

Quantify/providers (phase 4), `note` processing, graphs and HTML (phase 3), sleep
proxy, import, hooks (phase 4), and **no second collector** — resist the urge; one
collector done rigorously validates the interface.
