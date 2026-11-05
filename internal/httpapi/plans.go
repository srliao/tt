package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/srliao/tt/internal/task"
)

func (s *Server) mountPlanRoutes(r chi.Router) {
	r.Post("/plans", s.handleApplyPlan)
}

func (s *Server) handleApplyPlan(w http.ResponseWriter, r *http.Request) {
	var body task.Plan
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "invalid JSON body: edge ends must be a ref string or a task id", nil)
		return
	}
	out, err := s.tasks.ApplyPlan(r.Context(), body)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
