package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"depthalign/internal/domain"
	"depthalign/internal/fixture"
	"depthalign/internal/store"
)

func TestResetExportImportAndReplay(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "align.db")
	st, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := st.LoadDataset(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Curves) != 4 || len(data.Markers) != 7 {
		t.Fatalf("unexpected seed: curves=%d markers=%d", len(data.Curves), len(data.Markers))
	}
	for i := range data.Markers {
		if data.Markers[i].ID == "x1" {
			data.Markers[i].Active = false
		}
	}
	if err := st.SaveDataset(ctx, data, store.Operation{Kind: "marker_toggle", MarkerID: "x1", Payload: map[string]interface{}{"active": false}}); err != nil {
		t.Fatal(err)
	}
	cfg := domain.SolveConfig{MinStretch: .75, MaxStretch: 1.2}
	result := domain.Result{OK: true, SelectedID: "balanced"}
	if _, err := st.SaveRun(ctx, cfg, result); err != nil {
		t.Fatal(err)
	}
	bundle, err := st.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Operations) != 1 || len(bundle.Runs) != 1 {
		t.Fatalf("export missing history: ops=%d runs=%d", len(bundle.Operations), len(bundle.Runs))
	}
	if err := st.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	reset, err := st.LoadDataset(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !findMarker(t, reset, "x1").Active {
		t.Fatal("reset did not restore crossed fixture marker")
	}
	bundle.Runs = nil
	bundle.Operations = bundle.Operations[:1]
	if err := st.Import(ctx, bundle); err != nil {
		t.Fatal(err)
	}
	imported, err := st.LoadDataset(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if findMarker(t, imported, "x1").Active {
		t.Fatal("imported bundle did not restore disabled marker")
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestEmptyDatabaseSeedsFixture(t *testing.T) {
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	data, err := st.LoadDataset(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"gamma:A", "gamma:B", "resistivity:A", "resistivity:B"} {
		if _, ok := data.Curves[name]; !ok {
			t.Fatalf("missing fixed curve %s", name)
		}
	}
	if data.Curves["gamma:A"].Nodata[0] != fixture.GapA || data.Curves["gamma:B"].Nodata[0] != fixture.GapB {
		t.Fatal("common no-data intervals not seeded")
	}
}

func findMarker(t *testing.T, data domain.Dataset, id string) domain.Marker {
	t.Helper()
	for _, marker := range data.Markers {
		if marker.ID == id {
			return marker
		}
	}
	t.Fatalf("marker %s missing", id)
	return domain.Marker{}
}
