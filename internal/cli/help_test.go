package cli_test

import (
	"strings"
	"testing"

	"github.com/srliao/tt/internal/runtime/docs"
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
		"doctor", "install-skill",
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

func TestHelpTopics_EachPrints(t *testing.T) {
	f := newFixture(t)
	for topic, want := range map[string][]string{
		"":         {"tt-cli help plans", "Ready", "plan check", "script simulate"},
		"overview": {"tt-cli help plans", "Ready", "plan check", "script simulate"},
		"plans":    {`"deps_remove"`, `"ref"`, `"ids"`},
		"scripts":  {"weekly:<weekday>", "monthly:<n|last>", "--complete-after", "disabled"},
		"errors":   {"validation_failed", "not_found", "version_mismatch", "tt-cli doctor"},
		"config":   {"~/.config/tt-cli/config.toml", "TT_CF_CLIENT_SECRET", "CF-Access-Client-Id", "https://tt.example.com"},
	} {
		args := []string{"help"}
		if topic != "" {
			args = append(args, topic)
		}
		r := f.run(args...)
		if r.code != 0 {
			t.Fatalf("help %s: exit %d, stderr %s", topic, r.code, r.stderr)
		}
		for _, w := range want {
			if !strings.Contains(r.stdout, w) {
				t.Errorf("help %s missing %q", topic, w)
			}
		}
	}
}

func TestHelpScripts_IncludesCtxCheatsheet(t *testing.T) {
	r := newFixture(t).run("help", "scripts")
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %s", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, docs.CtxCheatsheet) {
		t.Errorf("help scripts does not contain the ctx cheatsheet:\n%s", r.stdout)
	}
	if !strings.Contains(r.stdout, "## ctx.queueTask(") {
		t.Errorf("cheatsheet entry missing:\n%s", r.stdout)
	}
}

func TestHelp_CommandNameStillShowsCommandHelp(t *testing.T) {
	r := newFixture(t).run("help", "task", "list")
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %s", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, "tt-cli task list") {
		t.Errorf("help task list did not show command help:\n%s", r.stdout)
	}
}

func TestHelp_UnknownTopicIsUsageError(t *testing.T) {
	e := newFixture(t).run("help", "nope").fails(t, 1, "usage")
	if e.Details["topics"] == nil {
		t.Errorf("details should list topics: %v", e.Details)
	}
}
