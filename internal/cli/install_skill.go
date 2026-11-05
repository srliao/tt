package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/srliao/tt/internal/ttclient"
)

// skillMD only points back at the CLI: the help text is the manual, so the
// skill never goes stale when commands change.
const skillMD = `---
name: tt
description: Track, add, plan or organize tasks, todos and recurring reminders in the user's tt task tracker. Use when the user asks to capture a task, plan a goal, check what to work on, or set up something that repeats.
---

Use ` + "`tt-cli`" + ` for everything in tt.
Run ` + "`tt-cli help`" + ` first, and ` + "`tt-cli <command> --help`" + ` before using a command.
Never guess flags; the help text lists every one.
On any error, run ` + "`tt-cli doctor`" + ` and report what it says.
Scripts are saved disabled: tell the user to enable them in the tt web UI.
`

func newInstallSkillCmd(s *session) *cobra.Command {
	var dir string
	var force, printOnly bool
	cmd := &cobra.Command{
		Use:   "install-skill --dir <path> [--force] [--print]",
		Short: "Install the tt agent skill (SKILL.md)",
		Long: `Write <path>/tt/SKILL.md, an agent skill that points the agent at tt-cli
and its help. Fails with conflict if the file exists, unless --force.
--print writes the skill to stdout instead and touches nothing on disk.

Output: {path}
With --print: the SKILL.md markdown itself (not JSON).`,
		Example: `  tt-cli install-skill --dir ~/.claude/skills`,
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if dir == "" {
				return usageErr("--dir is required", map[string]any{"field": "dir"})
			}
			if printOnly {
				_, err := fmt.Fprint(s.Stdout, skillMD)
				return err
			}
			path := filepath.Join(dir, "tt", "SKILL.md")
			if !force {
				if _, err := os.Stat(path); err == nil {
					return &Error{
						Code:    ttclient.CodeConflict,
						Message: path + " already exists; pass --force to overwrite",
						Details: map[string]any{"path": path},
						Exit:    ExitValidation,
					}
				} else if !errors.Is(err, fs.ErrNotExist) {
					return usageErr("stat skill: "+err.Error(), map[string]any{"path": path})
				}
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return usageErr("create skill dir: "+err.Error(), map[string]any{"path": path})
			}
			if err := os.WriteFile(path, []byte(skillMD), 0o644); err != nil {
				return usageErr("write skill: "+err.Error(), map[string]any{"path": path})
			}
			return s.out(map[string]string{"path": path})
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "skills directory; the skill goes in <dir>/tt/SKILL.md (required)")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing SKILL.md")
	cmd.Flags().BoolVar(&printOnly, "print", false, "write the skill to stdout instead of disk")
	return cmd
}
