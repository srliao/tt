package runtime

import (
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// dateLayout is the YYYY-MM-DD string form emitted by every ctx-API helper.
const dateLayout = "2006-01-02"

// acceptedDateLayouts are the formats ctx.* accepts on input. We always emit
// dateLayout but accept the SQLite timestamp shape (which is what task fields
// like completed_at carry) and RFC3339 so scripts can pass through ctx field
// values without slicing the time portion off first.
var acceptedDateLayouts = []string{
	dateLayout,
	"2006-01-02 15:04:05",
	time.RFC3339,
}

// parseDateInput parses a user-supplied date in any of acceptedDateLayouts,
// truncating to a calendar day anchored at UTC midnight. See civilDate for
// why every date value in this file shares that anchoring.
func parseDateInput(s string) (time.Time, error) {
	trimmed := strings.TrimSpace(s)
	for _, layout := range acceptedDateLayouts {
		if t, err := time.Parse(layout, trimmed); err == nil {
			return civilDate(t.UTC()), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date %q (expected YYYY-MM-DD)", s)
}

// civilDate strips t down to its year/month/day and re-anchors it at UTC
// midnight. Every date *value* in the ctx API — parseDate results, addDays
// results, today — uses this single representation, which is what makes
// day-difference arithmetic (daysSince, daysBetween) plain subtraction: with
// both operands on the same anchor there is no zone offset left to leak into
// the division and truncate a day away.
//
// Note this is deliberately not "the instant of local midnight". t is read in
// whatever zone it already carries; callers convert to the app zone first.
func civilDate(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// dateBindings returns the date-helper functions installed under ctx.* per
// spec §5. The supplied now lets tests inject a deterministic instant;
// production passes time.Now().
//
// loc is the configured app timezone (config.Config.Location) and decides
// when the calendar day rolls over — so a script run at 23:30 and one at
// 00:30 the next morning disagree about "today" exactly when the user
// expects them to. A nil loc means UTC.
//
// The zone affects which day it is, never how a date value is represented:
// results stay anchored at UTC midnight per civilDate, so scripts can keep
// round-tripping them through parseDate/addDays/formatDate unchanged.
func dateBindings(rt *goja.Runtime, now time.Time, loc *time.Location) map[string]any {
	nowLocal := now.In(ctxLocation(loc))
	today := civilDate(nowLocal)

	return map[string]any{
		"today": func() string {
			return today.Format(dateLayout)
		},
		"weekday": func() string {
			return strings.ToLower(nowLocal.Weekday().String())
		},
		"dayOfMonth": func() int {
			return nowLocal.Day()
		},
		"month": func() int {
			return int(nowLocal.Month())
		},
		"year": func() int {
			return nowLocal.Year()
		},
		"isFirstOfMonth": func(args ...goja.Value) (bool, error) {
			t, err := optionalDateArg(args, today)
			if err != nil {
				return false, fmt.Errorf("runtime: isFirstOfMonth: %w", err)
			}
			return t.Day() == 1, nil
		},
		"isLastOfMonth": func(args ...goja.Value) (bool, error) {
			t, err := optionalDateArg(args, today)
			if err != nil {
				return false, fmt.Errorf("runtime: isLastOfMonth: %w", err)
			}
			// The last day of t's month equals the day before the first of
			// the next month.
			firstNext := time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.UTC)
			last := firstNext.AddDate(0, 0, -1)
			return t.Day() == last.Day(), nil
		},
		"isWeekday": func(name string) bool {
			return strings.EqualFold(nowLocal.Weekday().String(), name)
		},
		"daysSince": func(s string) (int, error) {
			t, err := parseDateInput(s)
			if err != nil {
				return 0, fmt.Errorf("runtime: daysSince: %w", err)
			}
			d := today.Sub(t) / (24 * time.Hour)
			return int(d), nil
		},
		"daysBetween": func(a, b string) (int, error) {
			ta, err := parseDateInput(a)
			if err != nil {
				return 0, fmt.Errorf("runtime: daysBetween: %w", err)
			}
			tb, err := parseDateInput(b)
			if err != nil {
				return 0, fmt.Errorf("runtime: daysBetween: %w", err)
			}
			d := tb.Sub(ta) / (24 * time.Hour)
			return int(d), nil
		},
		"addDays": func(d goja.Value, n int) (goja.Value, error) {
			t, err := jsValueToTime(d)
			if err != nil {
				return nil, fmt.Errorf("runtime: addDays: %w", err)
			}
			return timeToJSDate(rt, t.AddDate(0, 0, n))
		},
		"formatDate": func(d goja.Value) (string, error) {
			t, err := jsValueToTime(d)
			if err != nil {
				return "", fmt.Errorf("runtime: formatDate: %w", err)
			}
			return t.UTC().Format(dateLayout), nil
		},
		"parseDate": func(s string) (goja.Value, error) {
			t, err := parseDateInput(s)
			if err != nil {
				return nil, fmt.Errorf("runtime: parseDate: %w", err)
			}
			return timeToJSDate(rt, t)
		},
	}
}

// timeToJSDate constructs a JS Date from a Go time.Time by invoking the JS
// Date constructor with the unix-millisecond representation. Using the JS
// constructor (rather than relying on goja's auto-wrap of time.Time) ensures
// the returned value exposes the full Date.prototype — getUTCDate, toISOString,
// and so on — to user scripts.
func timeToJSDate(rt *goja.Runtime, t time.Time) (goja.Value, error) {
	dateCtor, ok := goja.AssertConstructor(rt.Get("Date"))
	if !ok {
		return nil, fmt.Errorf("runtime: Date constructor unavailable")
	}
	ms := t.UnixMilli()
	v, err := dateCtor(nil, rt.ToValue(ms))
	if err != nil {
		return nil, fmt.Errorf("runtime: construct Date: %w", err)
	}
	return v, nil
}

// optionalDateArg resolves the variadic first argument of a ctx.* helper
// that defaults to today (in the configured zone) when omitted. Used by
// isFirstOfMonth /
// isLastOfMonth so both forms — bare query and arg form — share parsing
// with the rest of the date API (JS Date, RFC3339, both SQLite layouts,
// YYYY-MM-DD).
func optionalDateArg(args []goja.Value, fallback time.Time) (time.Time, error) {
	if len(args) == 0 || args[0] == nil || goja.IsUndefined(args[0]) || goja.IsNull(args[0]) {
		return fallback, nil
	}
	return jsValueToTime(args[0])
}

// jsValueToTime converts a value produced by ctx.parseDate / addDays (a JS
// Date object or a date string) back into a Go time.Time. Accepting both
// lets the bindings compose naturally — formatDate(addDays(...)) — even
// though goja's JS Date round-trips through Export as a time.Time. Strings
// flow through parseDateInput so SQLite timestamp values (completed_at,
// created_at, etc.) work without manual slicing.
func jsValueToTime(v goja.Value) (time.Time, error) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return time.Time{}, fmt.Errorf("date value is null/undefined")
	}
	exp := v.Export()
	switch x := exp.(type) {
	case time.Time:
		return x.UTC(), nil
	case string:
		return parseDateInput(x)
	}
	return time.Time{}, fmt.Errorf("expected JS Date or date string, got %T", exp)
}
