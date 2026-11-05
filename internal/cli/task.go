package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/srliao/tt/internal/task"
	"github.com/srliao/tt/internal/ttclient"
)

func newTaskCmd(s *session) *cobra.Command {
	return group("task", "Read and change tasks",
		newTaskListCmd(s),
		newTaskGetCmd(s),
		newTaskCreateCmd(s),
		newTaskUpdateCmd(s),
		newTaskStateCmd(s),
		newTaskStageCmd(s, true),
		newTaskStageCmd(s, false),
		newTaskTagCmd(s, task.BulkTagOpAdd),
		newTaskTagCmd(s, task.BulkTagOpRemove),
	)
}

// taskWrite is the POST/PATCH /tasks body.
type taskWrite struct {
	Title   string   `json:"title"`
	Notes   string   `json:"notes"`
	DueDate *string  `json:"due_date"`
	Tags    []string `json:"tags"`
}

const fullTaskShape = `{id, title, notes, state, due_date, priority, staged_order,
 spawned_by_script_id, created_at, completed_at, cancelled_at, updated_at,
 tags: [string], blocked_by: [id], blocks: [id], blocked: bool}`

const compactTaskShape = `{id, title, state, due_date, tags: [string], blocked: bool, staged: bool}`

// compactTask is the default list row.
type compactTask struct {
	ID      int64      `json:"id"`
	Title   string     `json:"title"`
	State   task.State `json:"state"`
	DueDate *string    `json:"due_date"`
	Tags    []string   `json:"tags"`
	Blocked bool       `json:"blocked"`
	Staged  bool       `json:"staged"`
}

func compact(t task.Task) compactTask {
	tags := t.Tags
	if tags == nil {
		tags = []string{}
	}
	return compactTask{
		ID: t.ID, Title: t.Title, State: t.State, DueDate: t.DueDate,
		Tags: tags, Blocked: t.Blocked, Staged: t.StagedOrder != nil,
	}
}

// taskRows renders tasks compact, or whole when full is set.
func taskRows(tasks []task.Task, full bool) any {
	if full {
		return tasks
	}
	rows := make([]compactTask, 0, len(tasks))
	for _, t := range tasks {
		rows = append(rows, compact(t))
	}
	return rows
}

// taskList is the output of `task list` and `stage list`.
type taskList struct {
	Tasks      any    `json:"tasks"`
	NextCursor string `json:"next_cursor,omitempty"`
}

const defaultListLimit = 50

func newTaskListCmd(s *session) *cobra.Command {
	var ready, blocked, full bool
	var states, tags []string
	var search, cursor string
	limit := defaultListLimit
	cmd := &cobra.Command{
		Use:   "list [--ready|--blocked] [--state <state>]... [--tag <name>]... [--search <text>] [--limit N] [--cursor C] [--full]",
		Short: "List tasks (open tasks, compact rows, by default)",
		Long: `List tasks in priority order. Defaults to open (not_done) tasks, 50 per page.
--ready keeps only unblocked tasks, --blocked only blocked ones. --tag
filters to tasks carrying all given tags. When more rows remain,
next_cursor is set; pass it back with --cursor for the next page.

Output: {tasks: [row], next_cursor?: string}
  row (default): ` + compactTaskShape + `
  row (--full):  ` + fullTaskShape,
		Example: `  tt-cli task list --ready --tag work`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if ready && blocked {
				return usageErr("--ready and --blocked are mutually exclusive", nil)
			}
			if limit <= 0 {
				return usageErr("--limit must be positive", map[string]any{"value": limit})
			}
			offset := 0
			if cursor != "" {
				n, err := strconv.Atoi(cursor)
				if err != nil || n < 0 {
					return usageErr("invalid --cursor; pass back a next_cursor value", map[string]any{"value": cursor})
				}
				offset = n
			}
			if len(states) == 0 {
				states = []string{string(task.StateNotDone)}
			}
			q := url.Values{"state": states}
			if ready {
				q.Set("blocked", "false")
			}
			if blocked {
				q.Set("blocked", "true")
			}
			if len(tags) > 0 {
				if err := s.requireTags(cmd.Context(), tags); err != nil {
					return err
				}
				q.Set("tag_filter", "all:"+strings.Join(tags, ","))
			}
			if search != "" {
				q.Set("q", search)
			}
			// One extra row tells us whether another page exists.
			q.Set("limit", strconv.Itoa(limit+1))
			q.Set("offset", strconv.Itoa(offset))

			got, err := list[task.Task](cmd.Context(), s, http.MethodGet, "/tasks", q, nil)
			if err != nil {
				return err
			}
			out := taskList{}
			if len(got) > limit {
				got = got[:limit]
				out.NextCursor = strconv.Itoa(offset + limit)
			}
			out.Tasks = taskRows(got, full)
			return s.out(out)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&ready, "ready", false, "only unblocked tasks: no blocker is open or cancelled")
	f.BoolVar(&blocked, "blocked", false, "only blocked tasks")
	f.StringArrayVar(&states, "state", nil, "not_done|done|cancelled; repeatable (default not_done)")
	f.StringArrayVar(&tags, "tag", nil, "existing tag name; repeatable, all must match")
	f.StringVar(&search, "search", "", "substring match on title and notes")
	f.IntVar(&limit, "limit", defaultListLimit, "page size")
	f.StringVar(&cursor, "cursor", "", "next_cursor from the previous page")
	f.BoolVar(&full, "full", false, "full task rows (notes, timestamps, edges)")
	return cmd
}

