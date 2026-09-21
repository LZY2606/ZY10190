package web

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"

	"wellalign/internal/align"
	"wellalign/internal/store"
)

//go:embed static/*
var staticFiles embed.FS

type Server struct {
	store *store.Store
	mux   *http.ServeMux
}

func New(st *store.Store) http.Handler {
	server := &Server{store: st, mux: http.NewServeMux()}
	server.routes()
	return server.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/state", s.state)
	s.mux.HandleFunc("POST /api/solve", s.solve)
	s.mux.HandleFunc("GET /api/runs", s.runs)
	s.mux.HandleFunc("POST /api/runs/select", s.selectRun)
	s.mux.HandleFunc("PATCH /api/controls/{id}", s.updateControl)
	s.mux.HandleFunc("POST /api/controls", s.createControl)
	s.mux.HandleFunc("POST /api/controls/{id}/revoke", s.revokeControl)
	s.mux.HandleFunc("POST /api/controls/{id}/restore", s.restoreControl)
	s.mux.HandleFunc("POST /api/proposals/{id}/accept", s.acceptProposal)
	s.mux.HandleFunc("PUT /api/settings", s.updateSettings)
	s.mux.HandleFunc("POST /api/admin/reset", s.reset)
	s.mux.HandleFunc("GET /api/export", s.export)
	s.mux.HandleFunc("POST /api/admin/import", s.importSnapshot)

	sub, _ := fs.Sub(staticFiles, "static")
	s.mux.Handle("GET /", http.FileServer(http.FS(sub)))
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	state, err := s.store.State(r.Context())
	write(w, state, err)
}

func (s *Server) solve(w http.ResponseWriter, r *http.Request) {
	state, err := s.store.Solve(r.Context())
	write(w, state, err)
}

func (s *Server) runs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	runs, err := s.store.Runs(r.Context(), limit)
	write(w, runs, err)
}

func (s *Server) selectRun(w http.ResponseWriter, r *http.Request) {
	var request struct {
		RunID int64 `json:"runId"`
		Rank  int   `json:"rank"`
	}
	if err := decode(r, &request); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	err := s.store.SelectCandidate(r.Context(), request.RunID, request.Rank)
	write(w, map[string]bool{"ok": err == nil}, err)
}

func (s *Server) updateControl(w http.ResponseWriter, r *http.Request) {
	var patch store.ControlPatch
	if err := decode(r, &patch); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	state, err := s.store.UpdateControl(r.Context(), r.PathValue("id"), patch)
	write(w, state, err)
}

func (s *Server) createControl(w http.ResponseWriter, r *http.Request) {
	var control align.Control
	if err := decode(r, &control); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	state, err := s.store.CreateControl(r.Context(), control)
	write(w, state, err)
}

func (s *Server) revokeControl(w http.ResponseWriter, r *http.Request) {
	state, err := s.store.RevokeControl(r.Context(), r.PathValue("id"))
	write(w, state, err)
}

func (s *Server) restoreControl(w http.ResponseWriter, r *http.Request) {
	state, err := s.store.RestoreControl(r.Context(), r.PathValue("id"))
	write(w, state, err)
}

func (s *Server) acceptProposal(w http.ResponseWriter, r *http.Request) {
	state, err := s.store.AcceptProposal(r.Context(), r.PathValue("id"))
	write(w, state, err)
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var settings align.Settings
	if err := decode(r, &settings); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	state, err := s.store.UpdateSettings(r.Context(), settings)
	write(w, state, err)
}

func (s *Server) reset(w http.ResponseWriter, r *http.Request) {
	err := s.store.ResetFixture(r.Context())
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	state, err := s.store.State(r.Context())
	write(w, state, err)
}

func (s *Server) export(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.store.Export(r.Context())
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=wellalign-snapshot.json")
	write(w, snapshot, nil)
}

func (s *Server) importSnapshot(w http.ResponseWriter, r *http.Request) {
	var snapshot store.Snapshot
	if err := decode(r, &snapshot); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	if err := s.store.Import(r.Context(), snapshot); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	state, err := s.store.State(r.Context())
	write(w, state, err)
}

func decode(r *http.Request, target any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(target)
}

func write(w http.ResponseWriter, value any, err error) {
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
