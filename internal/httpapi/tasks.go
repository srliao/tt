package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/srliao/tt/internal/task"
)

// mountTaskRoutes wires the /tasks subtree.
func (s *Server) mountTaskRoutes(r chi.Router) {
	r.Route("/tasks", func(r chi.Router) {
		r.Get("/", s.handleListTasks)
		r.Post("/", s.handleCreateTask)
		r.Post("/reorder", s.handleReorderMain)
		r.Post("/bulk-tag", s.handleBulkTag)
		r.Get("/{id}", s.handleGetTask)
		r.Patch("/{id}", s.handleUpdateTask)
		r.Delete("/{id}", s.handleDeleteTask)
		r.Post("/{id}/state", s.handleSetTaskState)
		r.Post("/{id}/stage", s.handleStageTask)
		r.Delete("/{id}/stage", s.handleUnstageTask)
	})
}

// taskBody is the inbound JSON shape for POST/PATCH /tasks. Fields are
// pointers where "omitted" must be distinguishable from "zero" (notes can be
// intentionally cleared by sending "", but a missing field on PATCH should
// leave the existing value alone). For phase 06 we keep things simple and
// require the client to send the full new state on PATCH — matching the
// task.Service contract.
type taskBody struct {
	Title   string   `json:"title"`
	Notes   string   `json:"notes"`
	DueDate *string  `json:"due_date"`
	Tags    []string `json:"tags"`
}

// reorderBody is the inbound JSON shape for POST /tasks/reorder and
// POST /stage/reorder. Pointer-int neighbors so the client can express
// "drop at top" (before_id=nil) and "drop at bottom" (after_id=nil).
type reorderBody struct {
	TaskID   int64  `json:"task_id"`
	BeforeID *int64 `json:"before_id"`
	AfterID  *int64 `json:"after_id"`
}

