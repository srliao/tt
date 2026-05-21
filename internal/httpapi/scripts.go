package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/srliao/tt/internal/script"
)

// mountScriptRoutes wires the /scripts subtree.
func (s *Server) mountScriptRoutes(r chi.Router) {
	r.Route("/scripts", func(r chi.Router) {
		r.Get("/", s.handleListScripts)
		r.Post("/", s.handleCreateScript)
		r.Get("/{id}", s.handleGetScript)
		r.Patch("/{id}", s.handleUpdateScript)
		r.Delete("/{id}", s.handleDeleteScript)
		r.Post("/{id}/run", s.handleManualRun)
		r.Get("/{id}/runs", s.handleListRunsByScript)
		r.Get("/{id}/tasks", s.handleListTasksByScript)
	})
}

// scheduleBody is the inbound JSON shape of a script's schedule. Kept distinct
// from script.Schedule so the wire format can evolve without coupling to the
// domain type — and so we can apply HTTP-layer validation in one place rather
// than spread across the service.
type scheduleBody struct {
	Kind    string          `json:"kind"`
	Weekday string          `json:"weekday,omitempty"`
	Day     json.RawMessage `json:"day,omitempty"`
}

// scriptBody mirrors the POST/PATCH /scripts payload.
type scriptBody struct {
	Name     string       `json:"name"`
	Code     string       `json:"code"`
	Enabled  bool         `json:"enabled"`
	Schedule scheduleBody `json:"schedule"`
}

// parseSchedule converts the wire schedule into a domain script.Schedule with
// strict validation. Returns an httpError-shaped message suitable for direct
// inclusion in the validation_failed envelope.
func parseSchedule(in scheduleBody) (script.Schedule, error) {
	kind := script.Kind(strings.TrimSpace(in.Kind))
	switch kind {
	case script.KindEveryTick, script.KindDaily:
		return script.Schedule{Kind: kind}, nil
	case script.KindWeekly:
		wd := script.Weekday(strings.TrimSpace(strings.ToLower(in.Weekday)))
		valid := false
		for _, candidate := range script.ValidWeekdays() {
			if candidate == wd {
				valid = true
				break
			}
		}
		if !valid {
			return script.Schedule{}, fmt.Errorf("invalid weekday %q", in.Weekday)
		}
		return script.Schedule{Kind: kind, Weekday: wd}, nil
	case script.KindMonthly:
		var day script.MonthlyDay
		if len(in.Day) == 0 || string(in.Day) == "null" {
			return script.Schedule{}, errors.New("monthly schedule missing day")
		}
		if err := json.Unmarshal(in.Day, &day); err != nil {
			return script.Schedule{}, fmt.Errorf("invalid monthly day: %w", err)
		}
		if !day.Valid {
			return script.Schedule{}, errors.New("monthly schedule missing day")
		}
		if !day.IsLast && (day.N < 1 || day.N > 31) {
			return script.Schedule{}, fmt.Errorf("monthly day out of range: %d", day.N)
		}
		return script.Schedule{Kind: kind, Day: day}, nil
	default:
		return script.Schedule{}, fmt.Errorf("invalid schedule kind %q", in.Kind)
	}
}

func (s *Server) handleListScripts(w http.ResponseWriter, r *http.Request) {
	out, err := s.scripts.List(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if out == nil {
		out = []script.Script{}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateScript(w http.ResponseWriter, r *http.Request) {
	var body scriptBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "invalid JSON body", nil)
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, CodeValidation, "name is required", map[string]any{"field": "name"})
		return
	}
	sch, err := parseSchedule(body.Schedule)
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, err.Error(), map[string]any{"field": "schedule"})
		return
	}
	created, err := s.scripts.Create(r.Context(), script.CreateInput{
		Name:     body.Name,
		Code:     body.Code,
		Enabled:  body.Enabled,
		Schedule: sch,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleGetScript(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r)
	if !ok {
		return
	}
	sc, err := s.scripts.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sc)
}

func (s *Server) handleUpdateScript(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r)
	if !ok {
		return
	}
	var body scriptBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "invalid JSON body", nil)
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, CodeValidation, "name is required", map[string]any{"field": "name"})
		return
	}
	sch, err := parseSchedule(body.Schedule)
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, err.Error(), map[string]any{"field": "schedule"})
		return
	}
	updated, err := s.scripts.Update(r.Context(), id, script.UpdateInput{
		Name:     body.Name,
		Code:     body.Code,
		Enabled:  body.Enabled,
		Schedule: sch,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteScript(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r)
	if !ok {
		return
	}
	if _, err := s.scripts.Get(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	if err := s.scripts.Delete(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleManualRun implements POST /scripts/:id/run.
//
// Flow per spec §6:
//  1. Load the script; 404 if missing.
//  2. Reject 409 with validation_failed when the script is disabled.
//  3. Pre-create the run row via StartRun so the response can return
//     {run_id} immediately.
//  4. Hand the run to the scheduler. On ErrSchedulerBusy mark the run as
//     errored so the row never sits in 'running'.
func (s *Server) handleManualRun(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r)
	if !ok {
		return
	}
	sc, err := s.scripts.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if !sc.Enabled {
		writeError(w, http.StatusConflict, CodeValidation, "script is disabled", map[string]any{"script_id": id})
		return
	}

	run, err := s.scripts.StartRun(r.Context(), id, script.TriggerManual)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	if err := s.enqueuer.EnqueueManual(id, run.ID); err != nil {
		// Best-effort: mark the freshly created row as errored. If THIS
		// fails too the row will be picked up by RecoverOrphanedRuns on
		// next boot.
		_ = s.scripts.FinishRun(r.Context(), run.ID, script.RunStatusError, "scheduler busy", nil)
		if isSchedulerBusy(err) {
			writeError(w, http.StatusServiceUnavailable, CodeSchedulerBusy, "scheduler is busy; try again", nil)
			return
		}
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run_id": run.ID})
}

// isSchedulerBusy returns true for any error whose message indicates the
// scheduler queue is full. We intentionally match by message so the HTTP
// layer doesn't need to import the scheduler package.
func isSchedulerBusy(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrSchedulerBusy) {
		return true
	}
	return strings.Contains(err.Error(), "scheduler busy")
}

func (s *Server) handleListRunsByScript(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	limit, err := parseIntDefault(q.Get("limit"), 10)
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "limit must be an integer", nil)
		return
	}
	offset, err := parseIntDefault(q.Get("before"), 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "before must be an integer", nil)
		return
	}
	runs, err := s.scripts.ListRunsByScript(r.Context(), id, limit, offset)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if runs == nil {
		runs = []script.Run{}
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) handleListTasksByScript(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	limit, err := parseIntDefault(q.Get("limit"), 50)
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "limit must be an integer", nil)
		return
	}
	offset, err := parseIntDefault(q.Get("cursor"), 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "cursor must be an integer", nil)
		return
	}
	tasks, err := s.tasks.ByScript(r.Context(), id, limit, offset)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tasks)
}
