package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/srliao/tt/internal/runtime/docs"
)

// helpTopics is the agent's manual beyond per-command --help. The order is
// the order `help overview` lists them in.
type helpTopic struct {
	name, short, body string
}

var helpTopics = []helpTopic{
	{"overview", "what tt is, definitions, common workflows", helpOverview},
	{"plans", "plan document schema, ref rules, output", helpPlans},
	{"scripts", "ctx cheatsheet, schedules, simulate", helpScripts + docs.CtxCheatsheet},
	{"errors", "error JSON, codes, exit codes", helpErrors},
	{"config", "config file, env vars, Cloudflare Access", helpConfig},
}

func topicNames() []string {
	names := make([]string, len(helpTopics))
	for i, t := range helpTopics {
		names[i] = t.name
	}
	return names
}

// newHelpCmd replaces cobra's help command: `help` and `help <topic>` print
// the hand-written topics, and `help <command>...` still shows that
// command's --help.
func newHelpCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "help [overview | plans | scripts | errors | config | <command>...]",
		Short: "Print the manual: start with `tt-cli help`",
		Long: `Print a help topic, or a command's --help.

Topics: ` + strings.Join(topicNames(), ", ") + `. With no argument prints the
overview. Help is plain text, the one exception to JSON-only stdout.`,
		Example: `  tt-cli help
  tt-cli help plans
  tt-cli help task list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := "overview"
			if len(args) > 0 {
				name = args[0]
			}
			if i := slices.IndexFunc(helpTopics, func(t helpTopic) bool { return t.name == name }); i >= 0 && len(args) <= 1 {
				body := helpTopics[i].body
				if name == "overview" {
					body += overviewTopicList()
				}
				_, err := fmt.Fprint(s.Stdout, body)
				return err
			}
			root := cmd.Root()
			if target, _, err := root.Find(args); err == nil && target != root {
				return target.Help()
			}
			return usageErr(fmt.Sprintf("unknown help topic or command %q", strings.Join(args, " ")),
				map[string]any{"topics": topicNames()})
		},
	}
}

func overviewTopicList() string {
	var b strings.Builder
	for _, t := range helpTopics {
		fmt.Fprintf(&b, "  tt-cli help %-9s %s\n", t.name, t.short)
	}
	return b.String()
}

var helpOverview = `tt-cli: the command line for a tt task tracker instance

tt is a single-user task tracker. Userscripts (small JavaScript programs)
run on a schedule and create tasks, so recurring chores appear on their own.
tt-cli reads and changes a tt instance: tasks, dependencies, the stage, tags,
projects and scripts. Every command prints JSON on stdout; errors are JSON on stderr
(see ` + "`tt-cli help errors`" + `). Run ` + "`tt-cli <command> --help`" + ` before using a
command; never guess flags.

Definitions
  Task        One unit of work: title, notes, optional due date and project, a
              state and tags.
  State       not_done, done or cancelled. An open task is not_done.
  Stage       A hand-picked, separately ordered set of tasks being worked now.
              Finished tasks stay visible in it until clear-finished.
  Tag         A named label. Tags must exist before use (` + "`tt-cli tag create`" + `);
              an unknown tag name is an error that lists the existing tags.
  Project     A free-text name for the body of work a task belongs to; at most
              one per task. Unlike tags, projects need not exist before use: a
              project exists while some task names it. Names are lowercased.
  Dependency  "Task A cannot be completed until task B is resolved." B is A's
              blocker; A is B's dependent.
  Blocker     The task a dependency waits on.
  Blocked     A task with at least one blocker still open or cancelled. Never
              stored, always worked out from the blockers.
  Ready       An open task that is not blocked.
  Goal        A task other tasks block, used as the end point of a plan. Not a
              separate kind of task, and not a project.
  Script      User JavaScript that runs on a schedule in a sandbox and can create
              tasks. Scripts saved by tt-cli are always disabled; the user
              enables them in the web UI.
  Spawn       A task created by a script run.

Dates are always YYYY-MM-DD. No relative dates: work out the date yourself.

