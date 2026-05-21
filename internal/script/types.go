// Package script provides the domain service for user scripts: CRUD,
// schedule parsing/matching, run lifecycle (start/log/finish), user-state
// read/write, FIFO retention pruning, and startup recovery of orphaned
// "running" rows.
package script

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// Kind identifies the scheduling cadence of a script. The values match the
// CHECK constraint on scripts.schedule_kind.
type Kind string

const (
	// KindEveryTick runs the script on every scheduler tick.
	KindEveryTick Kind = "every_tick"
	// KindDaily runs the script at most once per local calendar day.
	KindDaily Kind = "daily"
	// KindWeekly runs the script at most once per week on a fixed weekday.
	KindWeekly Kind = "weekly"
	// KindMonthly runs the script at most once per month on a specific day
	// (1..31) or the literal last day of the month.
	KindMonthly Kind = "monthly"
)

// Weekday is the lowercase string form used in weekly schedule configs.
type Weekday string

// The seven recognised weekday constants. Names are lowercase to match the
// JSON schema in the spec.
const (
	Monday    Weekday = "monday"
	Tuesday   Weekday = "tuesday"
	Wednesday Weekday = "wednesday"
	Thursday  Weekday = "thursday"
	Friday    Weekday = "friday"
	Saturday  Weekday = "saturday"
	Sunday    Weekday = "sunday"
)

// ValidWeekdays returns every recognised Weekday in canonical order. The
// schedule parser uses this to validate weekly configs.
func ValidWeekdays() []Weekday {
	return []Weekday{Monday, Tuesday, Wednesday, Thursday, Friday, Saturday, Sunday}
}

// Schedule is the parsed form of a script's schedule_kind + schedule_config.
// Only the field relevant to Kind is meaningful: Weekday for KindWeekly and
// Day for KindMonthly.
type Schedule struct {
	Kind    Kind       `json:"kind"`
	Weekday Weekday    `json:"weekday,omitempty"`
	Day     MonthlyDay `json:"day,omitempty"`
}

// MonthlyDay is a tagged-union value for the "day" field of monthly schedule
// configs. JSON encodes as either an integer 1..31 or the string literal
// "last". Valid is false when the schedule did not set a day (e.g. for
// non-monthly schedules).
type MonthlyDay struct {
	N      int  `json:"-"`
	IsLast bool `json:"-"`
	Valid  bool `json:"-"`
}

// MarshalJSON emits the appropriate JSON form. An invalid MonthlyDay marshals
// to JSON null so the surrounding config can omit the field via omitempty.
func (d MonthlyDay) MarshalJSON() ([]byte, error) {
	if !d.Valid {
		return []byte("null"), nil
	}
	if d.IsLast {
		return []byte(`"last"`), nil
	}
	return []byte(strconv.Itoa(d.N)), nil
}

// UnmarshalJSON accepts either an integer or the string "last", setting Valid
// to true on any successful decode. Other JSON shapes are rejected.
func (d *MonthlyDay) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*d = MonthlyDay{}
		return nil
	}
	// Try integer first.
	var n int
	if err := json.Unmarshal(b, &n); err == nil {
		*d = MonthlyDay{N: n, IsLast: false, Valid: true}
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		if s != "last" {
			return fmt.Errorf("script: monthly day string must be \"last\", got %q", s)
		}
		*d = MonthlyDay{IsLast: true, Valid: true}
		return nil
	}
	return fmt.Errorf("script: monthly day must be int or \"last\", got %s", string(b))
}

// Script is the domain-layer representation of a row in the scripts table.
type Script struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Code      string     `json:"code"`
	Enabled   bool       `json:"enabled"`
	Schedule  Schedule   `json:"schedule"`
	LastRunAt *time.Time `json:"last_run_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// CreateInput carries the fields a caller must supply when creating a script.
type CreateInput struct {
	Name     string   `json:"name"`
	Code     string   `json:"code"`
	Enabled  bool     `json:"enabled"`
	Schedule Schedule `json:"schedule"`
}

// UpdateInput mirrors CreateInput; an update replaces every user-facing
// field of a script.
type UpdateInput = CreateInput

// Trigger identifies how a script run was initiated. Matches the CHECK
// constraint on script_runs.trigger.
type Trigger string

const (
	// TriggerScheduled marks a run kicked off by the scheduler.
	TriggerScheduled Trigger = "scheduled"
	// TriggerManual marks a run kicked off manually by the user / UI.
	TriggerManual Trigger = "manual"
)

// RunStatus is the lifecycle status of a script run. Matches the CHECK
// constraint on script_runs.status.
type RunStatus string

const (
	// RunStatusRunning is the in-flight status assigned at start.
	RunStatusRunning RunStatus = "running"
	// RunStatusOK marks a successful completion.
	RunStatusOK RunStatus = "ok"
	// RunStatusError marks a run that finished with an error.
	RunStatusError RunStatus = "error"
	// RunStatusTimeout marks a run interrupted by the runtime budget.
	RunStatusTimeout RunStatus = "timeout"
)

// Run is the domain-layer representation of a row in the script_runs table.
type Run struct {
	ID             int64      `json:"id"`
	ScriptID       int64      `json:"script_id"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	Status         RunStatus  `json:"status"`
	ErrorMessage   string     `json:"error_message"`
	SpawnedTaskIDs []int64    `json:"spawned_task_ids"`
	Trigger        Trigger    `json:"trigger"`
}

// LogLevel is the severity of a script log entry. Matches the CHECK
// constraint on script_logs.level.
type LogLevel string

const (
	// LogDebug is the most verbose level.
	LogDebug LogLevel = "debug"
	// LogInfo is the default level.
	LogInfo LogLevel = "info"
	// LogWarn signals a non-fatal anomaly.
	LogWarn LogLevel = "warn"
	// LogError signals a failure (the run may still succeed overall).
	LogError LogLevel = "error"
)

// Log is the domain-layer representation of a row in the script_logs table.
type Log struct {
	ID       int64     `json:"id"`
	RunID    int64     `json:"run_id"`
	Level    LogLevel  `json:"level"`
	Message  string    `json:"message"`
	LoggedAt time.Time `json:"logged_at"`
}
