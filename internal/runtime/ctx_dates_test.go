package runtime

import (
	"testing"
	"time"

	"github.com/dop251/goja"
)

// fixedNow is the deterministic "now" used by the date binding tests: a
// Thursday at 14:30 UTC, day 21 of May 2026.
var fixedNow = time.Date(2026, 5, 21, 14, 30, 0, 0, time.UTC)

// newRuntimeWithDates installs ctx with date bindings only — enough to run a
// single JS expression against the date helpers.
func newRuntimeWithDates(t *testing.T, now time.Time) *goja.Runtime {
	t.Helper()
	rt := goja.New()
	ctxObj := rt.NewObject()
	if err := rt.Set("ctx", ctxObj); err != nil {
		t.Fatalf("set ctx: %v", err)
	}
	for name, fn := range dateBindings(rt, now) {
		if err := ctxObj.Set(name, fn); err != nil {
			t.Fatalf("set ctx.%s: %v", name, err)
		}
	}
	return rt
}

func runJS(t *testing.T, rt *goja.Runtime, src string) goja.Value {
	t.Helper()
	v, err := rt.RunString(src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return v
}

func TestDateBindings_Today(t *testing.T) {
	rt := newRuntimeWithDates(t, fixedNow)
	got := runJS(t, rt, `ctx.today()`).String()
	if want := "2026-05-21"; got != want {
		t.Fatalf("ctx.today() = %q, want %q", got, want)
	}
}

func TestDateBindings_Weekday(t *testing.T) {
	rt := newRuntimeWithDates(t, fixedNow)
	got := runJS(t, rt, `ctx.weekday()`).String()
	if want := "thursday"; got != want {
		t.Fatalf("ctx.weekday() = %q, want %q", got, want)
	}
}

func TestDateBindings_DayOfMonth(t *testing.T) {
	rt := newRuntimeWithDates(t, fixedNow)
	got := runJS(t, rt, `ctx.dayOfMonth()`).ToInteger()
	if got != 21 {
		t.Fatalf("ctx.dayOfMonth() = %d, want 21", got)
	}
}

func TestDateBindings_Month(t *testing.T) {
	rt := newRuntimeWithDates(t, fixedNow)
	got := runJS(t, rt, `ctx.month()`).ToInteger()
	if got != 5 {
		t.Fatalf("ctx.month() = %d, want 5", got)
	}
}

func TestDateBindings_Year(t *testing.T) {
	rt := newRuntimeWithDates(t, fixedNow)
	got := runJS(t, rt, `ctx.year()`).ToInteger()
	if got != 2026 {
		t.Fatalf("ctx.year() = %d, want 2026", got)
	}
}

func TestDateBindings_IsFirstOfMonth(t *testing.T) {
	rt := newRuntimeWithDates(t, fixedNow)
	if v := runJS(t, rt, `ctx.isFirstOfMonth()`).ToBoolean(); v {
		t.Fatalf("ctx.isFirstOfMonth() on 2026-05-21 = true, want false")
	}
	rtFirst := newRuntimeWithDates(t, time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC))
	if v := runJS(t, rtFirst, `ctx.isFirstOfMonth()`).ToBoolean(); !v {
		t.Fatalf("ctx.isFirstOfMonth() on 2026-06-01 = false, want true")
	}
}

func TestDateBindings_IsLastOfMonth(t *testing.T) {
	rt := newRuntimeWithDates(t, fixedNow)
	if v := runJS(t, rt, `ctx.isLastOfMonth()`).ToBoolean(); v {
		t.Fatalf("ctx.isLastOfMonth() on 2026-05-21 = true, want false")
	}
	rtLast := newRuntimeWithDates(t, time.Date(2026, 5, 31, 9, 0, 0, 0, time.UTC))
	if v := runJS(t, rtLast, `ctx.isLastOfMonth()`).ToBoolean(); !v {
		t.Fatalf("ctx.isLastOfMonth() on 2026-05-31 = false, want true")
	}
}

