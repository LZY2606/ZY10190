package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"wellalign/internal/align"
)

type ControlPatch struct {
	Kind       *string  `json:"kind"`
	Status     *string  `json:"status"`
	LeftDepth  *float64 `json:"leftDepth"`
	RightDepth *float64 `json:"rightDepth"`
	Penalty    *float64 `json:"penalty"`
	Note       *string  `json:"note"`
}

func (s *Store) UpdateSettings(ctx context.Context, settings align.Settings) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, _ := json.Marshal(settings)
	if _, err := s.db.ExecContext(ctx, `UPDATE runs SET settings = ? WHERE id = ?`, string(data), "run-ZY10190"); err != nil {
		return State{}, err
	}
	if err := s.event(ctx, "update_settings", settings); err != nil {
		return State{}, err
	}
	return s.stateLocked(ctx)
}

func (s *Store) UpdateControl(ctx context.Context, id string, patch ControlPatch) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.stateLocked(ctx)
	if err != nil {
		return State{}, err
	}
	control, table, err := findControl(state, id)
	if err != nil {
		return State{}, err
	}
	applyPatch(&control, patch)
	if err := writeControl(ctx, s.db, table, control); err != nil {
		return State{}, err
	}
	if err := s.event(ctx, "update_control", map[string]any{"id": id, "patch": patch}); err != nil {
		return State{}, err
	}
	if table == "controls" {
		return s.solveLocked(ctx, "control_update")
	}
	return s.stateLocked(ctx)
}

func (s *Store) RevokeControl(ctx context.Context, id string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.db.ExecContext(ctx, `UPDATE controls SET status = 'revoked' WHERE id = ? AND status = 'active'`, id)
	if err != nil {
		return State{}, err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return State{}, fmt.Errorf("控制点 %s 不是可撤销的活动约束", id)
	}
	if err := s.event(ctx, "revoke_control", map[string]string{"id": id}); err != nil {
		return State{}, err
	}
	return s.solveLocked(ctx, "revoke_control")
}

func (s *Store) RestoreControl(ctx context.Context, id string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.ExecContext(ctx, `UPDATE controls SET status = 'active' WHERE id = ? AND status = 'revoked'`, id); err != nil {
		return State{}, err
	}
	if err := s.event(ctx, "restore_control", map[string]string{"id": id}); err != nil {
		return State{}, err
	}
	return s.solveLocked(ctx, "restore_control")
}

func (s *Store) CreateControl(ctx context.Context, control align.Control) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if control.ID == "" {
		return State{}, errors.New("控制点 id 不能为空")
	}
	control.Status = "active"
	if control.Source == "" {
		control.Source = "manual"
	}
	if control.Kind != "soft" {
		control.Kind = "hard"
	}
	if err := execControl(ctx, s.db, control); err != nil {
		return State{}, err
	}
	if err := s.event(ctx, "create_control", control); err != nil {
		return State{}, err
	}
	return s.solveLocked(ctx, "create_control")
}

func (s *Store) AcceptProposal(ctx context.Context, id string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.stateLocked(ctx)
	if err != nil {
		return State{}, err
	}
	var proposal *align.Control
	for index := range state.Proposals {
		if state.Proposals[index].ID == id {
			proposal = &state.Proposals[index]
			break
		}
	}
	if proposal == nil {
		return State{}, fmt.Errorf("自动建议 %s 不存在", id)
	}
	proposal.Source = "auto_confirmed"
	proposal.Status = "active"
	proposal.Kind = "soft"
	if err := execControl(ctx, s.db, *proposal); err != nil {
		return State{}, err
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE proposals SET status = 'accepted' WHERE id = ?`, id); err != nil {
		return State{}, err
	}
	if err := s.event(ctx, "accept_proposal", *proposal); err != nil {
		return State{}, err
	}
	return s.solveLocked(ctx, "accept_proposal")
}

func (s *Store) Solve(ctx context.Context) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.solveLocked(ctx, "manual_solve")
}

func (s *Store) solveLocked(ctx context.Context, reason string) (State, error) {
	state, err := s.stateLocked(ctx)
	if err != nil {
		return State{}, err
	}
	result := align.Solve(state.Run, state.Controls, state.Proposals, state.Settings)
	settingsJSON, _ := json.Marshal(state.Settings)
	resultJSON, _ := json.Marshal(result)
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO solve_runs(selected_candidate, ok, message, settings, result) VALUES (?, ?, ?, ?, ?)`,
		1, result.OK, result.Message, string(settingsJSON), string(resultJSON)); err != nil {
		return State{}, err
	}
	if err := s.event(ctx, "solve", map[string]string{"reason": reason, "ok": fmt.Sprint(result.OK)}); err != nil {
		return State{}, err
	}
	return s.stateLocked(ctx)
}

func (s *Store) event(ctx context.Context, eventType string, payload any) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := addEventLocked(ctx, tx, eventType, payload); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func findControl(state State, id string) (align.Control, string, error) {
	for _, control := range state.Controls {
		if control.ID == id {
			return control, "controls", nil
		}
	}
	for _, proposal := range state.Proposals {
		if proposal.ID == id {
			return proposal, "proposals", nil
		}
	}
	return align.Control{}, "", fmt.Errorf("控制点 %s 不存在", id)
}

func applyPatch(control *align.Control, patch ControlPatch) {
	if patch.Kind != nil {
		control.Kind = *patch.Kind
	}
	if patch.Status != nil {
		control.Status = *patch.Status
	}
	if patch.LeftDepth != nil {
		control.LeftDepth = *patch.LeftDepth
	}
	if patch.RightDepth != nil {
		control.RightDepth = *patch.RightDepth
	}
	if patch.Penalty != nil {
		control.Penalty = *patch.Penalty
	}
	if patch.Note != nil {
		control.Note = *patch.Note
	}
}

func writeControl(ctx context.Context, db *sql.DB, table string, control align.Control) error {
	query := fmt.Sprintf(`UPDATE %s SET kind=?, source=?, status=?, left_depth=?, right_depth=?, penalty=?, note=? WHERE id=?`, table)
	_, err := db.ExecContext(ctx, query, control.Kind, control.Source, control.Status, control.LeftDepth, control.RightDepth, control.Penalty, control.Note, control.ID)
	return err
}