// stateBody is the inbound JSON for POST /tasks/:id/state.
type stateBody struct {
	State string `json:"state"`
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var body taskBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "invalid JSON body", nil)
		return
	}
	if strings.TrimSpace(body.Title) == "" {
		writeError(w, http.StatusBadRequest, CodeValidation, "title is required", map[string]any{"field": "title"})
		return
	}

	created, err := s.tasks.Create(r.Context(), task.CreateInput{
		Title:   body.Title,
		Notes:   body.Notes,
		DueDate: body.DueDate,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	if len(body.Tags) > 0 {
		tagIDs, err := s.tags.Resolve(r.Context(), body.Tags, true)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		if err := s.tasks.SetTagsByID(r.Context(), created.ID, tagIDs); err != nil {
			writeServiceError(w, err)
			return
		}
		// Reload to surface the freshly-attached tag names on the response.
		reloaded, err := s.tasks.Get(r.Context(), created.ID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		created = reloaded
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	var states []task.State
	for _, raw := range q["state"] {
		st := task.State(raw)
		if !st.IsValid() {
			writeError(w, http.StatusBadRequest, CodeValidation, "invalid state filter", map[string]any{"value": raw})
			return
		}
		states = append(states, st)
	}

	tagFilter, err := s.parseTagFilter(r.Context(), q)
	if err != nil {
		// Resolve errors (unknown tag names) → 400 with the validation
		// envelope. Malformed tag_filter strings never reach this branch —
		// parseTagFilter silently degrades them to "no filter".
		writeError(w, http.StatusBadRequest, CodeValidation, err.Error(), nil)
		return
	}

	// tags_exclude is CSV (mirroring how the UI serialises it). Parsed
	// independently of tag_filter so it composes orthogonally.
	var tagExcludeIDs []int64
	if raw := q.Get("tags_exclude"); raw != "" {
		parts := strings.Split(raw, ",")
		names := parts[:0]
		for _, p := range parts {
			if t := strings.TrimSpace(p); t != "" {
				names = append(names, t)
			}
		}
		if len(names) > 0 {
			ids, err := s.tags.Resolve(r.Context(), names, false)
			if err != nil {
				writeError(w, http.StatusBadRequest, CodeValidation, err.Error(), nil)
				return
			}
			tagExcludeIDs = ids
		}
	}

	due := task.DueRange(q.Get("due"))
	switch due {
	case task.DueAny, task.DueOverdue, task.DueToday, task.DueThisWeek, task.DueNone:
		// ok
	default:
		writeError(w, http.StatusBadRequest, CodeValidation, "invalid due filter", map[string]any{"value": string(due)})
		return
	}

	sortAxis := task.SortAxis(q.Get("sort"))
	switch sortAxis {
	case "", task.SortPriority, task.SortDueDate, task.SortCreatedAt, task.SortTitle:
		if sortAxis == "" {
			sortAxis = task.SortPriority
		}
	default:
		writeError(w, http.StatusBadRequest, CodeValidation, "invalid sort axis", map[string]any{"value": string(sortAxis)})
		return
	}

	// Ascending defaults to true for non-priority sorts; for priority the
	// service ignores Ascending and always returns canonical ASC.
	ascending := true
	if raw := q.Get("asc"); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeValidation, "asc must be true|false", map[string]any{"value": raw})
			return
		}
		ascending = v
	}

	limit, err := parseIntDefault(q.Get("limit"), 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "limit must be an integer", nil)
		return
	}
	offset, err := parseIntDefault(q.Get("offset"), 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "offset must be an integer", nil)
		return
	}

	out, err := s.tasks.List(r.Context(), task.FilterSort{
		States:        states,
		Tags:          tagFilter,
		TagExcludeIDs: tagExcludeIDs,
		Due:           due,
		Search:        q.Get("q"),
		Sort:          sortAxis,
		Ascending:     ascending,
		Limit:         limit,
		Offset:        offset,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if out == nil {
		out = []task.Task{}
	}
	writeJSON(w, http.StatusOK, out)
}

// parseTagFilter parses the inbound tag-inclusion param on GET /tasks. The
// only supported form is the single `tag_filter` param ("any:work,errand" or
// "all:work,urgent", optionally with the "@untagged" sentinel).
//
// Malformed `tag_filter` strings (missing colon, unknown mode, empty list)
// degrade silently to "no tag filter" so a typo in the URL never produces a
// 400 — only an unknown but otherwise well-formed tag NAME does.
//
// The legacy `tag=` (repeated) + `tag_mode=` reader was removed in Phase 6;
// those params are now silently ignored.
func (s *Server) parseTagFilter(ctx context.Context, q url.Values) (task.TagFilter, error) {
	raw := q.Get("tag_filter")
	if raw == "" {
		return task.TagFilter{}, nil
	}
	idx := strings.IndexByte(raw, ':')
	if idx < 0 {
		return task.TagFilter{}, nil
	}
	mode := task.TagMode(raw[:idx])
	if mode != task.TagModeAny && mode != task.TagModeAll {
		return task.TagFilter{}, nil
	}
	rest := raw[idx+1:]
	if rest == "" {
		return task.TagFilter{}, nil
	}
	var includeUntagged bool
	realNames := make([]string, 0, 4)
	for _, n := range strings.Split(rest, ",") {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if n == task.UntaggedToken {
			includeUntagged = true
			continue
		}
		realNames = append(realNames, n)
	}
	if !includeUntagged && len(realNames) == 0 {
		return task.TagFilter{}, nil
	}
	var ids []int64
	if len(realNames) > 0 {
		resolved, err := s.tags.Resolve(ctx, realNames, false)
		if err != nil {
			return task.TagFilter{}, err
		}
		ids = resolved
	}
	return task.TagFilter{
		Mode:            mode,
		RealTagIDs:      ids,
		IncludeUntagged: includeUntagged,
	}, nil
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r)
	if !ok {
		return
	}
	out, err := s.tasks.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r)
	if !ok {
		return
	}
	var body taskBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "invalid JSON body", nil)
		return
	}
	if strings.TrimSpace(body.Title) == "" {
		writeError(w, http.StatusBadRequest, CodeValidation, "title is required", map[string]any{"field": "title"})
		return
	}

	if _, err := s.tasks.Update(r.Context(), id, task.UpdateInput{
		Title:   body.Title,
		Notes:   body.Notes,
		DueDate: body.DueDate,
	}); err != nil {
		writeServiceError(w, err)
		return
	}

	// Tags is always replaced wholesale on PATCH so the client can clear all
	// tags by sending an empty array. A nil Tags means the client did not
	// include the field — but JSON decoding of an absent field gives a nil
	// slice, which we cannot distinguish from "[]". For v1, always apply.
	tagIDs, err := s.tags.Resolve(r.Context(), body.Tags, true)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if err := s.tasks.SetTagsByID(r.Context(), id, tagIDs); err != nil {
		writeServiceError(w, err)
		return
	}
	reloaded, err := s.tasks.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reloaded)
}

