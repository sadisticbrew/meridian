# sleep-proxy — verified journal findings (CachyOS, systemd)

Owner machine: CachyOS (Arch-based), journald, `systemd-sleep` on every
suspend/resume cycle.

## Verified marker lines (verbatim)

```
systemd-sleep[<pid>]: Performing sleep operation 'suspend'...
systemd-sleep[<pid>]: System returned from sleep operation 'suspend'.
```

Both carry the syslog identifier `systemd-sleep`; `journalctl -o short-iso`
renders a full line as:

```
2026-09-13T21:14:31+0530 cachy-brew systemd-sleep[29532]: Performing sleep operation 'suspend'...
```

These markers were observed repeatedly and consistently across multiple
suspend/resume cycles in boot -1 and boot 0, and on later boots. `systemd-sleep`
emits them on any Arch/systemd system, so the proxy is reliable on this machine;
it remains labeled experimental in spec 04 because it is a heuristic
(suspend→resume window, not true sleep onset).

## Production invocation

```
journalctl -o short-iso --grep "(Performing|returned from) sleep operation"
```

`journalctl --grep` matches the MESSAGE field only (not the syslog identifier),
so the pattern must match the message text; `systemd-sleep` itself cannot be used
as the grep pattern. The exact argv is `SleepProxy.JournalArgs` (default
`defaultJournalArgs`) and is overridable.

## Timestamp format

`-o short-iso` emits `2006-01-02T15:04:05-0700`-shaped local offsets
(`2026-09-13T21:14:31+0530`). Parsed with layout
`2006-01-02T15:04:05-0700`, falling back to RFC 3339 for the `Z` form; stored UTC.

## Event mapping

- One `sample` event per closed suspend/resume window.
- `source = "sleep-proxy"`, `type = "sample"`, `subject = "sleep/proxy"`,
  `value_num = hours`, `ts = resume time` (UTC RFC 3339).
- `payload_json = {"suspended_at":..., "resumed_at":..., "hours":...}`.
- `dedup_key = "sleep-proxy/<suspend time UTC RFC 3339>"` — the suspend moment
  identifies the window. `UpsertByDedup` only rewrites payloads on a higher
  `attempts` count, which sample payloads never have, so re-runs are no-ops.
- Stray resumes (no open suspend) and a final unclosed suspend are skipped and
  counted in `Result.Details`.
- `sleep/proxy` is self-healed by the collector as kind `habit`,
  `archived = true`, `active = false`, empty `decision_rule` (same shape as the
  seeded `habit/instagram`).

## Test strategy

No test runs `journalctl`. `parseJournal` is a pure function over lines, and
`SleepProxy.Lines` is an injected lines-provider used by store tests with
fixture journal text (including short-iso `+0530` and `Z` forms). DBs live in
`t.TempDir()`.
