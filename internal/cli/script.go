package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/srliao/tt/internal/runtime"
	"github.com/srliao/tt/internal/script"
)

func newScriptCmd(s *session) *cobra.Command {
	return group("script", "Read, check, save and simulate scripts",
		newScriptListCmd(s),
		newScriptGetCmd(s),
		newScriptCheckCmd(s),
		newScriptCreateCmd(s),
		newScriptUpdateCmd(s),
		newScriptRunCmd(s),
		newScriptRunsCmd(s),
		newScriptSimulateCmd(s),
	)
}

const scriptShape = `{id, name, code, enabled, schedule: {kind, weekday?, day?}, last_run_at?, created_at, updated_at}`

const runShape = `{id, script_id, started_at, finished_at?, status, error_message, spawned_task_ids: [id], trigger}`

const scheduleFlagHelp = "every_tick | daily | weekly:<weekday> | monthly:<1-31|last>"

// nextStepEnable tells the agent that a CLI-saved script does nothing until
// the user enables it in the UI.
const nextStepEnable = "enable in UI"

// scriptBody is the POST/PATCH /scripts body. PATCH replaces every field
// except enabled, which the server leaves alone when it is omitted.
type scriptBody struct {
	Name     string          `json:"name"`
	Code     string          `json:"code"`
	Enabled  *bool           `json:"enabled,omitempty"`
	Schedule script.Schedule `json:"schedule"`
}

type savedScript struct {
	script.Script
	NextStep string `json:"next_step"`
}

// parseScheduleFlag parses the --schedule format into a script.Schedule,
// with the same validation the server applies.
func parseScheduleFlag(raw string) (script.Schedule, error) {
	kind, arg, hasArg := strings.Cut(strings.TrimSpace(raw), ":")
	var cfg string
	switch script.Kind(kind) {
	case script.KindEveryTick, script.KindDaily:
		if hasArg {
			return script.Schedule{}, scheduleErr(raw, kind+" takes no argument")
		}
	case script.KindWeekly:
		cfg = fmt.Sprintf(`{"weekday": %q}`, arg)
	case script.KindMonthly:
		cfg = `{"day": "last"}`
		if arg != "last" {
			n, err := strconv.Atoi(arg)
			if err != nil {
				return script.Schedule{}, scheduleErr(raw, "monthly day must be 1-31 or last")
			}
			cfg = fmt.Sprintf(`{"day": %d}`, n)
		}
	}
	sch, err := script.ParseSchedule(kind, cfg)
	if err != nil {
		return script.Schedule{}, scheduleErr(raw, strings.TrimPrefix(err.Error(), "script: "))
	}
	return sch, nil
}

func scheduleErr(raw, msg string) error {
	return validationErr("invalid --schedule "+fmt.Sprintf("%q", raw)+": "+msg+"; want "+scheduleFlagHelp,
		map[string]any{"field": "schedule", "value": raw})
}

// checkCode compiles code the way the server and runtime do.
func checkCode(code string) error {
	err := runtime.Check(code)
	if err == nil {
		return nil
	}
	details := map[string]any{"field": "code"}
	var ce *runtime.CheckError
	if errors.As(err, &ce) {
		details["line"] = ce.Line
		details["column"] = ce.Column
	}
	return validationErr(err.Error(), details)
}

func newScriptListCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List every script",
		Long: `List every script, enabled or not.

Output: {scripts: [script]}
  script: ` + scriptShape,
		Example: `  tt-cli script list`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := s.api()
			if err != nil {
				return err
			}
			var out []script.Script
			if _, err := c.Do(cmd.Context(), http.MethodGet, "/scripts", nil, nil, &out); err != nil {
				return err
			}
			if out == nil {
				out = []script.Script{}
			}
			return s.out(map[string]any{"scripts": out})
		},
	}
}

func (s *session) getScript(ctx context.Context, id int64) (script.Script, error) {
	c, err := s.api()
	if err != nil {
		return script.Script{}, err
	}
	var out script.Script
	_, err = c.Do(ctx, http.MethodGet, fmt.Sprintf("/scripts/%d", id), nil, nil, &out)
	return out, err
}

func newScriptGetCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Show one script, including its code",
		Long: `Show one script, including its code.

Output: ` + scriptShape,
		Example: `  tt-cli script get 3`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID("id", args[0])
			if err != nil {
				return err
			}
			sc, err := s.getScript(cmd.Context(), id)
			if err != nil {
				return err
			}
			return s.out(sc)
		},
	}
}

func newScriptCheckCmd(s *session) *cobra.Command {
	var codeFile, schedule string
	cmd := &cobra.Command{
		Use:   "check [--code file.js] --schedule <schedule>",
		Short: "Compile a script and parse its schedule, locally",
		Long: `Compile the code (without running it) and parse the schedule. Runs locally
and needs no server. A compile error fails with validation_failed and
details {field: "code", line, column}.

--schedule: ` + scheduleFlagHelp + `

Output: {ok: true, schedule: {kind, weekday?, day?}}`,
		Example: `  tt-cli script check --code plants.js --schedule weekly:monday`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if schedule == "" {
				return usageErr("--schedule is required", nil)
			}
			sch, err := parseScheduleFlag(schedule)
			if err != nil {
				return err
			}
			code, err := s.readInput(codeFile)
			if err != nil {
				return err
			}
			if err := checkCode(string(code)); err != nil {
				return err
			}
			return s.out(map[string]any{"ok": true, "schedule": sch})
		},
	}
	cmd.Flags().StringVar(&codeFile, "code", "", "script file (default stdin)")
	cmd.Flags().StringVar(&schedule, "schedule", "", scheduleFlagHelp)
	return cmd
}

