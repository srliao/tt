package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// mountStageRoutes wires the /stage subtree. POST /stage/reorder targets the
// staged_order axis; DELETE /stage clears every staged task; DELETE
// /stage/finished only removes done/cancelled tasks from the focused batch.
func (s *Server) mountStageRoutes(r chi.Router) {
	r.Route("/stage", func(r chi.Router) {
		r.Post("/reorder", s.handleReorderStage)
		r.Delete("/", s.handleClearStage)
		r.Delete("/finished", s.handleClearFinishedStage)
	})
}

func (s *Server) handleReorderStage(w http.ResponseWriter, r *http.Request) {
	var body reorderBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "invalid JSON body", nil)
		return
	}
	if body.TaskID <= 0 {
		writeError(w, http.StatusBadRequest, CodeValidation, "task_id is required", nil)
		return
	}
	updated, err := s.tasks.ReorderStage(r.Context(), body.TaskID, body.BeforeID, body.AfterID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleClearStage(w http.ResponseWriter, r *http.Request) {
	if err := s.tasks.ClearStage(r.Context()); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleClearFinishedStage(w http.ResponseWriter, r *http.Request) {
	if err := s.tasks.ClearFinishedFromStage(r.Context()); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
