package config_test

import (
	"testing"

	"github.com/srliao/tt/internal/config"
)

func TestParseDefaults(t *testing.T) {
	cfg, err := config.Parse([]string{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.DataDir == "" {
		t.Errorf("DataDir is empty, want a resolved path")
	}
	if cfg.DBPath == "" {
		t.Errorf("DBPath is empty, want a resolved path")
	}
}

func TestParseFlags(t *testing.T) {
	cfg, err := config.Parse([]string{"--port", "9090", "--data-dir", "/tmp/tt"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Port != 9090 {
		t.Errorf("Port = %d, want 9090", cfg.Port)
	}
	if cfg.DataDir != "/tmp/tt" {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, "/tmp/tt")
	}
}

func TestParseDBOverride(t *testing.T) {
	cfg, err := config.Parse([]string{"--db", "/some/path/db.sqlite"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.DBPath != "/some/path/db.sqlite" {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, "/some/path/db.sqlite")
	}
}

func TestParseInvalidPort(t *testing.T) {
	_, err := config.Parse([]string{"--port", "abc"})
	if err == nil {
		t.Fatalf("Parse with invalid port returned nil error, want non-nil")
	}
}
