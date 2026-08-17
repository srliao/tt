package script

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Matches reports whether sch should run at now given the last successful
// run time (nil if the script has never run). The truth table mirrors
// spec §5:
//
//	every_tick → true
//	daily      → notRunToday(now, last)
//	weekly     → weekdayMatches(now, sch.Weekday) AND notRunToday(now, last)
//	monthly    → !sch.Day.Valid → false
//	             sch.Day.IsLast → isLastOfMonth(now) AND notRunToday(now, last)
//	             else           → now.Day() == sch.Day.N AND notRunToday(now, last)
//
// Every calendar-day question above — which weekday it is, which day of the
// month, and whether the script already ran "today" — is answered in loc, the
// configured app timezone (see config.Config.Location). Stored timestamps stay
// UTC instants; only the day boundary moves. A nil loc means UTC.
func (sch Schedule) Matches(now time.Time, lastRunAt *time.Time, loc *time.Location) bool {
	if loc == nil {
		loc = time.UTC
	}
	now = now.In(loc)

	switch sch.Kind {
	case KindEveryTick:
		return true
	case KindDaily:
		return notRunToday(now, lastRunAt, loc)
	case KindWeekly:
		return weekdayMatches(now, sch.Weekday) && notRunToday(now, lastRunAt, loc)
	case KindMonthly:
		if !sch.Day.Valid {
			return false
		}
		if sch.Day.IsLast {
			return isLastOfMonth(now) && notRunToday(now, lastRunAt, loc)
		}
		return now.Day() == sch.Day.N && notRunToday(now, lastRunAt, loc)
	}
	return false
}

// notRunToday returns true when last is nil or falls on a different calendar
// day than now when both are read in loc.
func notRunToday(now time.Time, last *time.Time, loc *time.Location) bool {
	if last == nil {
		return true
	}
	return !sameDateIn(now, *last, loc)
}

// sameDateIn compares two instants by their year/year-day in loc.
func sameDateIn(a, b time.Time, loc *time.Location) bool {
	la, lb := a.In(loc), b.In(loc)
	return la.Year() == lb.Year() && la.YearDay() == lb.YearDay()
}

// weekdayMatches case-insensitively compares now's weekday (Mon..Sun) to w.
func weekdayMatches(now time.Time, w Weekday) bool {
	return strings.EqualFold(now.Weekday().String(), string(w))
}

// isLastOfMonth reports whether adding one day to now lands in a different
// month, i.e. now is the last calendar day of its month.
func isLastOfMonth(now time.Time) bool {
	next := now.AddDate(0, 0, 1)
	return next.Month() != now.Month()
}

// ParseSchedule decodes the (kind, configJSON) pair stored on a scripts row
// into a Schedule, validating that the config matches the kind:
//
//   - every_tick / daily: no extras.
//   - weekly: {"weekday": "<one of seven>"}.
//   - monthly: {"day": 1..31} or {"day": "last"}.
func ParseSchedule(kind, configJSON string) (Schedule, error) {
	switch Kind(kind) {
	case KindEveryTick:
		return Schedule{Kind: KindEveryTick}, nil
	case KindDaily:
		return Schedule{Kind: KindDaily}, nil
	case KindWeekly:
		var cfg struct {
			Weekday string `json:"weekday"`
		}
		if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
			return Schedule{}, fmt.Errorf("script: parse weekly config: %w", err)
		}
		wd := Weekday(strings.ToLower(strings.TrimSpace(cfg.Weekday)))
		if !isValidWeekday(wd) {
			return Schedule{}, fmt.Errorf("script: invalid weekday %q", cfg.Weekday)
		}
		return Schedule{Kind: KindWeekly, Weekday: wd}, nil
	case KindMonthly:
		var cfg struct {
			Day MonthlyDay `json:"day"`
		}
		if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
			return Schedule{}, fmt.Errorf("script: parse monthly config: %w", err)
		}
		if !cfg.Day.Valid {
			return Schedule{}, fmt.Errorf("script: monthly schedule missing day")
		}
		if !cfg.Day.IsLast && (cfg.Day.N < 1 || cfg.Day.N > 31) {
			return Schedule{}, fmt.Errorf("script: monthly day out of range: %d", cfg.Day.N)
		}
		return Schedule{Kind: KindMonthly, Day: cfg.Day}, nil
	}
	return Schedule{}, fmt.Errorf("script: unknown schedule kind %q", kind)
}

// MarshalConfig produces the JSON shape that round-trips through ParseSchedule
// for the given kind:
//
//   - every_tick / daily → "{}"
//   - weekly             → {"weekday":"<wd>"}
//   - monthly            → {"day":<n or "last">}
func (sch Schedule) MarshalConfig() (string, error) {
	switch sch.Kind {
	case KindEveryTick, KindDaily:
		return "{}", nil
	case KindWeekly:
		if !isValidWeekday(sch.Weekday) {
			return "", fmt.Errorf("script: invalid weekday %q", sch.Weekday)
		}
		b, err := json.Marshal(struct {
			Weekday string `json:"weekday"`
		}{Weekday: string(sch.Weekday)})
		if err != nil {
			return "", fmt.Errorf("script: marshal weekly config: %w", err)
		}
		return string(b), nil
	case KindMonthly:
		if !sch.Day.Valid {
			return "", fmt.Errorf("script: monthly schedule missing day")
		}
		if !sch.Day.IsLast && (sch.Day.N < 1 || sch.Day.N > 31) {
			return "", fmt.Errorf("script: monthly day out of range: %d", sch.Day.N)
		}
		b, err := json.Marshal(struct {
			Day MonthlyDay `json:"day"`
		}{Day: sch.Day})
		if err != nil {
			return "", fmt.Errorf("script: marshal monthly config: %w", err)
		}
		return string(b), nil
	}
	return "", fmt.Errorf("script: unknown schedule kind %q", sch.Kind)
}

// isValidWeekday reports whether w is one of the seven recognised constants.
func isValidWeekday(w Weekday) bool {
	for _, candidate := range ValidWeekdays() {
		if candidate == w {
			return true
		}
	}
	return false
}