func newScriptCreateCmd(s *session) *cobra.Command {
	var name, codeFile, schedule string
	cmd := &cobra.Command{
		Use:   "create --name <name> [--code file.js] --schedule <schedule>",
		Short: "Save a new script (always disabled)",
		Long: `Check the script locally, then save it. Scripts saved from the CLI are always
disabled and the CLI cannot enable them: tell the user to enable it in the
UI. Preview what it will do first with ` + "`tt-cli script simulate`" + `.

--schedule: ` + scheduleFlagHelp + `

Output: {...script, next_step: "enable in UI"}
  script: ` + scriptShape,
		Example: `  tt-cli script create --name plants --code plants.js --schedule weekly:monday`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(name) == "" {
				return usageErr("--name is required", nil)
			}
			if schedule == "" {
				return usageErr("--schedule is required", nil)
			}
			sch, err := parseScheduleFlag(schedule)
			if err != nil {
				return err
			}
			code, err := s.readInput(codeFile)
			if err != nil {
				return err
			}
			if err := checkCode(string(code)); err != nil {
				return err
			}
			c, err := s.api()
			if err != nil {
				return err
			}
			var out script.Script
			disabled := false
			body := scriptBody{Name: name, Code: string(code), Enabled: &disabled, Schedule: sch}
			if _, err := c.Do(cmd.Context(), http.MethodPost, "/scripts", nil, body, &out); err != nil {
				return err
			}
			return s.out(savedScript{Script: out, NextStep: nextStepEnable})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "script name (required)")
	cmd.Flags().StringVar(&codeFile, "code", "", "script file (default stdin)")
	cmd.Flags().StringVar(&schedule, "schedule", "", scheduleFlagHelp+" (required)")
	return cmd
}

func newScriptUpdateCmd(s *session) *cobra.Command {
	var name, codeFile, schedule string
	cmd := &cobra.Command{
		Use:   "update <id> [--name <name>] [--code file.js|-] [--schedule <schedule>]",
		Short: "Change a script's name, code or schedule",
		Long: `Change the given fields; the rest keep their current values. New code is
checked locally first. Whether the script is enabled is never changed: only
the UI can enable or disable it. Pass --code - to read the code from stdin.

--schedule: ` + scheduleFlagHelp + `

Output: {...script, next_step: "enable in UI"}
  script: ` + scriptShape,
		Example: `  tt-cli script update 3 --code plants.js`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID("id", args[0])
			if err != nil {
				return err
			}
			flags := cmd.Flags()
			if !flags.Changed("name") && !flags.Changed("code") && !flags.Changed("schedule") {
				return usageErr("nothing to update: pass --name, --code or --schedule", nil)
			}
			var body scriptBody
			if flags.Changed("schedule") {
				if body.Schedule, err = parseScheduleFlag(schedule); err != nil {
					return err
				}
			}
			if flags.Changed("code") {
				code, err := s.readInput(codeFile)
				if err != nil {
					return err
				}
				if err := checkCode(string(code)); err != nil {
					return err
				}
				body.Code = string(code)
			}
			cur, err := s.getScript(cmd.Context(), id)
			if err != nil {
				return err
			}
			body.Name = cur.Name
			if flags.Changed("name") {
				body.Name = name
			}
			if !flags.Changed("code") {
				body.Code = cur.Code
			}
			if !flags.Changed("schedule") {
				body.Schedule = cur.Schedule
			}
			c, err := s.api()
			if err != nil {
				return err
			}
			var out script.Script
			if _, err := c.Do(cmd.Context(), http.MethodPatch, fmt.Sprintf("/scripts/%d", id), nil, body, &out); err != nil {
				return err
			}
			saved := savedScript{Script: out}
			if !out.Enabled {
				saved.NextStep = nextStepEnable
			}
			return s.out(saved)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "new name")
	cmd.Flags().StringVar(&codeFile, "code", "", "new script file, or - for stdin")
	cmd.Flags().StringVar(&schedule, "schedule", "", scheduleFlagHelp)
	return cmd
}

func newScriptRunCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "run <id>",
		Short: "Run an enabled script now, on the server",
		Long: `Queue a manual run on the server. This really spawns tasks and writes state.
A disabled script is refused with validation_failed; to see what a script
would do without running it, use ` + "`tt-cli script simulate`" + `. Follow the
run with ` + "`tt-cli script runs <id>`" + `.

Output: {run_id}`,
		Example: `  tt-cli script run 3`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID("id", args[0])
			if err != nil {
				return err
			}
			c, err := s.api()
			if err != nil {
				return err
			}
			var out map[string]int64
			if _, err := c.Do(cmd.Context(), http.MethodPost, fmt.Sprintf("/scripts/%d/run", id), nil, nil, &out); err != nil {
				return err
			}
			return s.out(out)
		},
	}
}

func newScriptRunsCmd(s *session) *cobra.Command {
	limit := 10
	cmd := &cobra.Command{
		Use:   "runs <id> [--limit N]",
		Short: "List a script's recent runs, newest first",
		Long: `List a script's most recent runs, newest first.

Output: {runs: [run]}
  run: ` + runShape,
		Example: `  tt-cli script runs 3 --limit 5`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID("id", args[0])
			if err != nil {
				return err
			}
			if limit <= 0 {
				return usageErr("--limit must be positive", map[string]any{"value": limit})
			}
			c, err := s.api()
			if err != nil {
				return err
			}
			var out []script.Run
			if _, err := c.Do(cmd.Context(), http.MethodGet, fmt.Sprintf("/scripts/%d/runs", id),
				url.Values{"limit": {fmt.Sprint(limit)}}, nil, &out); err != nil {
				return err
			}
			if out == nil {
				out = []script.Run{}
			}
			return s.out(map[string]any{"runs": out})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", limit, "number of runs")
	return cmd
}
