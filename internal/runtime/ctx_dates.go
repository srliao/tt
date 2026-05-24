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
// truncating to the UTC calendar day so day-math is independent of any time
// component.
func parseDateInput(s string) (time.Time, error) {
	trimmed := strings.TrimSpace(s)
	for _, layout := range acceptedDateLayouts {
		if t, err := time.Parse(layout, trimmed); err == nil {
			t = t.UTC()
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date %q (expected YYYY-MM-DD)", s)
}

// dateBindings returns the date-helper functions installed under ctx.* per
// spec §5. The supplied now lets tests inject a deterministic instant;
// production passes time.Now().
//
// Every function operates in UTC so behavior is independent of the host
// machine's local time zone. Truncation to a calendar day is done in UTC for
// the same reason: a script run at 23:30 PT and one at 00:30 PT must compute
// the same "today" against the same database.
func dateBindings(rt *goja.Runtime, now time.Time) map[string]any {
	nowUTC := now.UTC()
	today := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), 0, 0, 0, 0, time.UTC)

	return map[string]any{
		"today": func() string {
			return today.Format(dateLayout)
		},
		"weekday": func() string {
			return strings.ToLower(nowUTC.Weekday().String())
		},
		"dayOfMonth": func() int {
			return nowUTC.Day()
		},
		"month": func() int {
			return int(nowUTC.Month())
		},
		"year": func() int {
			return nowUTC.Year()
		},
		"isFirstOfMonth": func(args ...goja.Value) (bool, error) {
			t, err := optionalDateArg(args, nowUTC)
			if err != nil {
				return false, fmt.Errorf("runtime: isFirstOfMonth: %w", err)
			}
			return t.Day() == 1, nil
		},
		"isLastOfMonth": func(args ...goja.Value) (bool, error) {
			t, err := optionalDateArg(args, nowUTC)
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
			return strings.EqualFold(nowUTC.Weekday().String(), name)
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
// that defaults to "now" when omitted. Used by isFirstOfMonth /
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
