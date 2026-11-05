package cli

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/srliao/tt/internal/db"
	"github.com/srliao/tt/internal/runtime"
	"github.com/srliao/tt/internal/script"
	"github.com/srliao/tt/internal/tag"
	"github.com/srliao/tt/internal/task"
)

// tickInterval matches the scheduler's sweep cadence.
const tickInterval = 15 * time.Minute

const defaultSimulateDays = 35

type simLog struct {
	Level    script.LogLevel `json:"level"`
	Message  string          `json:"message"`
	LoggedAt time.Time       `json:"logged_at"`
}

type simRun struct {
	At         time.Time       `json:"at"`
	Trigger    script.Trigger  `json:"trigger"`
	Spawned    []task.Task     `json:"spawned"`
	Logs       []simLog        `json:"logs"`
	StateAfter json.RawMessage `json:"state_after"`
	Error      *string         `json:"error"`
}

type simResult struct {
	Runs    []simRun `json:"runs"`
	Summary struct {
		SpawnDates []string `json:"spawn_dates"`
	} `json:"summary"`
}

func newScriptSimulateCmd(s *session) *cobra.Command {
	var (
		scriptID                         int64
		codeFile, schedule, from, to, tz string
		completeAfter, snapshot          string
	)
	cmd := &cobra.Command{
		Use:   "simulate [--id N] [--code file.js] [--schedule <schedule>] [--from D] [--to D] [--tz Z] [--complete-after <dur>]",
		Short: "Play a script's schedule forward on a local copy of the server's data",
		Long: `Replay the scheduler's 15-minute ticks over a date range against an
in-memory snapshot, under a fake clock, and report every run. Only the
target script runs, and it runs even if disabled; no other script runs.
State, ctx.lastSpawn and ctx.script.lastRunAt carry from run to run, and
every timestamp written follows the fake clock. Nothing is sent to the server.

Modes:
  --id N                          the stored script, with its real state and last spawn
  --id N --code f [--schedule s]  preview an edit to a stored script, with its real state
  --code f --schedule s           a new script, starting from empty state

--schedule: ` + scheduleFlagHelp + `
--from / --to: YYYY-MM-DD, both inclusive, in the time zone. Default: now
  through now + 35 days.
--tz: IANA zone for calendar days. Default: the snapshot's (server's) zone.
--complete-after <dur>: before each tick, mark done every task the simulation
  spawned at least <dur> ago (Nd, or a Go duration like 36h); its completed_at
  is creation + <dur>. Without it, spawned tasks stay open.
--snapshot file.db: reuse a saved snapshot instead of fetching a fresh one.

Output: {runs: [{at, trigger, spawned: [task], logs: [{level, message, logged_at}],
         state_after: object, error: string|null}],
 summary: {spawn_dates: [YYYY-MM-DD]}}
  runs: only ticks where the script ran
  spawn_dates: the local date of each run that spawned a task, in order
  task: ` + fullTaskShape,
		Example: `  tt-cli script simulate --code plants.js --schedule daily --complete-after 0d`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags := cmd.Flags()
			hasID, hasCode := flags.Changed("id"), flags.Changed("code")
			if !hasID && !hasCode {
				return usageErr("pass --id N, --code file.js --schedule s, or both", nil)
			}
			if hasID && scriptID <= 0 {
				return usageErr("--id must be a positive integer", map[string]any{"value": scriptID})
			}
			if !hasID && schedule == "" {
				return usageErr("a new script (--code without --id) needs --schedule", nil)
			}
			var sch *script.Schedule
			if schedule != "" {
				parsed, err := parseScheduleFlag(schedule)
				if err != nil {
					return err
				}
				sch = &parsed
			}
			var code *string
			if hasCode {
				raw, err := s.readInput(codeFile)
				if err != nil {
					return err
				}
				if err := checkCode(string(raw)); err != nil {
					return err
				}
				c := string(raw)
				code = &c
			}
			var complete *time.Duration
			if completeAfter != "" {
				d, err := parseAge(completeAfter)
				if err != nil {
					return err
				}
				complete = &d
			}
			if tz != "" {
				if _, err := time.LoadLocation(tz); err != nil {
					return usageErr("unknown --tz "+strconv.Quote(tz), map[string]any{"value": tz})
				}
			}

			store, meta, err := s.openSnapshot(cmd.Context(), snapshot)
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()

			if tz == "" {
				tz = meta.Timezone
			}
			loc, err := time.LoadLocation(tz)
			if err != nil {
				return validationErr("snapshot time zone "+strconv.Quote(tz)+" is unknown; pass --tz", nil)
			}
			start, end := s.Now(), s.Now().AddDate(0, 0, defaultSimulateDays)
			if from != "" {
				if start, err = localDate("--from", from, loc); err != nil {
					return err
				}
			}
			if to != "" {
				if end, err = localDate("--to", to, loc); err != nil {
					return err
				}
				end = end.AddDate(0, 0, 1).Add(-time.Nanosecond)
			}
			if end.Before(start) {
				return validationErr("--to is before --from", nil)
			}

			sim := newSimulation(store, loc, start)
			id, err := sim.target(cmd.Context(), scriptID, code, sch)
			if err != nil {
				return err
			}
			out, err := sim.run(cmd.Context(), id, start, end, complete)
			if err != nil {
				return err
			}
			return s.out(out)
		},
	}
	f := cmd.Flags()
	f.Int64Var(&scriptID, "id", 0, "stored script id")
	f.StringVar(&codeFile, "code", "", "script file, or - for stdin")
	f.StringVar(&schedule, "schedule", "", scheduleFlagHelp)
	f.StringVar(&from, "from", "", "first day, YYYY-MM-DD (default now)")
	f.StringVar(&to, "to", "", "last day, YYYY-MM-DD (default now + 35 days)")
	f.StringVar(&tz, "tz", "", "IANA time zone (default: the snapshot's)")
	f.StringVar(&completeAfter, "complete-after", "", "auto-complete spawned tasks this long after creation, e.g. 0d, 2d, 36h")
	f.StringVar(&snapshot, "snapshot", "", "saved snapshot (default: fetch a fresh one)")
	return cmd
}