Workflows
  Capture a task
    tt-cli tag list                      # pick existing tags
    tt-cli task create --title "Renew passport" --due 2026-12-01 --tag admin

  Group tasks by project
    tt-cli project list                  # existing projects with counts
    tt-cli project set home 42 43        # put tasks in a project
    tt-cli task list --project home      # open tasks in it

  Plan a goal (several tasks wired with dependencies, in one step)
    tt-cli plan check -f plan.json       # dry run against a fresh snapshot
    tt-cli plan apply -f plan.json       # all-or-nothing write
    See ` + "`tt-cli help plans`" + `.

  Make a recurring task
    tt-cli script check --code chore.js --schedule weekly:monday
    tt-cli script simulate --code chore.js --schedule weekly:monday
    tt-cli script create --code chore.js --schedule weekly:monday ...   # see its --help
    Then tell the user to enable the script in the web UI; tt-cli cannot.
    See ` + "`tt-cli help scripts`" + `.

  Work the stage
    tt-cli stage list                    # what is being worked now, in order
    tt-cli task list --ready             # candidates to pull in
    tt-cli task stage 42                 # add a task to the stage
    tt-cli task state 42 done            # finish it
    tt-cli stage clear-finished          # drop finished tasks from the stage

  When something fails
    tt-cli doctor                        # config, auth, server version and time

Topics
`

var helpPlans = `tt-cli help plans: create tasks and wire dependencies in one step

A plan document is JSON, read from -f file.json or stdin (-f -):

  {
    "tasks": [
      {"ref": "design", "title": "Design schema", "notes": "", "due_date": "2026-11-20",
       "project": "tt", "tags": ["tt"], "stage": true},
      {"ref": "impl", "title": "Implement"}
    ],
    "deps": [
      {"task": "impl", "depends_on": "design"},
      {"task": "impl", "depends_on": 42}
    ],
    "deps_remove": [
      {"task": 17, "depends_on": 42}
    ]
  }

tasks        New tasks to create. "title" is required; "notes", "due_date"
             (YYYY-MM-DD), "project" (any name), "tags" (existing tag names)
             and "stage" (true puts the task on the stage) are optional.
deps         Dependencies to add: "task" cannot be completed until
             "depends_on" is resolved.
deps_remove  Dependencies to remove. Removals run before additions.

Ref rules
  - "ref" is a name local to this document; it is never stored.
  - Each end of a dependency is a ref (JSON string) naming a task in "tasks",
    or an existing task ID (JSON number).
  - Refs must be unique within the document; an unknown ref is an error.
  - The usual dependency rules apply: no cycles, no self-dependency, and the
    same checks as ` + "`tt-cli deps add`" + `.

Commands
  tt-cli plan check -f plan.json [--snapshot file.db]
      Applies the plan to an in-memory copy of the server's data (a fresh
      snapshot, or a saved one from ` + "`tt-cli snapshot pull`" + `). Writes nothing.
      The IDs it returns are provisional.
  tt-cli plan apply -f plan.json
      Sends the plan to the server, which applies it in one transaction. It
      is all-or-nothing: on any error nothing is written. There is no
      server-side dry run; apply is the final check.

Output (check and apply have the same shape)
  {"ids": {"design": 101, "impl": 102},
   "tasks": [full task, with blocked_by, blocks and blocked, ...]}

Plans only create tasks and change dependencies. To change an existing
task's state, tags or stage use the task commands; scripts are not part of
plans.
`

var helpScripts = `tt-cli help scripts: recurring tasks with userscripts

A script is JavaScript run by the scheduler on a schedule. It reads the
ctx object (reference below) and calls ctx.queueTask to create tasks
(spawns). Queued tasks and state writes are kept only if the run finishes
without an error.

Saved disabled
  Scripts created or updated through tt-cli are always saved disabled, and
  tt-cli cannot enable them. ` + "`script create`" + ` prints "next_step": "enable in UI":
  tell the user to enable the script in the tt web UI.

Schedules (--schedule)
  every_tick         every scheduler tick (every 15 minutes)
  daily              once per calendar day, at the first tick of the day
  weekly:<weekday>   once on that weekday: monday, tuesday, ... sunday
  monthly:<n|last>   once on day n (1..31) of the month, or the last day
  Days are read in the server's time zone. A monthly day the month does not
  have (e.g. 31 in April) does not fire that month.

