package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"

	_ "modernc.org/sqlite"

	"wellalign/internal/align"
)

type Store struct {
	db    *sql.DB
	mu    sync.Mutex
	seq   int64
	runID string
}

type Snapshot struct {
	Version   int               `json:"version"`
	Run       align.RunData     `json:"run"`
	Controls  []align.Control   `json:"controls"`
	Proposals []align.Control   `json:"proposals"`
	Settings  align.Settings    `json:"settings"`
	SolveRuns []json.RawMessage `json:"solveRuns"`
	Events    []Event           `json:"events"`
}

type Event struct {
	Seq       int64           `json:"seq"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt string          `json:"createdAt"`
}

type SolveRecord struct {
	ID        int64             `json:"id"`
	Selected  int               `json:"selected"`
	OK        bool              `json:"ok"`
	Message   string            `json:"message"`
	Settings  align.Settings    `json:"settings"`
	Result    align.SolveResult `json:"result"`
	CreatedAt string            `json:"createdAt"`
}

type State struct {
	Run       align.RunData   `json:"run"`
	Controls  []align.Control `json:"controls"`
	Proposals []align.Control `json:"proposals"`
	Settings  align.Settings  `json:"settings"`
	Latest    *SolveRecord    `json:"latest,omitempty"`
	Events    []Event         `json:"events"`
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.ensureFixture(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

func (s *Store) ensureFixture(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM runs`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return s.ResetFixture(ctx)
}

func (s *Store) ResetFixture(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, controls, proposals := align.Fixture()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM events; DELETE FROM solve_runs; DELETE FROM proposals; DELETE FROM controls; DELETE FROM runs;`); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := insertRun(ctx, tx, run, align.DefaultSettings()); err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, control := range controls {
		if err := insertControl(ctx, tx, control); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	for _, proposal := range proposals {
		if err := insertProposal(ctx, tx, proposal); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if err := addEventLocked(ctx, tx, "reset_fixture", map[string]string{"runId": run.ID}); err != nil {
		_ = tx.Rollback()
		return err
	}
	result := align.Solve(run, controls, proposals, align.DefaultSettings())
	settingsJSON, _ := json.Marshal(align.DefaultSettings())
	resultJSON, _ := json.Marshal(result)
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO solve_runs(selected_candidate, ok, message, settings, result) VALUES (?, ?, ?, ?, ?)`,
		1, result.OK, result.Message, string(settingsJSON), string(resultJSON)); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := addEventLocked(ctx, tx, "solve", map[string]string{"reason": "initial_fixture", "ok": fmt.Sprint(result.OK)}); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func insertRun(ctx context.Context, tx *sql.Tx, run align.RunData, settings align.Settings) error {
	runJSON, _ := json.Marshal(run)
	settingsJSON, _ := json.Marshal(settings)
	_, err := tx.ExecContext(ctx, `INSERT INTO runs(id, name, data, settings) VALUES (?, ?, ?, ?)`, run.ID, run.Name, string(runJSON), string(settingsJSON))
	return err
}

func insertControl(ctx context.Context, tx *sql.Tx, control align.Control) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO controls(id, run_id, kind, source, status, left_depth, right_depth, penalty, note, created_seq)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		control.ID, "run-ZY10190", control.Kind, control.Source, control.Status,
		control.LeftDepth, control.RightDepth, control.Penalty, control.Note, control.CreatedSeq)
	return err
}

func execControl(ctx context.Context, db *sql.DB, control align.Control) error {
	_, err := db.ExecContext(ctx, `INSERT INTO controls(id, run_id, kind, source, status, left_depth, right_depth, penalty, note, created_seq)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		control.ID, "run-ZY10190", control.Kind, control.Source, control.Status,
		control.LeftDepth, control.RightDepth, control.Penalty, control.Note, control.CreatedSeq)
	return err
}

func insertProposal(ctx context.Context, tx *sql.Tx, proposal align.Control) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO proposals(id, run_id, kind, source, status, left_depth, right_depth, penalty, note, created_seq)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		proposal.ID, "run-ZY10190", proposal.Kind, proposal.Source, proposal.Status,
		proposal.LeftDepth, proposal.RightDepth, proposal.Penalty, proposal.Note, proposal.CreatedSeq)
	return err
}

func addEventLocked(ctx context.Context, tx *sql.Tx, eventType string, payload any) error {
	data, _ := json.Marshal(payload)
	_, err := tx.ExecContext(ctx, `INSERT INTO events(type, payload) VALUES (?, ?)`, eventType, string(data))
	return err
}

func (s *Store) State(ctx context.Context) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stateLocked(ctx)
}

func (s *Store) stateLocked(ctx context.Context) (State, error) {
	var state State
	var runJSON, settingsJSON string
	err := s.db.QueryRowContext(ctx, `SELECT data, settings FROM runs ORDER BY id LIMIT 1`).Scan(&runJSON, &settingsJSON)
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal([]byte(runJSON), &state.Run); err != nil {
		return state, err
	}
	if err := json.Unmarshal([]byte(settingsJSON), &state.Settings); err != nil {
		state.Settings = align.DefaultSettings()
	}
	state.Controls, err = queryControls(ctx, s.db, false)
	if err != nil {
		return state, err
	}
	state.Proposals, err = queryControls(ctx, s.db, true)
	if err != nil {
		return state, err
	}
	state.Latest, err = s.latestLocked(ctx)
	if err != nil && err != sql.ErrNoRows {
		return state, err
	}
	state.Events, err = s.eventsLocked(ctx, 50)
	return state, err
}

func queryControls(ctx context.Context, db *sql.DB, proposals bool) ([]align.Control, error) {
	table := "controls"
	if proposals {
		table = "proposals"
	}
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`SELECT id, kind, source, status, left_depth, right_depth, penalty, coalesce(note, ''), created_seq FROM %s ORDER BY created_seq`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []align.Control
	for rows.Next() {
		var control align.Control
		if err := rows.Scan(&control.ID, &control.Kind, &control.Source, &control.Status, &control.LeftDepth, &control.RightDepth, &control.Penalty, &control.Note, &control.CreatedSeq); err != nil {
			return nil, err
		}
		out = append(out, control)
	}
	return out, rows.Err()
}

func (s *Store) latestLocked(ctx context.Context) (*SolveRecord, error) {
	var record SolveRecord
	var settingsJSON, resultJSON, createdAt string
	err := s.db.QueryRowContext(ctx, `SELECT id, selected_candidate, ok, message, settings, result, created_at FROM solve_runs ORDER BY id DESC LIMIT 1`).
		Scan(&record.ID, &record.Selected, &record.OK, &record.Message, &settingsJSON, &resultJSON, &createdAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(settingsJSON), &record.Settings)
	if err := json.Unmarshal([]byte(resultJSON), &record.Result); err != nil {
		return nil, err
	}
	record.CreatedAt = createdAt
	return &record, nil
}

func (s *Store) eventsLocked(ctx context.Context, limit int) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT seq, type, payload, created_at FROM events ORDER BY seq DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []Event{}
	for rows.Next() {
		var event Event
		var payload string
		if err := rows.Scan(&event.Seq, &event.Type, &payload, &event.CreatedAt); err != nil {
			return nil, err
		}
		event.Payload = json.RawMessage(payload)
		events = append(events, event)
	}
	return events, rows.Err()
}
