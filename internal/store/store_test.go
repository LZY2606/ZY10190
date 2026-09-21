package store

import (
	"context"
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "wellalign.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestInitialConflictThenRevokeAndSolve(t *testing.T) {
	db := openTestStore(t)
	ctx := context.Background()
	state, err := db.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.Latest == nil || state.Latest.OK {
		t.Fatal("initial solve run should record blocking fixture conflict")
	}
	state, err = db.RevokeControl(ctx, "x2")
	if err != nil {
		t.Fatal(err)
	}
	if state.Latest == nil || !state.Latest.OK || len(state.Latest.Result.Candidates) != 3 {
		t.Fatalf("revoked solve should retain three candidates: %#v", state.Latest)
	}
}

func TestExportResetImportRoundTrip(t *testing.T) {
	db := openTestStore(t)
	ctx := context.Background()
	if _, err := db.RevokeControl(ctx, "x2"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := db.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	foundRevoked := false
	for _, control := range snapshot.Controls {
		if control.ID == "x2" && control.Status == "revoked" {
			foundRevoked = true
		}
	}
	if !foundRevoked {
		t.Fatal("export should retain revoked manual control state")
	}
	if len(snapshot.SolveRuns) < 2 {
		t.Fatalf("export should retain initial and post-revoke runs, got %d", len(snapshot.SolveRuns))
	}
	if err := db.ResetFixture(ctx); err != nil {
		t.Fatal(err)
	}
	state, err := db.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.Latest.OK {
		t.Fatal("reset fixture should restore initial blocking conflict")
	}
	if err := db.Import(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	state, err = db.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.Latest == nil || !state.Latest.OK {
		t.Fatal("imported snapshot should replay revoked-state feasible result")
	}
}
