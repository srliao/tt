package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/srliao/tt/internal/script"
	"github.com/srliao/tt/internal/task"
)

// mountRunRoutes wires the /runs subtree.
func (s *Server) mountRunRoutes(r chi.Router) {
	r.Route("/runs", func(r chi.Router) {
		r.Get("/", s.handleListRuns)
		r.Get("/{id}", s.handleGetRun)
	})
}

// runListItem is the on-the-wire shape for entries in GET /runs. We reuse
// script.Run directly to avoid duplicating fields; the per-handler decoration
// (e.g. spawned task summaries) is reserved for the detail endpoint.
type spawnedTaskSummary struct {
	ID    int64      `json:"id"`
	Title string     `json:"title"`
	State task.State `json:"state"`
}

// runDetail is the response shape for GET /runs/:id. Logs and spawned-task
// summaries are attached so the UI can render a run page without a second
// round-trip per related resource.
type runDetail struct {
	script.Run
	Logs          []script.Log         `json:"logs"`
	SpawnedTasks  []spawnedTaskSummary `json:"spawned_tasks"`
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// Filter parsing. We accept the same shape spec §6 describes; filtering
	// happens in-memory off ListAllRuns / ListRunsByScript output. For v1
	// this is fine — the dataset is small (max ~500 rows by retention).
	var (
		scriptID    int64
		hasScriptID bool
	)
	if raw := q.Get("script_id"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v <= 0 {
			writeError(w, http.StatusBadRequest, CodeValidation, "script_id must be a positive integer", nil)
			return
		}
		scriptID = v
		hasScriptID = true
	}

	statusFilter := q.Get("status")
	if statusFilter != "" {
		switch script.RunStatus(statusFilter) {
		case script.RunStatusRunning, script.RunStatusOK, script.RunStatusError, script.RunStatusTimeout:
			// ok
		default:
			writeError(w, http.StatusBadRequest, CodeValidation, "invalid status filter", map[string]any{"value": statusFilter})
			return
		}
	}

	from, err := parseRFC3339Optional(q.Get("from"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "from must be RFC3339", nil)
		return
	}
	to, err := parseRFC3339Optional(q.Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "to must be RFC3339", nil)
		return
	}

	limit, err := parseIntDefault(q.Get("limit"), 25)
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "limit must be an integer", nil)
		return
	}
	offset, err := parseIntDefault(q.Get("cursor"), 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeValidation, "cursor must be an integer", nil)
		return
	}

	// Fetch a generous slice from the service then filter in Go. We pass
	// limit*4 + offset so the filtered subset can still hit `limit` entries
	// when most rows are filtered out; capping at 500 protects against
	// pathological filters.
	fetchSize := limit*4 + offset
	if fetchSize < 50 {
		fetchSize = 50
	}
	if fetchSize > 500 {
		fetchSize = 500
	}

	var runs []script.Run
	if hasScriptID {
		runs, err = s.scripts.ListRunsByScript(r.Context(), scriptID, fetchSize, 0)
	} else {
		runs, err = s.scripts.ListAllRuns(r.Context(), fetchSize, 0)
	}
	if err != nil {
		writeServiceError(w, err)
		return
	}

	filtered := make([]script.Run, 0, len(runs))
	for _, run := range runs {
		if statusFilter != "" && string(run.Status) != statusFilter {
			continue
		}
		if from != nil && run.StartedAt.Before(*from) {
			continue
		}
		if to != nil && run.StartedAt.After(*to) {
			continue
		}
		filtered = append(filtered, run)
	}

	if offset > len(filtered) {
		filtered = filtered[:0]
	} else {
		filtered = filtered[offset:]
	}
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	writeJSON(w, http.StatusOK, filtered)
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r)
	if !ok {
		return
	}
	run, err := s.scripts.GetRun(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	logs, err := s.scripts.GetLogs(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if logs == nil {
		logs = []script.Log{}
	}

	// Spawned tasks are best-effort: a deleted task should not break the run
	// detail page. Skip missing ids instead of erroring out.
	summaries := make([]spawnedTaskSummary, 0, len(run.SpawnedTaskIDs))
	for _, tid := range run.SpawnedTaskIDs {
		t, err := s.tasks.Get(r.Context(), tid)
		if err != nil {
			if isNotFound(err) {
				continue
			}
			writeServiceError(w, err)
			return
		}
		summaries = append(summaries, spawnedTaskSummary{
			ID:    t.ID,
			Title: t.Title,
			State: t.State,
		})
	}

	writeJSON(w, http.StatusOK, runDetail{
		Run:          run,
		Logs:         logs,
		SpawnedTasks: summaries,
	})
}

// parseRFC3339Optional parses raw as RFC3339 when non-empty. Empty input
// returns (nil, nil) so callers can express "no bound".
func parseRFC3339Optional(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// isNotFound recognises a sql.ErrNoRows propagated up through the service
// layer (which wraps with %w). Used by the run-detail handler to drop
// references to deleted tasks silently rather than failing the whole page.
func isNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
