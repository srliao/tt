package db

import "time"

// TimeLayout is the layout of every timestamp column: what SQLite's
// datetime('now') produces (UTC, second precision).
const TimeLayout = "2006-01-02 15:04:05"

// Clock is the time source services stamp rows with. Simulations pass a fake
// one so the rows they write follow it.
type Clock func() time.Time

// Stamp is the clock's current time in TimeLayout.
func (c Clock) Stamp() string {
	return FormatTime(c())
}

// FormatTime renders t in UTC in TimeLayout.
func FormatTime(t time.Time) string {
	return t.UTC().Format(TimeLayout)
}

// ParseTime reads a timestamp column. It accepts TimeLayout and, for rows
// written by other code paths, RFC3339; anything else is the zero time.
func ParseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(TimeLayout, s); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC()
	}
	return time.Time{}
}
