# contrib — optional git hooks

## `post-commit`

`contrib/post-commit` is a one-line wrapper that calls the meridian helper:

```sh
#!/bin/sh
exec meridian hook post-commit
```

Install it as `<repo>/.git/hooks/post-commit` (mode 0755), or symlink it.
Git ignores a post-commit hook's exit status, so a missing `meridian`
binary or an unreadable database never blocks a commit.

## Why the helper shape

The mapping from repo path to subject lives in the meridian config, not in
the hook script, so the script stays a static two-liner. The helper does the
lookup itself:

1. `git rev-parse --show-toplevel` resolves the current repo root.
2. The root is looked up in `[hooks.repos]`.
3. If mapped and the subject exists in the registry, one `occurrence` event
   is written: `source="hook"`, `type="occurrence"`, payload
   `{"source":"hook"}`, `ts` = now (UTC RFC 3339).
4. If there is no mapping, or the mapped subject does not exist, the event is
   **dropped silently** (nothing printed, exit 0).

## Config

```toml
[hooks.repos]
"/home/caffeine/Documents/Projects/burg-rt" = "project/burg-rt"
```

Keys are absolute repo roots exactly as `git rev-parse --show-toplevel`
reports them (symlinks resolved). Values should be `project/*` tracked-thing
ids — spec 04's hook ingestion is project-oriented. Create the thing first
(`meridian track add project/burg-rt --name "burg-rt"`), otherwise the hook
keeps dropping silently.

## Archived tier

Hook events are archived-tier only (spec 00's banned-metrics list): they
never appear in `today` or `week`. Keep the mapped `project/*` things
archived — archived things are excluded from the daily/weekly views by
design.
