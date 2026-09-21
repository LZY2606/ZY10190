package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"wellalign/internal/store"
)

func testServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "web.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	server := httptest.NewServer(New(db))
	t.Cleanup(server.Close)
	return server, db
}

func TestHomeShowsWellAlignmentBench(t *testing.T) {
	server, _ := testServer(t)
	response, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	buffer := make([]byte, 4096)
	count, _ := response.Body.Read(buffer)
	if !strings.Contains(string(buffer[:count]), "井深对齐台") {
		t.Fatal("home page should contain 井深对齐台")
	}
}

func TestAPIConflictRevokeAndExport(t *testing.T) {
	server, _ := testServer(t)
	state := getState(t, server.URL)
	if state.Latest.OK {
		t.Fatal("initial fixture should block on crossed hard pair")
	}
	if len(state.Latest.Result.Conflicts) == 0 || state.Latest.Result.Conflicts[0].Chain[0] != "x1" {
		t.Fatalf("unexpected initial conflicts: %#v", state.Latest.Result.Conflicts)
	}

	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/controls/x2/revoke", nil)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("revoke status = %d", response.StatusCode)
	}
	var after store.State
	if err := json.NewDecoder(response.Body).Decode(&after); err != nil {
		t.Fatal(err)
	}
	if !after.Latest.OK || len(after.Latest.Result.Candidates) != 3 {
		t.Fatalf("revoked solve should be feasible with candidates: %#v", after.Latest)
	}

	exportResponse, err := http.Get(server.URL + "/api/export")
	if err != nil {
		t.Fatal(err)
	}
	defer exportResponse.Body.Close()
	var snapshot store.Snapshot
	if err := json.NewDecoder(exportResponse.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Version != 1 || len(snapshot.SolveRuns) < 2 {
		t.Fatalf("export snapshot incomplete: %#v", snapshot)
	}
}

func getState(t *testing.T, baseURL string) store.State {
	t.Helper()
	response, err := http.Get(baseURL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var state store.State
	if err := json.NewDecoder(response.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	return state
}
