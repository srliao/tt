package cli_test

import (
	"strings"
	"testing"
)

// Each command's --help is part of the agent's manual: a description, its
// flags, one example and the output JSON shape.
func TestCommandHelp_HasExampleAndOutputShape(t *testing.T) {
	f := newFixture(t)
	for _, path := range []string{
		"task list", "task get", "task create", "task update", "task state",
		"task stage", "task unstage", "task tag", "task untag",
		"deps add", "deps rm", "deps batch", "deps graph",
		"stage list", "stage reorder", "stage clear-finished",
		"tag list", "tag create",
		"doctor",
	} {
		args := append(strings.Fields(path), "--help")
		r := f.run(args...)
		if r.code != 0 {
			t.Fatalf("%s --help: exit %d, stderr %s", path, r.code, r.stderr)
		}
		for _, want := range []string{"Usage:", "Examples:", "Output:", "tt-cli " + path} {
			if !strings.Contains(r.stdout, want) {
				t.Errorf("%s --help missing %q:\n%s", path, want, r.stdout)
			}
		}
	}
}
