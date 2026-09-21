package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"depthalign/internal/domain"
	"depthalign/internal/server"
	"depthalign/internal/store"
)

func TestPageAndAlignmentFlow(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "server.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	handler := server.New(st)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("井深对齐台")) {
		t.Fatalf("page title missing, code=%d", rec.Code)
	}
	post := func(path string, body any) map[string]any {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code >= 300 {
			t.Fatalf("%s: %s", path, w.Body.String())
		}
		out := map[string]any{}
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}
	initial := post("/api/solve", map[string]any{"min_stretch": .75, "max_stretch": 1.2})
	resultJSON, _ := json.Marshal(initial)
	var initialRun struct {
		Result domain.Result `json:"result"`
	}
	_ = json.Unmarshal(resultJSON, &initialRun)
	initialResult := initialRun.Result
	if initialResult.OK || len(initialResult.Conflicts[0].Chain) != 2 {
		t.Fatalf("initial solve should expose conflict chain: %s", resultJSON)
	}
	post("/api/markers/toggle", map[string]any{"id": "x1", "active": false})
	solved := post("/api/solve", map[string]any{"min_stretch": .75, "max_stretch": 1.2})
	solvedJSON, _ := json.Marshal(solved)
	var solvedRun struct {
		Result domain.Result `json:"result"`
	}
	_ = json.Unmarshal(solvedJSON, &solvedRun)
	solvedResult := solvedRun.Result
	if !solvedResult.OK || len(solvedResult.Candidates) != 3 {
		t.Fatalf("toggle did not restore solve: %s", solvedJSON)
	}
	get := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	first := get("/api/export")
	if first.Code != http.StatusOK || !bytes.Contains(first.Body.Bytes(), []byte(`"version":1`)) {
		t.Fatalf("export failed code=%d body=%s", first.Code, first.Body.String())
	}
}

func TestResetReplayAndImportFlow(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "replay.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	handler := server.New(st)
	post := func(path string, body any) []byte {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code >= 300 {
			t.Fatalf("%s: %s", path, w.Body.String())
		}
		return w.Body.Bytes()
	}
	get := func(path string) []byte {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code >= 300 {
			t.Fatalf("%s: %s", path, w.Body.String())
		}
		return w.Body.Bytes()
	}
	post("/api/solve", map[string]any{"min_stretch": .75, "max_stretch": 1.2})
	post("/api/markers/toggle", map[string]any{"id": "x1", "active": false})
	post("/api/solve", map[string]any{"min_stretch": .75, "max_stretch": 1.2})
	bundle := get("/api/export")
	replayResponse := post("/api/replay", nil)
	t.Logf("replay %s", replayResponse)
	state := get("/api/state")
	var replayed struct {
		Dataset domain.Dataset `json:"dataset"`
		Latest  struct {
			Result domain.Result `json:"result"`
		} `json:"latest"`
	}
	if err := json.Unmarshal(state, &replayed); err != nil {
		t.Fatal(err)
	}
	for _, marker := range replayed.Dataset.Markers {
		if marker.ID == "x1" && marker.Active {
			t.Fatal("replay did not restore disabled x1 marker")
		}
	}
	if !replayed.Latest.Result.OK {
		t.Fatal("replayed latest run should be solvable after undoing x1")
	}
	importRequest := httptest.NewRequest(http.MethodPost, "/api/import", bytes.NewReader(bundle))
	importRecorder := httptest.NewRecorder()
	handler.ServeHTTP(importRecorder, importRequest)
	if importRecorder.Code != http.StatusOK {
		t.Fatalf("import exported bundle: %s", importRecorder.Body.String())
	}
}
