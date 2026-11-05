package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/srliao/tt/internal/task"
)

func (s *Server) mountDepRoutes(r chi.Router) {
	r.Route("/deps", func(r chi.Router) {
		r.Get("/", s.handleListDeps)
		r.Post("/batch", s.handleApplyDeps)
	})
}

type addDepBody struct {
	DependsOnID int64 `json:"depends_on_id"`
}

func (s *Server) handleAddDep(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r)
	if !ok {
		return
	}
	var body addDepBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "invalid JSON body", nil)
		return
	}
	if body.DependsOnID <= 0 {
		writeError(w, http.StatusBadRequest, CodeValidation, "depends_on_id is required", map[string]any{"field": "depends_on_id"})
		return
	}
	updated, err := s.tasks.AddDep(r.Context(), id, body.DependsOnID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleRemoveDep(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r)
	if !ok {
		return
	}
	depID, ok := parseIDParam(w, r, "dep_id")
	if !ok {
		return
	}
	updated, err := s.tasks.RemoveDep(r.Context(), id, depID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleApplyDeps(w http.ResponseWriter, r *http.Request) {
	var body task.DepBatch
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "invalid JSON body: each pair must be [task_id, depends_on_id]", nil)
		return
	}
	out, err := s.tasks.ApplyDeps(r.Context(), body)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleTaskGraph(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r)
	if !ok {
		return
	}
	dir := task.DirBoth
	if raw := r.URL.Query().Get("direction"); raw != "" {
		dir = task.Direction(raw)
	}
	if !dir.IsValid() {
		writeError(w, http.StatusBadRequest, CodeValidation, "direction must be up|down|both", map[string]any{"value": string(dir)})
		return
	}
	out, err := s.tasks.Graph(r.Context(), id, dir)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListDeps(w http.ResponseWriter, r *http.Request) {
	out, err := s.tasks.ListDeps(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
