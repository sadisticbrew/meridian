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
