// Package task provides the domain service for task tracker entries.
//
// Tasks support fractional-key ordering for both the main list (priority) and
// a staged "focused batch" (staged_order). The service exposes CRUD, state
// transitions, staging, drag-drop reorder via midpoint computation,
// rebalancing, dynamic filter/sort listing, and script-spawn lookups.
package task

import "time"

// State enumerates the lifecycle of a task. The values match the database
// CHECK constraint on tasks.state.
type State string

const (
	// StateNotDone marks a task that is still actionable.
	StateNotDone State = "not_done"
	// StateDone marks a completed task; CompletedAt is set on transition in.
	StateDone State = "done"
	// StateCancelled marks a cancelled task; CancelledAt is set on transition in.
	StateCancelled State = "cancelled"
)

// IsValid reports whether s is one of the recognized State constants.
func (s State) IsValid() bool {
	switch s {
	case StateNotDone, StateDone, StateCancelled:
		return true
	}
	return false
}

// ValidStates returns every recognized State value in canonical order.
func ValidStates() []State {
	return []State{StateNotDone, StateDone, StateCancelled}
}

// DueRange identifies a relative due-date filter window.
type DueRange string

const (
	// DueAny disables due-date filtering.
	DueAny DueRange = ""
	// DueOverdue matches tasks whose due_date is strictly before today.
	DueOverdue DueRange = "overdue"
	// DueToday matches tasks whose due_date equals today.
	DueToday DueRange = "today"
	// DueThisWeek matches tasks whose due_date falls within the next seven days.
	DueThisWeek DueRange = "this_week"
	// DueNone matches tasks with no due_date set.
	DueNone DueRange = "none"
)

// TagMode controls how multiple tag filters combine. The empty value behaves
// as TagModeAll to preserve backward-compatible behavior for callers that do
// not set the field.
type TagMode string

const (
	// TagModeAll requires a task to carry every supplied tag (AND semantics).
	// This is the default when TagMode is unset.
	TagModeAll TagMode = "all"
	// TagModeAny requires a task to carry at least one supplied tag (OR).
	TagModeAny TagMode = "any"
)

// IsValid reports whether m is a recognized TagMode (treating "" as valid:
// it defaults to TagModeAll).
func (m TagMode) IsValid() bool {
	switch m {
	case "", TagModeAll, TagModeAny:
		return true
	}
	return false
}

// SortAxis identifies the column the list query should order by.
type SortAxis string

const (
	// SortPriority orders by priority ascending (the canonical main-list order).
	SortPriority SortAxis = "priority"
	// SortDueDate orders by due_date (NULLs last).
	SortDueDate SortAxis = "due_date"
	// SortCreatedAt orders by created_at.
	SortCreatedAt SortAxis = "created_at"
	// SortTitle orders by LOWER(title).
	SortTitle SortAxis = "title"
)

// Task is the domain-layer representation of a row in the tasks table joined
// with its tag names.
type Task struct {
	ID                int64      `json:"id"`
	Title             string     `json:"title"`
	Notes             string     `json:"notes"`
	State             State      `json:"state"`
	DueDate           *string    `json:"due_date"`
	Priority          float64    `json:"priority"`
	StagedOrder       *float64   `json:"staged_order"`
	SpawnedByScriptID *int64     `json:"spawned_by_script_id"`
	CreatedAt         time.Time  `json:"created_at"`
	CompletedAt       *time.Time `json:"completed_at"`
	CancelledAt       *time.Time `json:"cancelled_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	Tags              []string   `json:"tags"`
}

// CreateInput carries the fields a caller must supply when creating a task.
// Tag attachment is handled separately via SetTagsByID once the caller has
// resolved name→id with the tag service.
type CreateInput struct {
	Title             string   `json:"title"`
	Notes             string   `json:"notes"`
	DueDate           *string  `json:"due_date"`
	Tags              []string `json:"tags"`
	SpawnedByScriptID *int64   `json:"spawned_by_script_id"`
}

// UpdateInput carries the mutable user-facing fields of a task. Title, notes,
// and due_date are replaced wholesale; tags is informational here and
// attachment continues to flow through SetTagsByID.
type UpdateInput struct {
	Title   string   `json:"title"`
	Notes   string   `json:"notes"`
	DueDate *string  `json:"due_date"`
	Tags    []string `json:"tags"`
}

// BulkTagOp identifies the kind of mutation requested by a bulk tag call.
// The HTTP layer parses the request "op" string via ParseBulkTagOp.
type BulkTagOp string

const (
	// BulkTagOpAdd attaches the supplied tag ids to each task. Idempotent —
	// re-adding an existing tag is a no-op courtesy of INSERT OR IGNORE.
	BulkTagOpAdd BulkTagOp = "add"
	// BulkTagOpRemove detaches the supplied tag ids from each task. Tag ids
	// not currently attached are silently ignored.
	BulkTagOpRemove BulkTagOp = "remove"
	// BulkTagOpSet replaces each task's tag set wholesale with the supplied
	// tag ids. Passing an empty slice clears all tags.
	BulkTagOpSet BulkTagOp = "set"
)

// IsValid reports whether o is one of the recognized BulkTagOp constants.
func (o BulkTagOp) IsValid() bool {
	switch o {
	case BulkTagOpAdd, BulkTagOpRemove, BulkTagOpSet:
		return true
	}
	return false
}

// ParseBulkTagOp turns a string ("add"|"remove"|"set") into a BulkTagOp.
// Unknown values return the empty BulkTagOp (which fails IsValid) so the
// caller can branch on a single validation check.
func ParseBulkTagOp(s string) BulkTagOp {
	op := BulkTagOp(s)
	if !op.IsValid() {
		return ""
	}
	return op
}

// BulkTagInput parameterises Service.BulkTag. IDs and TagIDs are pre-resolved
// by the caller (the HTTP layer turns tag names into ids); the service
// operates strictly on numeric ids so it has no dependency on the tag
// service.
type BulkTagInput struct {
	IDs    []int64
	Op     BulkTagOp
	TagIDs []int64
}

// FilterSort parameterises List. Zero values disable each filter.
type FilterSort struct {
	States        []State  `json:"states"`
	TagIDs        []int64  `json:"tag_ids"`
	TagMode       TagMode  `json:"tag_mode"`
	TagExcludeIDs []int64  `json:"tag_exclude_ids"`
	Due           DueRange `json:"due"`
	Search        string   `json:"search"`
	Sort          SortAxis `json:"sort"`
	Ascending     bool     `json:"ascending"`
	Limit         int      `json:"limit"`
	Offset        int      `json:"offset"`
}
