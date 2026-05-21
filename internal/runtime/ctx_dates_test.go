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
