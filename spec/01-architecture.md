# meridian — spec 01: architecture & conventions

## Stack

- **Language:** Go. Module path `github.com/sadisticbrew/meridian`. Minimum Go 1.24.
- **Single static binary**, name `meridian`, no cgo anywhere.
- **SQLite:** `modernc.org/sqlite` (pure-Go driver). This is a hard constraint —
  no `mattn/go-sqlite3` (cgo), no embedded Postgres, no server.
- **CLI framework:** `spf13/cobra`.
- **Config parsing:** `BurntSushi/toml`.
- **Graphs (phase 3):** `guptarohit/asciigraph` for terminal sparklines.
- **TUI (phase 3):** `charmbracelet/bubbletea` + `charmbracelet/lipgloss`.

This is the **complete dependency list**. Adding any dependency outside it requires
updating this spec file first with a one-line justification.

## Paths (XDG)

| Purpose | Path | Flag override |
|---|---|---|
| Database | `$XDG_DATA_HOME/meridian/meridian.db` (default `~/.local/share/meridian/meridian.db`) | `--db` |
| Collector mirrors | `$XDG_DATA_HOME/meridian/mirror/<collector>/` | — |
| HTML export | `$XDG_DATA_HOME/meridian/export.html` | `--out` |
| Config | `$XDG_CONFIG_HOME/meridian/meridian.toml` (default `~/.config/...`) | `--config` |

- The config file is **optional** — zero configuration is required for phases 0–2.
  `--db` and `--config` overrides exist primarily for tests.
- The binary **never writes** outside `$XDG_DATA_HOME/meridian/` (mirrors included)
  and `$XDG_STATE_HOME` is not used (running-session state lives in the DB `state`
  table, so it survives crashes by construction).

## Package layout

```
cmd/meridian/main.go        # three lines: cli.Execute()
internal/cli/               # one file per command group (track.go, session.go,
                            # log.go, view.go, sync.go, quantify.go, ...)
internal/model/             # event, trackedThing, result types + JSON tags
internal/store/             # Open, migrations, CRUD, queries
internal/config/            # TOML load with defaults
internal/collectors/        # registry.go, neetcode.go (phase 2), sleepproxy.go (phase 4)
internal/normalizer/        # quantify.go, provider_opencode.go, provider_http.go (phase 4)
internal/views/             # today.go, week.go, subject.go, dsa.go — pure functions
                            # writing to io.Writer (reused by CLI and TUI)
internal/render/            # table + sparkline helpers shared by views
migrations/                 # embedded *.sql, applied in numeric order
data/                       # neetcode150-patterns.json (phase 2)
contrib/                    # post-commit hook script (phase 4)
```

## Database rules

- Open with WAL mode and `busy_timeout = 5000`.
- Migrations: `.sql` files embedded via `embed.FS`, tracked in a `schema_version`
  table, applied in order inside a transaction. Never mutate an applied migration —
  add a new one.
- All writes go through `internal/store`. No package outside `store` issues DDL or
  raw INSERT/UPDATE; store methods are the only API.
- Timestamps: stored as **UTC RFC 3339** strings (`2006-01-02T15:04:05Z`).
  Displayed in local time. All "week" boundaries are Monday-start, local time.

## Conventions (mandatory)

- **Errors are returned, never fatal.** No `log.Fatal`, no `os.Exit` outside
  `cmd/`. No panics in `internal/`. (Precedent from the owner's own audit of his
  tracer project: `log.Fatal` in a goroutine under a TUI wrecks the terminal and
  hides the error — G10. Don't repeat it.)
- **IDs:** tracked things use hierarchical kebab-case IDs, `<kind>/<name>`, e.g.
  `course/ddco`, `pattern/monotonic-stack`, `language/german`.
- **Output:** human-readable tables by default; every read command also accepts
  `--json` emitting one JSON object or NDJSON. Machine output is a first-class
  feature (it feeds the HTML export and future tooling).
- **Comments:** sparse, only where the *why* is non-obvious. The owner's house
  style is terse code with external docs.
- **Commits:** `feat:`, `fix:`, `test:`, `docs:`, `chore:` prefixes.
- **No feature flags / dead code.** If a phase file marks something out of scope,
  it does not appear as a stub.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | success |
| 1 | runtime error (bad subject, DB failure, collector error) |
| 2 | usage error (cobra default) |
| 3 | no running session (`stop`/`abort` with nothing running) |
| 4 | ambiguous subject match (multiple candidates; message must list them) |

## Testing rules (per phase, not optional)

- **Table-driven tests** are the default shape.
- **Store tests** run against a DB file in `t.TempDir()`; never the user's DB.
- **Collector tests** build fixture git repositories in `t.TempDir()` using
  `git init` + commits with controlled author dates (`GIT_AUTHOR_DATE`,
  `GIT_COMMITTER_DATE`). No network in tests.
- **View tests** are golden-file tests: expected output committed under
  `internal/views/testdata/`, compared with `cmp.Diff`. Golden files are updated
  deliberately, never to make a test pass.
- Every phase ships with `go test ./...` green and `go vet ./...` clean.
  `gofmt -l .` must be empty.
- No benchmarks are required.

## Makefile

```
build:  ./bin/meridian
test:   go test ./...
vet:    go vet ./...
fmt:    gofmt check
clean:
```

`make build && make test && make vet` is the verification loop every phase ends with.
