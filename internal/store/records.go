package store

import (
	"context"
	"encoding/json"
	"fmt"

	"wellalign/internal/align"
)

func (s *Store) Runs(ctx context.Context, limit int) ([]SolveRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, selected_candidate, ok, message, settings, result, created_at FROM solve_runs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []SolveRecord{}
	for rows.Next() {
		var record SolveRecord
		var settingsJSON, resultJSON string
		if err := rows.Scan(&record.ID, &record.Selected, &record.OK, &record.Message, &settingsJSON, &resultJSON, &record.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(settingsJSON), &record.Settings)
		if err := json.Unmarshal([]byte(resultJSON), &record.Result); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) SelectCandidate(ctx context.Context, runID int64, rank int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.db.ExecContext(ctx, `UPDATE solve_runs SET selected_candidate = ? WHERE id = ?`, rank, runID)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return fmt.Errorf("运行记录 %d 不存在", runID)
	}
	return s.event(ctx, "select_candidate", map[string]any{"runId": runID, "rank": rank})
}

func (s *Store) Export(ctx context.Context) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.stateLocked(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT result FROM solve_runs ORDER BY id`)
	if err != nil {
		return Snapshot{}, err
	}
	defer rows.Close()
	snapshot := Snapshot{Version: 1, Run: state.Run, Controls: state.Controls, Proposals: state.Proposals, Settings: state.Settings, SolveRuns: []json.RawMessage{}, Events: []Event{}}
	for rows.Next() {
		var resultJSON string
		if err := rows.Scan(&resultJSON); err != nil {
			return Snapshot{}, err
		}
		snapshot.SolveRuns = append(snapshot.SolveRuns, json.RawMessage(resultJSON))
	}
	if err := rows.Err(); err != nil {
		return Snapshot{}, err
	}
	eventRows, err := s.db.QueryContext(ctx, `SELECT seq, type, payload, created_at FROM events ORDER BY seq`)
	if err != nil {
		return Snapshot{}, err
	}
	defer eventRows.Close()
	for eventRows.Next() {
		var event Event
		var payload string
		if err := eventRows.Scan(&event.Seq, &event.Type, &payload, &event.CreatedAt); err != nil {
			return Snapshot{}, err
		}
		event.Payload = json.RawMessage(payload)
		snapshot.Events = append(snapshot.Events, event)
	}
	return snapshot, eventRows.Err()
}

func (s *Store) Import(ctx context.Context, snapshot Snapshot) error {
	if snapshot.Version != 1 || snapshot.Run.ID == "" {
		return fmt.Errorf("快照版本或井次数据无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM events; DELETE FROM solve_runs; DELETE FROM proposals; DELETE FROM controls; DELETE FROM runs;`); err != nil {
		_ = tx.Rollback()
		return err
	}
	settings := snapshot.Settings
	if settings.MaxCandidates == 0 {
		settings = align.DefaultSettings()
	}
	if err := insertRun(ctx, tx, snapshot.Run, settings); err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, control := range snapshot.Controls {
		if err := insertControl(ctx, tx, control); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	for _, proposal := range snapshot.Proposals {
		if err := insertProposal(ctx, tx, proposal); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	for _, resultJSON := range snapshot.SolveRuns {
		var result align.SolveResult
		if err := json.Unmarshal(resultJSON, &result); err != nil {
			_ = tx.Rollback()
			return err
		}
		settingsJSON, _ := json.Marshal(settings)
		if _, err := tx.ExecContext(ctx, `INSERT INTO solve_runs(selected_candidate, ok, message, settings, result) VALUES (?, ?, ?, ?, ?)`,
			1, result.OK, result.Message, string(settingsJSON), string(resultJSON)); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	for _, event := range snapshot.Events {
		if _, err := tx.ExecContext(ctx, `INSERT INTO events(seq, type, payload, created_at) VALUES (?, ?, ?, ?)`, event.Seq, event.Type, string(event.Payload), event.CreatedAt); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if err := addEventLocked(ctx, tx, "import_snapshot", map[string]string{"runId": snapshot.Run.ID}); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
