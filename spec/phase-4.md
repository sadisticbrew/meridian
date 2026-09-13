# meridian — phase 4: quantification, hooks, import, sleep proxy

**Hand to the implementing LLM with:** spec 00–04 and this file. Phases 0–3 merged
and green first.

## Scope

The optional-intelligence and extensibility phase. Everything here must obey the
phase-0 philosophy rule: if every item in this phase were deleted, phases 1–3 would
be unaffected. Build the items in order; each is small and independently mergeable.

## Tasks

### 1. Normalizer core: `meridian quantify [--pending] [--redo] [--dry-run]`

Per spec 04: processes `note` events (default `--pending` = all `quantified=0`),
runs the provider, validates against closed vocabularies (registry ids + fixed
enums), stores `payload_json`, sets `quantified=1`. Rejected output → warning line,
note stays pending, **never fatal**. `--redo` deletes all `source='quantify'` events
and resets all notes first. `--dry-run` annotates without writing.

Materialization (spec 04 table): subject+minutes → session; lesson → German
occurrence; pattern+problem+outcome → occurrence; subject+exam+score → milestone.
Derived events: `source='quantify'`, `dedup_key='quantify/<note-id>'`.

Tests use a **fake provider** (interface makes this trivial): happy path, invalid
tag rejection, JSON-in-prose extraction, materialization, redo idempotency.

### 2. Provider: `opencode` (first, real implementation)

Shell out per spec 04 config (`[quantify.opencode]`). Verify the exact `opencode
run` non-interactive invocation against the installed version during development
and record the working command line in `spec/04-plugins.md` (this is the one place
the spec expects the implementer to amend the spec). ANSI stripping, `{`-to-last-`}`
JSON extraction, 60s timeout, error → provider error (note stays pending). An
integration test is allowed to **skip** if `opencode` isn't on PATH (`t.Skip`),
never to fail.

### 3. Provider: `http`

OpenAI-compatible chat completions per spec 04 config. API key only from
`api_key_env` — never in the config file, never in logs or errors (redact on
print). Unit test with `httptest.Server` covering: request shape, response parsing,
non-200, timeout.

### 4. `meridian ingest -` + git hook (archived tier)

Stdin NDJSON → validate subject exists → insert. Unknown subject: reject that line
with its line number, continue processing others, exit 1 at the end. `contrib/post-commit`:

```sh
#!/bin/sh
echo "{\"source\":\"hook\",\"type\":\"occurrence\",\"subject\":\"$MERIDIAN_SUBJECT\",\"ts\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}" \
  | meridian ingest - >/dev/null 2>&1 || true
```

with repo-path → subject mapping from `[hooks.repos]` config (the hook itself just
needs `git rev-parse --show-toplevel` and a lookup; a tiny helper `meridian hook
post-commit` doing the mapping internally is the cleaner shape — implementer's
choice, document it in `contrib/README.md`). Hook subjects must be `project/*`
things; if none exists the event is dropped silently. Hook events never appear in
`today`/`week` — archived tier only (spec 00).

### 5. `meridian import --file F --map M.json`

Generic file importer for future/unknown formats. Mapping JSON:

```json
{"type": "session", "subject": "course/ddco", "ts_field": "date", "value_field": "minutes", "format": "ndjson|csv"}
```

Rows → events, `source='import'`, `dedup_key='import/<sha of file+row>'` (importing
the same file twice is a no-op). Keep this deliberately dumb: no heuristics, the
mapping file does all the work. One NDJSON + one CSV test.

### 6. Collector: `sleep-proxy` (experimental, archived)

Per spec 04: suspend/resume from the system journal → `sample` events on
`sleep/proxy` (an **archived** tracked thing — `meridian track add sleep/proxy
--name "Sleep (proxy)" --archived` creates it; note: `--archived` means no
decision rule required). Sleep window = last user activity before suspend →
resume; hours → `value_num`, resume time → `ts`.

The implementer first discovers the exact journal markers on CachyOS
(`journalctl --list-boots`, `journalctl --grep -i suspend`), records findings in a
short `internal/collectors/sleepproxy_NOTES.md`, and the test uses a fixture
journal file — never the live journal. This is the one phase-4 item explicitly
labeled experimental: if journal markers prove unreliable on the owner's machine,
document that and stop — an unreliable sleep proxy is worse than none.

## Acceptance criteria

- [ ] Fake-provider tests: happy path, tag rejection, materialization, redo
      idempotency (run redo twice → same event count).
- [ ] Quantify with no provider configured → clean error telling the user to
      configure one; nothing crashes, notes intact.
- [ ] `opencode` provider verified live once (manual, noted in PR); test skips
      gracefully without the binary.
- [ ] `http` provider test suite green via httptest; a leaked API key in any error
      string fails a dedicated redaction test.
- [ ] `ingest`: valid+invalid mixed stdin → valid rows stored, exit 1, line
      numbers in output. `dump | ingest -` round-trips (subject ids all valid).
- [ ] Import same file twice → zero new events.
- [ ] Sleep proxy: fixture journal → correct sample events; documented findings.
- [ ] `make build && make test && make vet` clean.

## Verification

```
make build && make test && make vet
echo 'note about studying DDCO for 45 minutes' | ... # via: meridian note "..." && meridian quantify --dry-run --db /tmp/m.db
```

## Out of scope

Any new main-view rendering of archived data, TUI changes, a second LLM prompt
("weekly summary generation" — explicitly rejected as insight prose, spec 00),
web dashboard, sync to other machines.
