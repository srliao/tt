package script_test

import (
	"testing"
	"time"

	"github.com/srliao/tt/internal/script"
)

// at parses a "2006-01-02 15:04" timestamp in UTC. Tests use UTC clocks
// throughout to keep weekday/day-boundary checks deterministic.
func at(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse("2006-01-02 15:04", s)
	if err != nil {
		t.Fatalf("at(%q): %v", s, err)
	}
	return ts.UTC()
}

// ptr is a tiny helper to turn a time.Time into a *time.Time literal.
func ptr(t time.Time) *time.Time { return &t }

func TestScheduleMatches(t *testing.T) {
	cases := []struct {
		name string
		sch  script.Schedule
		now  time.Time
		last *time.Time
		want bool
	}{
		{
			name: "every_tick always",
			sch:  script.Schedule{Kind: script.KindEveryTick},
			now:  at(t, "2026-05-21 09:30"),
			last: ptr(at(t, "2026-05-21 09:29")),
			want: true,
		},
		{
			name: "daily last yesterday",
			sch:  script.Schedule{Kind: script.KindDaily},
			now:  at(t, "2026-05-21 09:30"),
			last: ptr(at(t, "2026-05-20 23:50")),
			want: true,
		},
		{
			name: "daily last today",
			sch:  script.Schedule{Kind: script.KindDaily},
			now:  at(t, "2026-05-21 09:30"),
			last: ptr(at(t, "2026-05-21 08:00")),
			want: false,
		},
		{
			name: "daily never",
			sch:  script.Schedule{Kind: script.KindDaily},
			now:  at(t, "2026-05-21 09:30"),
			last: nil,
			want: true,
		},
		{
			name: "weekly monday on mon",
			sch:  script.Schedule{Kind: script.KindWeekly, Weekday: script.Monday},
			now:  at(t, "2026-05-25 09:00"), // Monday
			last: nil,
			want: true,
		},
		{
			name: "weekly monday on tue",
			sch:  script.Schedule{Kind: script.KindWeekly, Weekday: script.Monday},
			now:  at(t, "2026-05-26 09:00"), // Tuesday
			last: nil,
			want: false,
		},
		{
			name: "weekly monday already ran same monday",
			sch:  script.Schedule{Kind: script.KindWeekly, Weekday: script.Monday},
			now:  at(t, "2026-05-25 09:00"),
			last: ptr(at(t, "2026-05-25 08:00")),
			want: false,
		},
		{
			name: "monthly 15 on 15th",
			sch:  script.Schedule{Kind: script.KindMonthly, Day: script.MonthlyDay{N: 15, Valid: true}},
			now:  at(t, "2026-05-15 09:00"),
			last: nil,
			want: true,
		},
		{
			name: "monthly 15 on 14th",
			sch:  script.Schedule{Kind: script.KindMonthly, Day: script.MonthlyDay{N: 15, Valid: true}},
			now:  at(t, "2026-05-14 09:00"),
			last: nil,
			want: false,
		},
		{
			name: "monthly last on may 31",
			sch:  script.Schedule{Kind: script.KindMonthly, Day: script.MonthlyDay{IsLast: true, Valid: true}},
			now:  at(t, "2026-05-31 09:00"),
			last: nil,
			want: true,
		},
		{
			name: "monthly last on may 30",
			sch:  script.Schedule{Kind: script.KindMonthly, Day: script.MonthlyDay{IsLast: true, Valid: true}},
			now:  at(t, "2026-05-30 09:00"),
			last: nil,
			want: false,
		},
		{
			name: "monthly last on feb 28 2026",
			sch:  script.Schedule{Kind: script.KindMonthly, Day: script.MonthlyDay{IsLast: true, Valid: true}},
			now:  at(t, "2026-02-28 09:00"),
			last: nil,
			want: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := tc.sch.Matches(tc.now, tc.last)
			if got != tc.want {
				t.Errorf("Matches() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseScheduleEveryTickAndDaily(t *testing.T) {
	for _, kind := range []string{"every_tick", "daily"} {
		sch, err := script.ParseSchedule(kind, "{}")
		if err != nil {
			t.Fatalf("ParseSchedule(%q): %v", kind, err)
		}
		if string(sch.Kind) != kind {
			t.Errorf("Kind = %q, want %q", sch.Kind, kind)
		}
	}
}

func TestParseScheduleWeeklyValid(t *testing.T) {
	sch, err := script.ParseSchedule("weekly", `{"weekday":"wednesday"}`)
	if err != nil {
		t.Fatalf("ParseSchedule: %v", err)
	}
	if sch.Kind != script.KindWeekly {
		t.Errorf("Kind = %q, want weekly", sch.Kind)
	}
	if sch.Weekday != script.Wednesday {
		t.Errorf("Weekday = %q, want wednesday", sch.Weekday)
	}
}

func TestParseScheduleWeeklyInvalid(t *testing.T) {
	if _, err := script.ParseSchedule("weekly", `{"weekday":"funday"}`); err == nil {
		t.Fatalf("expected error for invalid weekday")
	}
}

func TestParseScheduleMonthlyByDay(t *testing.T) {
	sch, err := script.ParseSchedule("monthly", `{"day":15}`)
	if err != nil {
		t.Fatalf("ParseSchedule: %v", err)
	}
	if sch.Kind != script.KindMonthly {
		t.Errorf("Kind = %q, want monthly", sch.Kind)
	}
	if !sch.Day.Valid || sch.Day.IsLast || sch.Day.N != 15 {
		t.Errorf("Day = %+v, want {N:15 Valid}", sch.Day)
	}
}

func TestParseScheduleMonthlyLast(t *testing.T) {
	sch, err := script.ParseSchedule("monthly", `{"day":"last"}`)
	if err != nil {
		t.Fatalf("ParseSchedule: %v", err)
	}
	if !sch.Day.Valid || !sch.Day.IsLast {
		t.Errorf("Day = %+v, want {IsLast Valid}", sch.Day)
	}
}

func TestParseScheduleMonthlyOutOfRange(t *testing.T) {
	if _, err := script.ParseSchedule("monthly", `{"day":0}`); err == nil {
		t.Fatalf("expected error for day=0")
	}
	if _, err := script.ParseSchedule("monthly", `{"day":32}`); err == nil {
		t.Fatalf("expected error for day=32")
	}
}

func TestMarshalConfigRoundTrip(t *testing.T) {
	cases := []script.Schedule{
		{Kind: script.KindEveryTick},
		{Kind: script.KindDaily},
		{Kind: script.KindWeekly, Weekday: script.Friday},
		{Kind: script.KindMonthly, Day: script.MonthlyDay{N: 7, Valid: true}},
		{Kind: script.KindMonthly, Day: script.MonthlyDay{IsLast: true, Valid: true}},
	}
	for _, in := range cases {
		cfg, err := in.MarshalConfig()
		if err != nil {
			t.Fatalf("MarshalConfig(%+v): %v", in, err)
		}
		got, err := script.ParseSchedule(string(in.Kind), cfg)
		if err != nil {
			t.Fatalf("ParseSchedule(%q,%q): %v", in.Kind, cfg, err)
		}
		if got.Kind != in.Kind || got.Weekday != in.Weekday ||
			got.Day.Valid != in.Day.Valid || got.Day.IsLast != in.Day.IsLast || got.Day.N != in.Day.N {
			t.Errorf("round-trip mismatch: in=%+v cfg=%q got=%+v", in, cfg, got)
		}
	}
}
