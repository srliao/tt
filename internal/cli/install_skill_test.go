package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readSkill(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "tt", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInstallSkill_WritesSkillFile(t *testing.T) {
	dir := t.TempDir()
	var out struct {
		Path string `json:"path"`
	}
	newFixture(t).run("install-skill", "--dir", dir).ok(t, &out)

	want := filepath.Join(dir, "tt", "SKILL.md")
	if out.Path != want {
		t.Errorf("path = %q, want %q", out.Path, want)
	}
	body := readSkill(t, dir)
	if !strings.HasPrefix(body, "---\nname: tt\ndescription: ") {
		t.Errorf("missing frontmatter:\n%s", body)
	}
	for _, w := range []string{"tt-cli help", "--help", "tt-cli doctor", "enable", "UI", "projects"} {
		if !strings.Contains(body, w) {
			t.Errorf("skill body missing %q:\n%s", w, body)
		}
	}
}

func TestInstallSkill_RefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tt", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	newFixture(t).run("install-skill", "--dir", dir).fails(t, 3, "conflict")
	if got := readSkill(t, dir); got != "mine" {
		t.Errorf("file was overwritten: %q", got)
	}
}

func TestInstallSkill_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tt", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	newFixture(t).run("install-skill", "--dir", dir, "--force").ok(t, nil)
	if got := readSkill(t, dir); !strings.Contains(got, "name: tt") {
		t.Errorf("file not replaced: %q", got)
	}
}

func TestInstallSkill_PrintWritesNothing(t *testing.T) {
	dir := t.TempDir()
	r := newFixture(t).run("install-skill", "--dir", dir, "--print")
	r.ok(t, nil)
	if !strings.HasPrefix(r.stdout, "---\nname: tt\n") {
		t.Errorf("stdout is not the skill:\n%s", r.stdout)
	}
	if json.Valid([]byte(r.stdout)) {
		t.Errorf("--print should emit the markdown, not JSON")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("--print wrote to disk: %v", entries)
	}
}

func TestInstallSkill_DirRequired(t *testing.T) {
	newFixture(t).run("install-skill").fails(t, 1, "usage")
}