// parseAge parses Nd (whole days) or a Go duration.
func parseAge(raw string) (time.Duration, error) {
	if n, ok := strings.CutSuffix(raw, "d"); ok {
		if days, err := strconv.Atoi(n); err == nil && days >= 0 {
			return time.Duration(days) * 24 * time.Hour, nil
		}
	} else if d, err := time.ParseDuration(raw); err == nil && d >= 0 {
		return d, nil
	}
	return 0, usageErr("--complete-after must be Nd (e.g. 0d, 2d) or a duration like 36h", map[string]any{"value": raw})
}

// localDate is the start of the YYYY-MM-DD day raw in loc.
func localDate(flag, raw string, loc *time.Location) (time.Time, error) {
	if err := checkDate(flag, raw); err != nil {
		return time.Time{}, err
	}
	return time.ParseInLocation(time.DateOnly, raw, loc)
}

// simulation runs the real task, script and runtime services against a
// snapshot under one fake clock.
type simulation struct {
	now     time.Time
	loc     *time.Location
	tasks   *task.Impl
	scripts *script.Impl
	runner  *runtime.Runner
}

func newSimulation(store *db.Store, loc *time.Location, start time.Time) *simulation {
	sim := &simulation{now: start, loc: loc}
	clock := func() time.Time { return sim.now }
	sim.tasks = task.New(store, task.WithClock(clock))
	sim.scripts = script.New(store, script.WithClock(clock))
	sim.runner = runtime.New(sim.tasks, tag.New(store), sim.scripts,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		runtime.WithClock(clock), runtime.WithLocation(loc))
	return sim
}

// target prepares the script to simulate and returns its id: the stored
// script id (with any code/schedule override), or a new script when id is 0.
// It is enabled in the snapshot because the runtime refuses disabled
// scripts; the snapshot is thrown away afterwards.
func (sim *simulation) target(ctx context.Context, id int64, code *string, sch *script.Schedule) (int64, error) {
	if id == 0 {
		created, err := sim.scripts.Create(ctx, script.CreateInput{
			Name: "simulation", Code: *code, Enabled: true, Schedule: *sch,
		})
		if err != nil {
			return 0, serviceErr(err)
		}
		return created.ID, nil
	}
	sc, err := sim.scripts.Get(ctx, id)
	if err != nil {
		return 0, serviceErr(err)
	}
	in := script.UpdateInput{Name: sc.Name, Code: sc.Code, Enabled: true, Schedule: sc.Schedule}
	if code != nil {
		in.Code = *code
	}
	if sch != nil {
		in.Schedule = *sch
	}
	if _, err := sim.scripts.Update(ctx, id, in); err != nil {
		return 0, serviceErr(err)
	}
	return id, nil
}

