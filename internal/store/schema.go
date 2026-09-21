package store

const schema = `
PRAGMA journal_mode = WAL;
CREATE TABLE IF NOT EXISTS runs (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  data TEXT NOT NULL,
  settings TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS controls (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  kind TEXT NOT NULL CHECK(kind IN ('hard', 'soft')),
  source TEXT NOT NULL,
  status TEXT NOT NULL,
  left_depth REAL NOT NULL,
  right_depth REAL NOT NULL,
  penalty REAL NOT NULL DEFAULT 0,
  note TEXT,
  created_seq INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS proposals (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  source TEXT NOT NULL,
  status TEXT NOT NULL,
  left_depth REAL NOT NULL,
  right_depth REAL NOT NULL,
  penalty REAL NOT NULL DEFAULT 0,
  note TEXT,
  created_seq INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS solve_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  selected_candidate INTEGER NOT NULL DEFAULT 1,
  ok INTEGER NOT NULL,
  message TEXT NOT NULL,
  settings TEXT NOT NULL,
  result TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS events (
  seq INTEGER PRIMARY KEY AUTOINCREMENT,
  type TEXT NOT NULL,
  payload TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_controls_order ON controls(created_seq);
CREATE INDEX IF NOT EXISTS idx_proposals_order ON proposals(created_seq);
CREATE INDEX IF NOT EXISTS idx_solve_runs_time ON solve_runs(id);
CREATE INDEX IF NOT EXISTS idx_events_time ON events(seq);
`
