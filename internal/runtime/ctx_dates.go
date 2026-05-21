package runtime

import (
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// dateLayout is the YYYY-MM-DD string form used at every ctx-API boundary that
// accepts or emits a date.
const dateLayout = "2006-01-02"

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
		"isFirstOfMonth": func() bool {
			return nowUTC.Day() == 1
		},
		"isLastOfMonth": func() bool {
			// The last day of the current month equals the day before the
			// first of the next month.
			firstNext := time.Date(nowUTC.Year(), nowUTC.Month()+1, 1, 0, 0, 0, 0, time.UTC)
			last := firstNext.AddDate(0, 0, -1)
			return nowUTC.Day() == last.Day()
		},
		"isWeekday": func(name string) bool {
			return strings.EqualFold(nowUTC.Weekday().String(), name)
		},
		"daysSince": func(s string) (int, error) {
			t, err := time.Parse(dateLayout, strings.TrimSpace(s))
			if err != nil {
				return 0, fmt.Errorf("runtime: daysSince: invalid date %q: %w", s, err)
			}
			d := today.Sub(t.UTC()) / (24 * time.Hour)
			return int(d), nil
		},
		"daysBetween": func(a, b string) (int, error) {
			ta, err := time.Parse(dateLayout, strings.TrimSpace(a))
			if err != nil {
				return 0, fmt.Errorf("runtime: daysBetween: invalid date %q: %w", a, err)
			}
			tb, err := time.Parse(dateLayout, strings.TrimSpace(b))
			if err != nil {
				return 0, fmt.Errorf("runtime: daysBetween: invalid date %q: %w", b, err)
			}
			d := tb.UTC().Sub(ta.UTC()) / (24 * time.Hour)
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
			t, err := time.Parse(dateLayout, strings.TrimSpace(s))
			if err != nil {
				return nil, fmt.Errorf("runtime: parseDate: invalid date %q: %w", s, err)
			}
			return timeToJSDate(rt, t.UTC())
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

// jsValueToTime converts a value produced by ctx.parseDate / addDays (a JS
// Date object or a YYYY-MM-DD string) back into a Go time.Time. Accepting
// both lets the bindings compose naturally — formatDate(addDays(...)) — even
// though goja's JS Date round-trips through Export as a time.Time.
func jsValueToTime(v goja.Value) (time.Time, error) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return time.Time{}, fmt.Errorf("date value is null/undefined")
	}
	exp := v.Export()
	switch x := exp.(type) {
	case time.Time:
		return x.UTC(), nil
	case string:
		t, err := time.Parse(dateLayout, strings.TrimSpace(x))
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid date string %q: %w", x, err)
		}
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("expected JS Date or YYYY-MM-DD string, got %T", exp)
}