// run walks the scheduler's ticks from start through end. At each tick it
// first completes spawns older than complete, then runs the script if its
// schedule is due, the same test the scheduler's DueAt applies.
func (sim *simulation) run(ctx context.Context, id int64, start, end time.Time, complete *time.Duration) (simResult, error) {
	out := simResult{Runs: []simRun{}}
	out.Summary.SpawnDates = []string{}
	var open []task.Task
	for at := start; !at.After(end); at = at.Add(tickInterval) {
		if complete != nil {
			var err error
			if open, err = sim.completeSpawns(ctx, open, at, *complete); err != nil {
				return out, err
			}
		}
		sim.now = at
		sc, err := sim.scripts.Get(ctx, id)
		if err != nil {
			return out, serviceErr(err)
		}
		if !sc.Schedule.Matches(at, sc.LastRunAt, sim.loc) {
			continue
		}
		r, err := sim.runOnce(ctx, id, at)
		if err != nil {
			return out, err
		}
		out.Runs = append(out.Runs, r)
		if len(r.Spawned) > 0 {
			out.Summary.SpawnDates = append(out.Summary.SpawnDates, at.In(sim.loc).Format(time.DateOnly))
		}
		open = append(open, r.Spawned...)
	}
	return out, nil
}

// completeSpawns marks done every open spawn created at least age before
// at, stamping completed_at at creation + age. It returns the rest.
func (sim *simulation) completeSpawns(ctx context.Context, open []task.Task, at time.Time, age time.Duration) ([]task.Task, error) {
	var still []task.Task
	for _, t := range open {
		due := t.CreatedAt.Add(age)
		if due.After(at) {
			still = append(still, t)
			continue
		}
		sim.now = due
		if _, err := sim.tasks.SetState(ctx, t.ID, task.StateDone); err != nil {
			return nil, serviceErr(err)
		}
	}
	return still, nil
}

func (sim *simulation) runOnce(ctx context.Context, id int64, at time.Time) (simRun, error) {
	started, err := sim.scripts.StartRun(ctx, id, script.TriggerScheduled)
	if err != nil {
		return simRun{}, serviceErr(err)
	}
	if err := sim.runner.Run(ctx, id, started.ID, script.TriggerScheduled); err != nil {
		return simRun{}, serviceErr(err)
	}
	run, err := sim.scripts.GetRun(ctx, started.ID)
	if err != nil {
		return simRun{}, serviceErr(err)
	}
	logs, err := sim.scripts.GetLogs(ctx, started.ID)
	if err != nil {
		return simRun{}, serviceErr(err)
	}
	state, err := sim.scripts.ReadUserState(ctx, id)
	if err != nil {
		return simRun{}, serviceErr(err)
	}
	r := simRun{
		At:         at,
		Trigger:    run.Trigger,
		Spawned:    []task.Task{},
		Logs:       make([]simLog, 0, len(logs)),
		StateAfter: json.RawMessage(state),
	}
	if !json.Valid(r.StateAfter) {
		r.StateAfter = json.RawMessage("{}")
	}
	for _, l := range logs {
		r.Logs = append(r.Logs, simLog{Level: l.Level, Message: l.Message, LoggedAt: l.LoggedAt})
	}
	for _, tid := range run.SpawnedTaskIDs {
		t, err := sim.tasks.Get(ctx, tid)
		if err != nil {
			return simRun{}, serviceErr(err)
		}
		r.Spawned = append(r.Spawned, t)
	}
	if run.Status != script.RunStatusOK {
		msg := run.ErrorMessage
		if msg == "" {
			msg = string(run.Status)
		}
		r.Error = &msg
	}
	return r, nil
}