func (s *session) getTask(ctx context.Context, id int64) (task.Task, error) {
	var out task.Task
	err := s.do(ctx, http.MethodGet, fmt.Sprintf("/tasks/%d", id), nil, nil, &out)
	return out, err
}

func newTaskGetCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Show one task in full",
		Long: `Show one task, always in full.

Output: ` + fullTaskShape,
		Example: `  tt-cli task get 42`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID("id", args[0])
			if err != nil {
				return err
			}
			t, err := s.getTask(cmd.Context(), id)
			if err != nil {
				return err
			}
			return s.out(t)
		},
	}
}

func newTaskCreateCmd(s *session) *cobra.Command {
	var title, notes, due string
	var tags []string
	cmd := &cobra.Command{
		Use:   "create --title <title> [--notes <text>] [--due YYYY-MM-DD] [--tag <name>]...",
		Short: "Create a task",
		Long: `Create a task. Tags must already exist (see ` + "`tt-cli tag list`" + `).

Output: ` + fullTaskShape,
		Example: `  tt-cli task create --title "Renew passport" --due 2026-12-01 --tag errand`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if title == "" {
				return usageErr("--title is required", map[string]any{"field": "title"})
			}
			body := taskWrite{Title: title, Notes: notes, Tags: tags}
			if due != "" {
				if err := checkDate("due", due); err != nil {
					return err
				}
				body.DueDate = &due
			}
			if err := s.requireTags(cmd.Context(), tags); err != nil {
				return err
			}
			var out task.Task
			if err := s.do(cmd.Context(), http.MethodPost, "/tasks", nil, body, &out); err != nil {
				return err
			}
			return s.out(out)
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "task title (required)")
	cmd.Flags().StringVar(&notes, "notes", "", "free-text notes")
	cmd.Flags().StringVar(&due, "due", "", "due date, YYYY-MM-DD")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "existing tag name; repeatable")
	return cmd
}

