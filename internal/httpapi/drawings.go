package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/srliao/tt/internal/drawing"
)

// maxDrawingBody caps POST /drawings and PATCH /drawings/:id payloads. Excalidraw inlines pasted
// images into the scene as data URLs, so a legitimate scene can run to many
// MB; this only guards against runaway uploads.
const maxDrawingBody = 64 << 20

// mountDrawingRoutes wires the /drawings subtree.
func (s *Server) mountDrawingRoutes(r chi.Router) {
	r.Route("/drawings", func(r chi.Router) {
		r.Get("/", s.handleListDrawings)
		r.Post("/", s.handleCreateDrawing)
		r.Get("/{id}", s.handleGetDrawing)
		r.Patch("/{id}", s.handleUpdateDrawing)
		r.Delete("/{id}", s.handleDeleteDrawing)
	})
}

// drawingCreateBody is the inbound shape for POST /drawings. Scene is
// optional; import sends it so the drawing is created with content in one
// request.
type drawingCreateBody struct {
	Name  string          `json:"name"`
	Scene json.RawMessage `json:"scene"`
}

// drawingPatchBody is the inbound shape for PATCH /drawings/:id. Both fields
// are optional; absent fields are left unchanged.
type drawingPatchBody struct {
	Name  *string         `json:"name"`
	Scene json.RawMessage `json:"scene"`
}

func (s *Server) handleListDrawings(w http.ResponseWriter, r *http.Request) {
	rows, err := s.drawings.List(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if rows == nil {
		rows = []drawing.Summary{}
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) handleCreateDrawing(w http.ResponseWriter, r *http.Request) {
	var body drawingCreateBody
	if !decodeDrawingBody(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, CodeValidation, "name is required", map[string]any{"field": "name"})
		return
	}
	created, err := s.drawings.Create(r.Context(), body.Name, body.Scene)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleGetDrawing(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r)
	if !ok {
		return
	}
	d, err := s.drawings.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleUpdateDrawing(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r)
	if !ok {
		return
	}
	var body drawingPatchBody
	if !decodeDrawingBody(w, r, &body) {
		return
	}
	if body.Name == nil && body.Scene == nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "name or scene is required", nil)
		return
	}

	var (
		out drawing.Drawing
		err error
	)
	if body.Name != nil {
		if out, err = s.drawings.Rename(r.Context(), id, *body.Name); err != nil {
			writeServiceError(w, err)
			return
		}
	}
	if body.Scene != nil {
		if out, err = s.drawings.SetScene(r.Context(), id, body.Scene); err != nil {
			writeServiceError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteDrawing(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r)
	if !ok {
		return
	}
	if err := s.drawings.Delete(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// decodeDrawingBody decodes a size-capped JSON body into dst, writing the
// error envelope (413 when over maxDrawingBody) and returning false on
// failure.
func decodeDrawingBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxDrawingBody)).Decode(dst)
	if err == nil {
		return true
	}
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		writeError(w, http.StatusRequestEntityTooLarge, CodeValidation, "drawing is too large", nil)
		return false
	}
	writeError(w, http.StatusBadRequest, CodeValidation, "invalid JSON body", nil)
	return false
}