func TestDateBindings_IsFirstOfMonth_WithArg(t *testing.T) {
	rt := newRuntimeWithDates(t, fixedNow)
	cases := []struct {
		expr string
		want bool
	}{
		{`ctx.isFirstOfMonth("2026-06-01")`, true},
		{`ctx.isFirstOfMonth("2026-06-02")`, false},
		// SQLite timestamp form
		{`ctx.isFirstOfMonth("2026-06-01 09:30:00")`, true},
		// RFC3339
		{`ctx.isFirstOfMonth("2026-06-01T09:30:00Z")`, true},
		// JS Date
		{`ctx.isFirstOfMonth(ctx.parseDate("2026-06-01"))`, true},
		{`ctx.isFirstOfMonth(ctx.parseDate("2026-06-02"))`, false},
		// Cross-year edge
		{`ctx.isFirstOfMonth("2027-01-01")`, true},
		{`ctx.isFirstOfMonth("2026-12-31")`, false},
	}
	for _, tc := range cases {
		if got := runJS(t, rt, tc.expr).ToBoolean(); got != tc.want {
			t.Fatalf("%s = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

func TestDateBindings_IsLastOfMonth_WithArg(t *testing.T) {
	rt := newRuntimeWithDates(t, fixedNow)
	cases := []struct {
		expr string
		want bool
	}{
		{`ctx.isLastOfMonth("2026-05-31")`, true},
		{`ctx.isLastOfMonth("2026-05-30")`, false},
		// SQLite timestamp form
		{`ctx.isLastOfMonth("2026-05-31 23:59:59")`, true},
		// RFC3339
		{`ctx.isLastOfMonth("2026-05-31T12:00:00Z")`, true},
		// JS Date
		{`ctx.isLastOfMonth(ctx.parseDate("2026-05-31"))`, true},
		{`ctx.isLastOfMonth(ctx.parseDate("2026-05-30"))`, false},
		// Cross-year edge
		{`ctx.isLastOfMonth("2026-12-31")`, true},
		{`ctx.isLastOfMonth("2027-01-01")`, false},
		// Leap year: Feb 29 is the last day of Feb 2024.
		{`ctx.isLastOfMonth("2024-02-29")`, true},
		// Non-leap year: Feb 28 is the last day of Feb 2025.
		{`ctx.isLastOfMonth("2025-02-28")`, true},
	}
	for _, tc := range cases {
		if got := runJS(t, rt, tc.expr).ToBoolean(); got != tc.want {
			t.Fatalf("%s = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// The motivating use case: schedule something N days before the end of
// the month by composing addDays with isLastOfMonth.
func TestDateBindings_IsLastOfMonth_AddDaysComposition(t *testing.T) {
	// On 2026-05-26, today+5 = 2026-05-31 (last of month) → true.
	rt := newRuntimeWithDates(t, time.Date(2026, 5, 26, 14, 30, 0, 0, time.UTC))
	if v := runJS(t, rt, `ctx.isLastOfMonth(ctx.addDays(ctx.today(), 5))`).ToBoolean(); !v {
		t.Fatalf("isLastOfMonth(today+5) on 2026-05-26 = false, want true")
	}
	// On 2026-05-27, today+5 = 2026-06-01 → false.
	rt2 := newRuntimeWithDates(t, time.Date(2026, 5, 27, 14, 30, 0, 0, time.UTC))
	if v := runJS(t, rt2, `ctx.isLastOfMonth(ctx.addDays(ctx.today(), 5))`).ToBoolean(); v {
		t.Fatalf("isLastOfMonth(today+5) on 2026-05-27 = true, want false")
	}
}

func TestDateBindings_IsFirstOfMonth_AddDaysComposition(t *testing.T) {
	// On 2026-05-27, today+5 = 2026-06-01 → true.
	rt := newRuntimeWithDates(t, time.Date(2026, 5, 27, 14, 30, 0, 0, time.UTC))
	if v := runJS(t, rt, `ctx.isFirstOfMonth(ctx.addDays(ctx.today(), 5))`).ToBoolean(); !v {
		t.Fatalf("isFirstOfMonth(today+5) on 2026-05-27 = false, want true")
	}
	// On 2026-05-26, today+5 = 2026-05-31 → false.
	rt2 := newRuntimeWithDates(t, time.Date(2026, 5, 26, 14, 30, 0, 0, time.UTC))
	if v := runJS(t, rt2, `ctx.isFirstOfMonth(ctx.addDays(ctx.today(), 5))`).ToBoolean(); v {
		t.Fatalf("isFirstOfMonth(today+5) on 2026-05-26 = true, want false")
	}
}

// Feb 29 in a non-leap year must surface as a parse error, not silently
// roll into March. The script can catch it; we just have to refuse the
// invalid calendar date.
func TestDateBindings_IsLastOfMonth_InvalidDate(t *testing.T) {
	rt := newRuntimeWithDates(t, fixedNow)
	if _, err := rt.RunString(`ctx.isLastOfMonth("2025-02-29")`); err == nil {
		t.Fatalf("isLastOfMonth(\"2025-02-29\") returned no error; want parse error")
	}
}

func TestDateBindings_IsWeekday(t *testing.T) {
	rt := newRuntimeWithDates(t, fixedNow)
	if v := runJS(t, rt, `ctx.isWeekday("thursday")`).ToBoolean(); !v {
		t.Fatalf("ctx.isWeekday(thursday) = false, want true")
	}
	if v := runJS(t, rt, `ctx.isWeekday("monday")`).ToBoolean(); v {
		t.Fatalf("ctx.isWeekday(monday) = true, want false")
	}
}

func TestDateBindings_DaysSince(t *testing.T) {
	rt := newRuntimeWithDates(t, fixedNow)
	if v := runJS(t, rt, `ctx.daysSince("2026-05-19")`).ToInteger(); v != 2 {
		t.Fatalf("daysSince(2026-05-19) = %d, want 2", v)
	}
	if v := runJS(t, rt, `ctx.daysSince("2026-05-23")`).ToInteger(); v != -2 {
		t.Fatalf("daysSince(2026-05-23) = %d, want -2", v)
	}
}

// Task fields like completed_at come back as SQLite timestamps
// ("YYYY-MM-DD HH:MM:SS"). Scripts must be able to pass those directly
// into daysSince/daysBetween/parseDate without slicing the time portion off.
func TestDateBindings_AcceptsSqliteTimestamp(t *testing.T) {
	rt := newRuntimeWithDates(t, fixedNow)
	if v := runJS(t, rt, `ctx.daysSince("2026-05-19 14:30:00")`).ToInteger(); v != 2 {
		t.Fatalf("daysSince with SQLite ts = %d, want 2", v)
	}
	if v := runJS(t, rt, `ctx.daysBetween("2026-05-21 09:00:00","2026-05-24 23:59:59")`).ToInteger(); v != 3 {
		t.Fatalf("daysBetween with SQLite ts = %d, want 3", v)
	}
	got := runJS(t, rt, `ctx.formatDate(ctx.parseDate("2026-05-21 14:30:00"))`).String()
	if want := "2026-05-21"; got != want {
		t.Fatalf("formatDate(parseDate(SQLite ts)) = %q, want %q", got, want)
	}
}

func TestDateBindings_DaysBetween(t *testing.T) {
	rt := newRuntimeWithDates(t, fixedNow)
	if v := runJS(t, rt, `ctx.daysBetween("2026-05-21","2026-05-24")`).ToInteger(); v != 3 {
		t.Fatalf("daysBetween = %d, want 3", v)
	}
}

func TestDateBindings_AddDaysFormatDate(t *testing.T) {
	rt := newRuntimeWithDates(t, fixedNow)
	got := runJS(t, rt, `ctx.formatDate(ctx.addDays(ctx.parseDate("2026-05-21"), 3))`).String()
	if want := "2026-05-24"; got != want {
		t.Fatalf("formatDate(addDays(parseDate(2026-05-21),3)) = %q, want %q", got, want)
	}
}

func TestDateBindings_ParseDateReturnsJSDate(t *testing.T) {
	rt := newRuntimeWithDates(t, fixedNow)
	// parseDate returns a JS Date; verify by calling .getDate() in JS.
	got := runJS(t, rt, `ctx.parseDate("2026-05-21").getUTCDate()`).ToInteger()
	if got != 21 {
		t.Fatalf("parseDate(2026-05-21).getUTCDate() = %d, want 21", got)
	}
}
