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

// FilterSort parameterises List. Zero values disable each filter.
type FilterSort struct {
	States    []State  `json:"states"`
	TagIDs    []int64  `json:"tag_ids"`
	Due       DueRange `json:"due"`
	Search    string   `json:"search"`
	Sort      SortAxis `json:"sort"`
	Ascending bool     `json:"ascending"`
	Limit     int      `json:"limit"`
	Offset    int      `json:"offset"`
}
