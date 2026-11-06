package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/srliao/tt/internal/task"
)

func (s *Server) mountProjectRoutes(r chi.Router) {
	r.Route("/projects", func(r chi.Router) {
		r.Get("/", s.handleListProjects)
		r.Post("/rename", s.handleRenameProject)
	})
}

type setProjectBody struct {
	IDs     []int64 `json:"ids"`
	Project *string `json:"project"`
}

type renameProjectBody struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func (s *Server) handleSetProject(w http.ResponseWriter, r *http.Request) {
	var body setProjectBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "invalid JSON body", nil)
		return
	}
	out, err := s.tasks.SetProject(r.Context(), body.IDs, body.Project)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	out, err := s.tasks.ListProjects(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleRenameProject(w http.ResponseWriter, r *http.Request) {
	var body renameProjectBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "invalid JSON body", nil)
		return
	}
	out, err := s.tasks.RenameProject(r.Context(), body.From, body.To)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func parseProjectFilter(raw []string) (task.ProjectFilter, error) {
	var f task.ProjectFilter
	for _, v := range raw {
		for n := range strings.SplitSeq(v, ",") {
			if strings.EqualFold(strings.TrimSpace(n), task.NoProjectToken) {
				f.IncludeNone = true
				continue
			}
			name, err := task.NormalizeProject(&n)
			if err != nil {
				return task.ProjectFilter{}, err
			}
			if name != nil {
				f.Names = append(f.Names, *name)
			}
		}
	}
	return f, nil
}
