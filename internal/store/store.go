package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"depthalign/internal/domain"
	"depthalign/internal/fixture"
)

type Store struct {
	db *sql.DB
}

type Operation struct {
	ID        int64                  `json:"id"`
	Kind      string                 `json:"kind"`
	MarkerID  string                 `json:"marker_id,omitempty"`
	Payload   map[string]interface{} `json:"payload,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

type RunRecord struct {
	ID         int64           `json:"id"`
	SelectedID string          `json:"selected_id"`
	Config     json.RawMessage `json:"config"`
	Result     domain.Result   `json:"result"`
	CreatedAt  time.Time       `json:"created_at"`
}

type Bundle struct {
	Version    int            `json:"version"`
	ExportedAt time.Time      `json:"exported_at"`
	Dataset    domain.Dataset `json:"dataset"`
	Operations []Operation    `json:"operations"`
	Runs       []RunRecord    `json:"runs"`
}

func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.ensureSeed(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
PRAGMA journal_mode=WAL;
CREATE TABLE IF NOT EXISTS metadata (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS datasets (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  data TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS operations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  kind TEXT NOT NULL,
  marker_id TEXT NOT NULL DEFAULT '',
  payload TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  selected_id TEXT NOT NULL DEFAULT '',
  config TEXT NOT NULL DEFAULT '{}',
  result TEXT NOT NULL,
  created_at TEXT NOT NULL
);`)
	return err
}

func (s *Store) ensureSeed(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM datasets`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return replaceDataset(ctx, s.db, fixture.Build(), nil)
}

func (s *Store) Reset(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM runs; DELETE FROM operations; DELETE FROM datasets;`); err != nil {
		return err
	}
	data, _ := json.Marshal(fixture.Build())
	if _, err := tx.ExecContext(ctx, `INSERT INTO datasets(id,data,updated_at) VALUES(1,?,?)`, string(data), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO operations(kind,marker_id,payload,created_at) VALUES(?,?,?,?)`, "reset_fixture", "", "{}", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) LoadDataset(ctx context.Context) (domain.Dataset, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT data FROM datasets WHERE id=1`).Scan(&raw)
	if err != nil {
		return domain.Dataset{}, err
	}
	var data domain.Dataset
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return domain.Dataset{}, err
	}
	return data, nil
}

func (s *Store) SaveDataset(ctx context.Context, data domain.Dataset, op Operation) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := writeDataset(ctx, tx, data); err != nil {
		return err
	}
	if err := insertOperation(ctx, tx, op); err != nil {
		return err
	}
	return tx.Commit()
}

func replaceDataset(ctx context.Context, db *sql.DB, data domain.Dataset, ops *[]Operation) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM datasets`); err != nil {
		return err
	}
	if err := writeDataset(ctx, tx, data); err != nil {
		return err
	}
	if ops != nil {
		if _, err := tx.ExecContext(ctx, `DELETE FROM operations`); err != nil {
			return err
		}
		for _, op := range *ops {
			if err := insertOperation(ctx, tx, op); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func writeDataset(ctx context.Context, tx *sql.Tx, data domain.Dataset) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO datasets(id,data,updated_at) VALUES(1,?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data, updated_at=excluded.updated_at`, string(raw), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func insertOperation(ctx context.Context, tx *sql.Tx, op Operation) error {
	if op.CreatedAt.IsZero() {
		op.CreatedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(op.Payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO operations(kind,marker_id,payload,created_at) VALUES(?,?,?,?)`, op.Kind, op.MarkerID, string(payload), op.CreatedAt.Format(time.RFC3339Nano))
	return err
}

func (s *Store) ListOperations(ctx context.Context) ([]Operation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,kind,marker_id,payload,created_at FROM operations ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanOperations(rows)
}

func (s *Store) SaveRun(ctx context.Context, cfg domain.SolveConfig, result domain.Result) (RunRecord, error) {
	cfgRaw, err := json.Marshal(cfg)
	if err != nil {
		return RunRecord{}, err
	}
	resultRaw, err := json.Marshal(result)
	if err != nil {
		return RunRecord{}, err
	}
	created := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `INSERT INTO runs(selected_id,config,result,created_at) VALUES(?,?,?,?)`, result.SelectedID, string(cfgRaw), string(resultRaw), created.Format(time.RFC3339Nano))
	if err != nil {
		return RunRecord{}, err
	}
	id, _ := res.LastInsertId()
	return RunRecord{ID: id, SelectedID: result.SelectedID, Config: cfgRaw, Result: result, CreatedAt: created}, nil
}

func (s *Store) ListRuns(ctx context.Context) ([]RunRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,selected_id,config,result,created_at FROM runs ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRuns(rows)
}

func (s *Store) Export(ctx context.Context) (Bundle, error) {
	data, err := s.LoadDataset(ctx)
	if err != nil {
		return Bundle{}, err
	}
	ops, err := s.ListOperations(ctx)
	if err != nil {
		return Bundle{}, err
	}
	runs, err := s.ListRuns(ctx)
	if err != nil {
		return Bundle{}, err
	}
	return Bundle{Version: 1, ExportedAt: time.Now().UTC(), Dataset: data, Operations: ops, Runs: runs}, nil
}

func (s *Store) Import(ctx context.Context, bundle Bundle) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, table := range []string{`DELETE FROM runs`, `DELETE FROM operations`, `DELETE FROM datasets`} {
		if _, err := tx.ExecContext(ctx, table); err != nil {
			return err
		}
	}
	if err := writeDataset(ctx, tx, bundle.Dataset); err != nil {
		return err
	}
	for _, op := range bundle.Operations {
		if err := insertOperation(ctx, tx, op); err != nil {
			return err
		}
	}
	for _, run := range bundle.Runs {
		cfgRaw, _ := json.Marshal(run.Result.Config)
		if len(run.Config) > 0 {
			cfgRaw = run.Config
		}
		resultRaw, err := json.Marshal(run.Result)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO runs(id,selected_id,config,result,created_at) VALUES(?,?,?,?,?)`, run.ID, run.SelectedID, string(cfgRaw), string(resultRaw), run.CreatedAt.Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func scanOperations(rows *sql.Rows) ([]Operation, error) {
	var out []Operation
	for rows.Next() {
		var op Operation
		var payload, created string
		if err := rows.Scan(&op.ID, &op.Kind, &op.MarkerID, &payload, &created); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(payload), &op.Payload); err != nil {
			return nil, err
		}
		op.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, op)
	}
	return out, rows.Err()
}

func scanRuns(rows *sql.Rows) ([]RunRecord, error) {
	var out []RunRecord
	for rows.Next() {
		var run RunRecord
		var cfg, result, created string
		if err := rows.Scan(&run.ID, &run.SelectedID, &cfg, &result, &created); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(result), &run.Result); err != nil {
			return nil, fmt.Errorf("run %d: %w", run.ID, err)
		}
		run.Config = json.RawMessage(cfg)
		run.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, run)
	}
	return out, rows.Err()
}