Workflow
  tt-cli script check --code chore.js --schedule weekly:monday
      Compile the code and parse the schedule. Local, no network.
  tt-cli script simulate --code chore.js --schedule weekly:monday
      Play the schedule against a snapshot before saving (below).
  tt-cli script create --code chore.js --schedule weekly:monday ...
      Runs check locally first, then saves the script disabled.
  tt-cli script update <id> --code chore.js
      Runs check locally first; never changes enabled.
  Code is read from --code file, or stdin with --code -.
  Other commands: script list | get | run | runs (see each --help).

Simulate
  Modes
    --id N                         the stored script, with its real state and
                                   lastSpawn from the snapshot
    --id N --code f [--schedule s] preview an edit to a stored script
    --code f --schedule s          a new script, starting from empty state
  Options
    --from YYYY-MM-DD, --to YYYY-MM-DD   range; default now to now+35 days
    --tz <IANA zone>                     default: the snapshot's time zone
    --complete-after <dur>               e.g. 0d, 2d: mark each spawned task done
                                         that long after it was created.
                                         Without it spawns stay open.
    --snapshot file.db                   reuse a saved snapshot instead of
                                         fetching a fresh one
  It plays the scheduler's 15-minute ticks across the range under a fake
  clock. Only the target script runs, even if it is disabled. State,
  lastSpawn and lastRunAt carry over between ticks. Nothing is written to
  the server.
  Output
    {"runs": [{"at", "trigger", "spawned": [task, ...], "logs": [...],
               "state_after": {...}, "error"}],
     "summary": {"spawn_dates": ["YYYY-MM-DD", ...]}}
  Check summary.spawn_dates against what the user asked for.

`

var helpErrors = `tt-cli help errors: error shape, codes and exit codes

On failure stdout is empty and stderr has one JSON object:

  {"code": "validation_failed", "message": "human-readable text",
   "details": {...}}

"details" is optional and carries hints, e.g. the existing tags when a tag
name is unknown, or field-level validation problems.

Exit codes
  0  ok
  1  usage: bad flags or arguments, missing or invalid config
  2  not found
  3  validation or conflict: the request was understood but rejected
  4  auth, network or server problem, including version_mismatch

Codes
  usage              bad command line (exit 1)
  config_missing     no server url configured (exit 1)
  config_invalid     the config file cannot be read or parsed (exit 1)
  not_found          the task, tag or script does not exist (exit 2)
  validation_failed  invalid input: bad date, unknown tag, dependency cycle,
                     script compile error, ... (exit 3)
  conflict           the change clashes with current data (exit 3)
  auth_failed        the server or Cloudflare Access rejected the credentials
                     (exit 4)
  network_error      the server could not be reached or sent a bad response
                     (exit 4)
  version_mismatch   a snapshot's or the server's schema differs from this
                     tt-cli; the message says whether to upgrade tt-cli or the
                     server (exit 4)
  scheduler_busy     the server's scheduler could not take the request; retry
                     later (exit 4)
  internal           server-side bug (exit 4)
  any other code     passed through unchanged from the server (exit 4)

On exit 4, or whenever a failure is unclear, run ` + "`tt-cli doctor`" + `: it reports
the config source, reachability, auth, the server's version and time, and
whether the server's schema matches this tt-cli.
`

var helpConfig = `tt-cli help config: connecting to a tt instance

Config file: ~/.config/tt-cli/config.toml (keep it mode 0600; it may hold a
secret)

  url              = "https://tt.example.com"
  cf_client_id     = "<id>.access"
  cf_client_secret = "<secret>"

Environment variables override the file:
  TT_URL               url
  TT_CF_CLIENT_ID      cf_client_id
  TT_CF_CLIENT_SECRET  cf_client_secret

The secret is never accepted as a flag.

Cloudflare Access
  If the instance sits behind Cloudflare Access, set both cf_client_id and
  cf_client_secret to a service token. They are sent as the
  CF-Access-Client-Id and CF-Access-Client-Secret headers on every request.
  When they are not set, no auth headers are sent (a plain local tt).

Check the setup with ` + "`tt-cli doctor`" + `.
`