func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r)
	if !ok {
		return
	}
	if _, err := s.tasks.Get(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	if err := s.tasks.Delete(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSetTaskState(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r)
	if !ok {
		return
	}
	var body stateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "invalid JSON body", nil)
		return
	}
	st := task.State(body.State)
	if !st.IsValid() {
		writeError(w, http.StatusBadRequest, CodeValidation, "invalid state", map[string]any{"value": body.State})
		return
	}
	if _, err := s.tasks.Get(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	updated, err := s.tasks.SetState(r.Context(), id, st)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleStageTask(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r)
	if !ok {
		return
	}
	if _, err := s.tasks.Get(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	updated, err := s.tasks.Stage(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleUnstageTask(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r)
	if !ok {
		return
	}
	if _, err := s.tasks.Get(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	updated, err := s.tasks.Unstage(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleReorderMain(w http.ResponseWriter, r *http.Request) {
	var body reorderBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "invalid JSON body", nil)
		return
	}
	if body.TaskID <= 0 {
		writeError(w, http.StatusBadRequest, CodeValidation, "task_id is required", nil)
		return
	}
	updated, err := s.tasks.ReorderMain(r.Context(), body.TaskID, body.BeforeID, body.AfterID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// bulkTagBody is the inbound JSON shape for POST /tasks/bulk-tag. Tag name
// resolution happens in the handler so the task service operates on plain
// numeric tag ids; see handleBulkTag for the validation and resolution rules.
type bulkTagBody struct {
	IDs  []int64  `json:"ids"`
	Op   string   `json:"op"`
	Tags []string `json:"tags"`
}

// handleBulkTag applies a tag mutation across many tasks in a single
// transaction. The supported ops are "add", "remove", and "set" (see
// task.BulkTagOp). Tag names are resolved into ids here so the service stays
// IO-free against the tag store; add/set autoCreate missing names while
// remove silently ignores unknowns (removing a tag a task doesn't carry
// should never be an error).
//
// Validation:
//   - ids must be non-empty
//   - op must be add|remove|set
//   - tags must be non-empty for add/remove; empty is allowed for set
//     (the explicit "clear all tags" pathway)
//
// Response is the full updated task DTOs in the request order so the React
// Query cache can be patched in place without a follow-up refetch.
func (s *Server) handleBulkTag(w http.ResponseWriter, r *http.Request) {
	var body bulkTagBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "invalid JSON body", nil)
		return
	}
	if len(body.IDs) == 0 {
		writeError(w, http.StatusBadRequest, CodeValidation, "ids is required", nil)
		return
	}
	op := task.ParseBulkTagOp(body.Op)
	if !op.IsValid() {
		writeError(w, http.StatusBadRequest, CodeValidation, "op must be add|remove|set", map[string]any{"value": body.Op})
		return
	}
	if (op == task.BulkTagOpAdd || op == task.BulkTagOpRemove) && len(body.Tags) == 0 {
		writeError(w, http.StatusBadRequest, CodeValidation, "tags is required", nil)
		return
	}

	var tagIDs []int64
	var err error
	switch op {
	case task.BulkTagOpAdd, task.BulkTagOpSet:
		tagIDs, err = s.tags.Resolve(r.Context(), body.Tags, true)
	case task.BulkTagOpRemove:
		// Remove tolerates unknown tag names — removing a tag that doesn't
		// exist anywhere is a no-op, not an error.
		tagIDs, err = s.tags.ResolveExisting(r.Context(), body.Tags)
	}
	if err != nil {
		writeServiceError(w, err)
		return
	}

	out, err := s.tasks.BulkTag(r.Context(), task.BulkTagInput{
		IDs:    body.IDs,
		Op:     op,
		TagIDs: tagIDs,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// parsePathID extracts the {id} URL param and returns (id, true) on success.
// On failure it has already written a 400 envelope and returns (0, false).
func parsePathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, CodeValidation, "invalid id", map[string]any{"value": raw})
		return 0, false
	}
	return id, true
}

// parseIntDefault parses an optional integer query param, returning fallback
// when raw is the empty string. An invalid integer (non-numeric) returns the
// error.
func parseIntDefault(raw string, fallback int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// writeServiceError maps a domain-service error to the appropriate HTTP
// response. Centralised here so every handler picks the same status for the
// same shape of failure.
//
// Recognised mappings:
//   - errors.Is(err, sql.ErrNoRows): 404 not_found.
//   - SQLite UNIQUE constraint failures (modernc driver returns the message
//     verbatim from sqlite): 409 conflict.
//   - Anything containing common validation hints (required / invalid /
//     must be / unknown): 400 validation_failed.
//   - Everything else: 500 internal.
func writeServiceError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	// sql.ErrNoRows: the task/tag/script services wrap this in fmt.Errorf
	// with %w, so errors.Is sees through.
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, CodeNotFound, "not found", nil)
		return
	}
	msg := err.Error()
	if strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "constraint failed") {
		writeError(w, http.StatusConflict, CodeConflict, "resource already exists", nil)
		return
	}
	if isValidationMessage(msg) {
		writeError(w, http.StatusBadRequest, CodeValidation, msg, nil)
		return
	}
	writeError(w, http.StatusInternalServerError, CodeInternal, "internal error", nil)
}

// isValidationMessage applies a tiny heuristic to detect validation errors
// surfaced by domain services. The services return plain errors with
// descriptive prefixes (e.g. "task: title is required"); matching by
// substring is good enough for v1.
func isValidationMessage(msg string) bool {
	for _, needle := range []string{
		"is required",
		"invalid",
		"must be",
		"unknown tags",
		"out of range",
		"not staged",
		"monthly schedule missing day",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}