func newTaskUpdateCmd(s *session) *cobra.Command {
	var title, notes, due string
	var clearDue bool
	cmd := &cobra.Command{
		Use:   "update <id> [--title <title>] [--notes <text>] [--due YYYY-MM-DD | --clear-due]",
		Short: "Change a task's title, notes or due date",
		Long: `Change a task's title, notes or due date. Fields you don't pass are kept.
Use task state / stage / tag for the rest.

Output: ` + fullTaskShape,
		Example: `  tt-cli task update 42 --due 2026-12-15`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID("id", args[0])
			if err != nil {
				return err
			}
			fl := cmd.Flags()
			if !fl.Changed("title") && !fl.Changed("notes") && !fl.Changed("due") && !clearDue {
				return usageErr("nothing to update: pass --title, --notes, --due or --clear-due", nil)
			}
			if fl.Changed("due") && clearDue {
				return usageErr("--due and --clear-due are mutually exclusive", nil)
			}
			if fl.Changed("due") {
				if err := checkDate("due", due); err != nil {
					return err
				}
			}
			cur, err := s.getTask(cmd.Context(), id)
			if err != nil {
				return err
			}
			// PATCH replaces every field, tags included, so start from the
			// current task.
			body := taskWrite{Title: cur.Title, Notes: cur.Notes, DueDate: cur.DueDate, Tags: cur.Tags}
			if body.Tags == nil {
				body.Tags = []string{}
			}
			if fl.Changed("title") {
				body.Title = title
			}
			if fl.Changed("notes") {
				body.Notes = notes
			}
			if fl.Changed("due") {
				body.DueDate = &due
			}
			if clearDue {
				body.DueDate = nil
			}
			var out task.Task
			if err := s.do(cmd.Context(), http.MethodPatch, fmt.Sprintf("/tasks/%d", id), nil, body, &out); err != nil {
				return err
			}
			return s.out(out)
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "new title")
	cmd.Flags().StringVar(&notes, "notes", "", "new notes (\"\" clears)")
	cmd.Flags().StringVar(&due, "due", "", "new due date, YYYY-MM-DD")
	cmd.Flags().BoolVar(&clearDue, "clear-due", false, "remove the due date")
	return cmd
}

func newTaskStateCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "state <id> <not_done|done|cancelled>",
		Short: "Set a task's state",
		Long: `Set a task's state. Agents cancel tasks instead of deleting them.

Output: ` + fullTaskShape,
		Example: `  tt-cli task state 42 done`,
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID("id", args[0])
			if err != nil {
				return err
			}
			st := task.State(args[1])
			if !st.IsValid() {
				return validationErr("state must be not_done|done|cancelled", map[string]any{"value": args[1]})
			}
			var out task.Task
			if err := s.do(cmd.Context(), http.MethodPost, fmt.Sprintf("/tasks/%d/state", id), nil,
				map[string]string{"state": string(st)}, &out); err != nil {
				return err
			}
			return s.out(out)
		},
	}
}

func newTaskStageCmd(s *session, stage bool) *cobra.Command {
	use, short, method := "stage", "Add a task to the stage", http.MethodPost
	if !stage {
		use, short, method = "unstage", "Remove a task from the stage", http.MethodDelete
	}
	return &cobra.Command{
		Use:   use + " <id>",
		Short: short,
		Long: short + `.

Output: ` + fullTaskShape,
		Example: `  tt-cli task ` + use + ` 42`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID("id", args[0])
			if err != nil {
				return err
			}
			var out task.Task
			if err := s.do(cmd.Context(), method, fmt.Sprintf("/tasks/%d/stage", id), nil, nil, &out); err != nil {
				return err
			}
			return s.out(out)
		},
	}
}

func newTaskTagCmd(s *session, op task.BulkTagOp) *cobra.Command {
	use, short := "tag", "Attach existing tags to a task"
	if op == task.BulkTagOpRemove {
		use, short = "untag", "Detach tags from a task"
	}
	return &cobra.Command{
		Use:   use + " <id> <tag>...",
		Short: short,
		Long: short + `. Unknown tag names are an error.

Output: ` + fullTaskShape,
		Example: `  tt-cli task ` + use + ` 42 work urgent`,
		Args:    cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID("id", args[0])
			if err != nil {
				return err
			}
			names := args[1:]
			if err := s.requireTags(cmd.Context(), names); err != nil {
				return err
			}
			// bulk-tag reports a missing task as a constraint conflict, so
			// check existence first to get a not_found.
			if _, err := s.getTask(cmd.Context(), id); err != nil {
				return err
			}
			var out []task.Task
			body := map[string]any{"ids": []int64{id}, "op": string(op), "tags": names}
			if err := s.do(cmd.Context(), http.MethodPost, "/tasks/bulk-tag", nil, body, &out); err != nil {
				return err
			}
			if len(out) != 1 {
				return &Error{Code: ttclient.CodeInternal, Message: fmt.Sprintf("bulk-tag returned %d tasks, want 1", len(out)), Exit: ExitRemote}
			}
			return s.out(out[0])
		},
	}
}
