package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/srliao/tt/internal/tag"
)

// mountTagRoutes wires the /tags subtree.
func (s *Server) mountTagRoutes(r chi.Router) {
	r.Route("/tags", func(r chi.Router) {
		r.Get("/", s.handleListTags)
		r.Post("/", s.handleCreateTag)
		r.Patch("/{id}", s.handleRenameTag)
		r.Delete("/{id}", s.handleDeleteTag)
	})
}

// tagBody is the inbound JSON shape for POST /tags and PATCH /tags/:id.
type tagBody struct {
	Name string `json:"name"`
}

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	// ?counts=1 switches to the heavier listing that aggregates task counts.
	// We keep the default cheap (no join) so callers that only need names —
	// the resolver, the legacy filter chips — stay fast.
	if r.URL.Query().Get("counts") == "1" {
		rows, err := s.tags.ListWithCounts(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		if rows == nil {
			rows = []tag.TagWithCount{}
		}
		writeJSON(w, http.StatusOK, rows)
		return
	}

	tags, err := s.tags.List(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if tags == nil {
		tags = []tag.Tag{}
	}
	writeJSON(w, http.StatusOK, tags)
}

func (s *Server) handleCreateTag(w http.ResponseWriter, r *http.Request) {
	var body tagBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "invalid JSON body", nil)
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, CodeValidation, "name is required", map[string]any{"field": "name"})
		return
	}
	created, err := s.tags.Create(r.Context(), body.Name)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleRenameTag(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r)
	if !ok {
		return
	}
	var body tagBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "invalid JSON body", nil)
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, CodeValidation, "name is required", map[string]any{"field": "name"})
		return
	}
	renamed, err := s.tags.Rename(r.Context(), id, body.Name)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, renamed)
}

func (s *Server) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r)
	if !ok {
		return
	}
	if err := s.tags.Delete(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
