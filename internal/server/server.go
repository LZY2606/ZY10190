package server

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"

	"depthalign/internal/align"
	"depthalign/internal/domain"
	"depthalign/internal/store"
)

//go:embed static/*
var staticFiles embed.FS

type Server struct {
	store *store.Store
	mux   *http.ServeMux
}

func New(st *store.Store) http.Handler {
	s := &Server{store: st, mux: http.NewServeMux()}
	s.routes()
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /", s.index)
	sub, _ := fs.Sub(staticFiles, "static")
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))
	s.mux.HandleFunc("GET /api/state", s.state)
	s.mux.HandleFunc("POST /api/markers", s.upsertMarker)
	s.mux.HandleFunc("POST /api/markers/toggle", s.toggleMarker)
	s.mux.HandleFunc("POST /api/solve", s.solve)
	s.mux.HandleFunc("GET /api/runs", s.runs)
	s.mux.HandleFunc("POST /api/reset", s.reset)
	s.mux.HandleFunc("POST /api/replay", s.replay)
	s.mux.HandleFunc("GET /api/export", s.exportBundle)
	s.mux.HandleFunc("POST /api/import", s.importBundle)
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.ServeFileFS(w, r, staticFiles, "static/index.html")
}

type stateResponse struct {
	Dataset domain.Dataset     `json:"dataset"`
	Latest  *store.RunRecord   `json:"latest"`
	Config  domain.SolveConfig `json:"config"`
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.LoadDataset(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	runs, err := s.store.ListRuns(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	resp := stateResponse{Dataset: data, Config: align.DefaultConfig()}
	if len(runs) > 0 {
		resp.Latest = &runs[0]
	}
	writeJSON(w, resp)
}

type markerRequest struct {
	ID     string  `json:"id"`
	Label  string  `json:"label"`
	AFrom  float64 `json:"a_from"`
	BTo    float64 `json:"b_to"`
	Kind   string  `json:"kind"`
	Origin string  `json:"origin"`
	Active *bool   `json:"active"`
}

func (s *Server) upsertMarker(w http.ResponseWriter, r *http.Request) {
	var req markerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	data, err := s.store.LoadDataset(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	kind := req.Kind
	if kind != "hard" {
		kind = "soft"
	}
	origin := req.Origin
	if origin == "" {
		origin = "manual"
	}
	index := -1
	for i := range data.Markers {
		if data.Markers[i].ID == req.ID {
			index = i
		}
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	marker := domain.Marker{ID: req.ID, Label: req.Label, PassAFrom: req.AFrom, PassBTo: req.BTo, Kind: kind, Origin: origin, Active: active}
	if marker.ID == "" {
		writeError(w, http.StatusBadRequest, errMarkerID)
		return
	}
	if index >= 0 {
		marker.CreatedAt = data.Markers[index].CreatedAt
		data.Markers[index] = marker
	} else {
		data.Markers = append(data.Markers, marker)
	}
	op := store.Operation{Kind: "marker_upsert", MarkerID: marker.ID, Payload: map[string]interface{}{"a_from": req.AFrom, "b_to": req.BTo, "kind": kind, "origin": origin, "active": active, "label": marker.Label}}
	if err := s.store.SaveDataset(r.Context(), data, op); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) toggleMarker(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID     string `json:"id"`
		Active *bool  `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		writeError(w, http.StatusBadRequest, errMarkerID)
		return
	}
	data, err := s.store.LoadDataset(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	active := true
	for i := range data.Markers {
		if data.Markers[i].ID == req.ID {
			if req.Active != nil {
				active = *req.Active
			} else {
				active = !data.Markers[i].Active
			}
			data.Markers[i].Active = active
			op := store.Operation{Kind: "marker_toggle", MarkerID: req.ID, Payload: map[string]interface{}{"active": active}}
			if err := s.store.SaveDataset(r.Context(), data, op); err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, map[string]string{"status": "ok"})
			return
		}
	}
	writeError(w, http.StatusNotFound, errMarkerNotFound)
}

func (s *Server) solve(w http.ResponseWriter, r *http.Request) {
	var cfg domain.SolveConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if cfg.MinStretch <= 0 {
		cfg = align.DefaultConfig()
	}
	data, err := s.store.LoadDataset(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	result := align.Solve(data, cfg)
	run, err := s.store.SaveRun(r.Context(), cfg, result)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, run)
}

func (s *Server) runs(w http.ResponseWriter, r *http.Request) {
	runs, err := s.store.ListRuns(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, runs)
}

func (s *Server) reset(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Reset(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]string{"status": "reset_to_fixture"})
}

func (s *Server) exportBundle(w http.ResponseWriter, r *http.Request) {
	bundle, err := s.store.Export(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=depth-align-runs.json")
	writeJSON(w, bundle)
}

func (s *Server) importBundle(w http.ResponseWriter, r *http.Request) {
	var bundle store.Bundle
	if err := json.NewDecoder(r.Body).Decode(&bundle); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if bundle.Version != 1 || len(bundle.Dataset.Curves) == 0 {
		writeError(w, http.StatusBadRequest, errBundleInvalid)
		return
	}
	if err := s.store.Import(r.Context(), bundle); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]string{"status": "imported"})
}

func (s *Server) replay(w http.ResponseWriter, r *http.Request) {
	bundle, err := s.store.Export(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.store.Reset(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	applied := 0
	for _, op := range bundle.Operations {
		if op.Kind == "reset_fixture" || op.Kind == "bundle_import" {
			continue
		}
		data, err := s.store.LoadDataset(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if !applyOperation(&data, op) {
			continue
		}
		if err := s.store.SaveDataset(r.Context(), data, op); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		applied++
	}
	for i := len(bundle.Runs) - 1; i >= 0; i-- {
		data, err := s.store.LoadDataset(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		result := align.Solve(data, bundle.Runs[i].Result.Config)
		if _, err := s.store.SaveRun(r.Context(), bundle.Runs[i].Result.Config, result); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, map[string]string{"status": "replayed", "operations": strconv.Itoa(applied), "runs": strconv.Itoa(len(bundle.Runs))})
}

func applyOperation(data *domain.Dataset, op store.Operation) bool {
	switch op.Kind {
	case "marker_upsert":
		marker := domain.Marker{ID: op.MarkerID, Active: true}
		if v, ok := op.Payload["label"].(string); ok {
			marker.Label = v
		}
		marker.PassAFrom = payloadFloat(op.Payload, "a_from")
		marker.PassBTo = payloadFloat(op.Payload, "b_to")
		if v, ok := op.Payload["kind"].(string); ok {
			marker.Kind = v
		}
		if v, ok := op.Payload["origin"].(string); ok {
			marker.Origin = v
		}
		if v, ok := op.Payload["active"].(bool); ok {
			marker.Active = v
		}
		if marker.Kind == "" {
			marker.Kind = "hard"
		}
		if marker.Origin == "" {
			marker.Origin = "manual"
		}
		replaceMarker(data, marker)
		return true
	case "marker_toggle":
		for i := range data.Markers {
			if data.Markers[i].ID == op.MarkerID {
				if v, ok := op.Payload["active"].(bool); ok {
					data.Markers[i].Active = v
				}
				return true
			}
		}
	}
	return false
}

func replaceMarker(data *domain.Dataset, marker domain.Marker) {
	for i := range data.Markers {
		if data.Markers[i].ID == marker.ID {
			data.Markers[i] = marker
			return
		}
	}
	data.Markers = append(data.Markers, marker)
}

func payloadFloat(payload map[string]interface{}, key string) float64 {
	switch v := payload[key].(type) {
	case float64:
		return v
	case json.Number:
		f, _ := v.Float64()
		return f
	default:
		return 0
	}
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

var (
	errMarkerID       = validationError("marker id is required")
	errMarkerNotFound = validationError("marker not found")
	errBundleInvalid  = validationError("bundle version or dataset is invalid")
)

type validationError string

func (e validationError) Error() string { return string(e) }
