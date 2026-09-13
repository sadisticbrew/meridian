# meridian — spec 04: plugins (collectors, normalizer, providers)

Two extension points exist, both compiled-in (no dynamic plugin loading — a new
source is a new Go file in `internal/collectors/`, which is cheap by design).

## Collectors

```go
type Collector interface {
    Name() string
    // Sync fetches from the source and writes events to the store.
    // Must be idempotent: running twice must produce zero duplicate events
    // (dedup_key + upsert, spec 02). Must not fail the whole sync on network
    // errors — return them; the sync command reports per-collector.
    Sync(ctx context.Context, st *store.Store) (Result, error)
}

type Result struct {
    New     int
    Updated int
    Details string // human line, e.g. "attempts +1 on car-fleet"
}
```

`meridian sync` iterates a compiled-in registry (`map[string]Collector`); `--only`
selects one. Collectors MUST set `source` (`neetcode`, `hook`, `import`,
`sleep-proxy`) and `dedup_key` on every event they write, and MUST use the store's
upsert method.

### Collector: `neetcode` (phase 2)

Source repo (public, no auth): `https://github.com/sadisticbrew/neetcode-submissions.git`
(override in config, `[collectors.neetcode] url = ...`).

On-disk layout produced by NeetCode's GitHub sync:

```
Data Structures & Algorithms/<problem-slug>/submission-0.py
                                            submission-1.py
```

Algorithm:

1. Mirror: if `~/.local/share/meridian/mirror/neetcode/` doesn't exist, `git clone`
   (bare not required) the URL there. Else `git fetch origin && git reset --hard
   origin/main` (the mirror is a cache, never edited).
2. Walk `git log --diff-filter=A --name-only --format='%H %at'` to find, per file,
   the **first-add commit**. Group files by problem directory:
   - solve `ts` = author date of the first-add of `submission-0.*`
   - `attempts` = highest `submission-N` index present at current HEAD + 1
   - `language` = extension (`py`, `go`, `rs`, ...)
   - problem `slug` = directory name
3. Pattern: look up `data/neetcode150-patterns.json` (a flat
   `{"<slug>": "<pattern-id>"}` map, values must exist as `pattern/*` tracked
   things — a validation test enforces this). Unknown slug → subject
   `pattern/unclassified`.
4. Upsert by `dedup_key = "neetcode/<slug>"`. If the event exists and the new
   `attempts` is higher, update `payload_json` only (enrich, never duplicate) and
   count it in `Result.Updated`. Otherwise skip.
5. Network/git errors: return the error with the collector name; `sync` prints
   "neetcode: unavailable (offline?) — skipped" and continues.

Edge cases: rebase/force-push on the mirror (reset --hard handles it), multiple
languages for the same slug (first-add wins for ts, attempts uses max across
languages), slugs with URL-escaped characters (leave as-is).

### Collector: `sleep-proxy` (phase 4, experimental, archived)

Derives sleep hours from suspend/resume in the system journal. Heuristic, documented
as a proxy: sleep window = [last recorded user activity before suspend, resume].
Emits `sample` events, `subject = sleep/proxy` (an **archived** tracked thing —
invisible in main views by design). Implementer discovers exact journal entries on
CachyOS via `journalctl --grep suspend --since ...`; the acceptance test uses a
fixture journal file, not a live one.

## Normalizer (`quantify`, phase 4)

Turns `note` events into structured annotations. Four hard rules (spec 00):

1. **Closed vocabularies only.** Valid tag values come from the registry itself:
   - key `subject` → any tracked-thing id
   - key `pattern` → any `pattern/*` id
   - `fields.outcome` ∈ {solved, reviewed, struggled}
   - `fields.difficulty` ∈ {easy, medium, hard}
   Anything else in the LLM output → the whole note is **rejected**: warning
   printed, `quantified` stays 0. No partial accepts, no fuzzy matching.
2. **Raw text is immutable.** Output goes to `payload_json` only; `raw_text` is
   never modified. `--redo` can therefore re-run over all history.
3. **Explicit command.** Nothing in `log note` or any view ever calls a provider
   implicitly.
4. **Deterministic aggregation.** Quantify only annotates/derives events; every
   displayed number is still SQL/Go-computed.

### Prompt contract

System prompt (implementers copy verbatim, `<VOCAB>` injected from the registry):

```
You are a data-extraction engine. Convert the user's note into JSON.
Rules:
- Output ONLY valid JSON. No prose, no markdown fences.
- "tags": object; allowed keys "subject", "pattern"; each value MUST come from
  the provided vocabularies. If nothing fits, omit the key.
- "fields": object; allowed keys "minutes" (number), "lesson" (number),
  "problem" (string), "outcome" (solved|reviewed|struggled),
  "difficulty" (easy|medium|hard), "exam" (t1|t2|quiz1|quiz2|see),
  "score" (number).
- If nothing is extractable: {"tags": {},"fields": {}}
Vocabularies: <VOCAB>
```

Expected output on the note: `{"tags": {...}, "fields": {...}}` — stored as the
note's `payload_json`, `quantified = 1`.

### Derivation (materialization)

After a note quantifies, extractable combinations become real events, dedup-keyed
`quantify/<note-id>` so re-runs never duplicate:

| Extracted | Derived event |
|---|---|
| `subject` + `minutes` | `session` (manual-style payload) |
| `lesson` | `occurrence` on `language/german` |
| `pattern` + `problem` + `outcome` | `occurrence` on the pattern |
| `subject` + `exam` + `score` | `milestone` |

`--redo` first deletes ALL events with `source = 'quantify'`, resets
`quantified=0` on notes, then re-runs. `--dry-run` annotates without writing.

### Providers

```go
type Provider interface {
    Name() string
    // Complete sends prompt, returns the model's raw text output.
    Complete(ctx context.Context, prompt string) (string, error)
}
```

Selected via config `[quantify] provider = "opencode" | "http"`.

**`opencode` (first implementation).** Shells out to the user's existing opencode
CLI — no new API keys. Config:

```toml
[quantify.opencode]
command = "opencode run"   # {PROMPT} is appended as the final argument
model = ""                 # optional; passed as -m when set
timeout = "60s"
```

Implementation notes: run with `PROMPT` substituted as one argv element; strip ANSI
escape sequences from stdout; extract JSON as the substring from the first `{` to
the last `}`; on timeout or unparseable output the provider returns an error (the
note simply stays pending — never fatal). Verify the exact `opencode run` flags
against the installed version during phase 4 and record them in this file.

**`http` (adapter).** OpenAI-compatible chat completions. Config:

```toml
[quantify.http]
url = ""                   # e.g. an OpenRouter endpoint
api_key_env = "MERIDIAN_LLM_KEY"   # key from environment, NEVER from config file
model = ""
timeout = "60s"
```

The api key must only ever be read from the environment. It must never appear in
config, logs, or error messages.

## Hook ingestion (`ingest -`, phase 4)

Stdin NDJSON → validate (subject must exist in registry; unknown → reject with
line number, exit 1) → store as-is. The optional git `post-commit` hook
(`contrib/post-commit`) maps repo path → `project/*` subject via config:

```toml
[hooks.repos]
"/home/caffeine/Documents/Projects/burg-rt" = "project/burg-rt"
```

Hook events are archived-tier only (spec 00's banned-metrics list). If no project
thing exists for the repo, the hook event is dropped silently.
