package config_test

import (
	"time"
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
	if cfg.DBPath != "/tmp/tt/db.sqlite" {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, "/tmp/tt/db.sqlite")
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

func TestParseTimezoneDefaultsToUTC(t *testing.T) {
	t.Setenv("TT_TIMEZONE", "")
	t.Setenv("TZ", "")
	cfg, err := config.Parse([]string{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Location != time.UTC {
		t.Errorf("Location = %v, want UTC", cfg.Location)
	}
}

func TestParseTimezoneFlag(t *testing.T) {
	cfg, err := config.Parse([]string{"--timezone", "America/New_York"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Location.String() != "America/New_York" {
		t.Errorf("Location = %q, want America/New_York", cfg.Location)
	}
}

func TestParseTimezoneFlagBeatsEnv(t *testing.T) {
	t.Setenv("TT_TIMEZONE", "Asia/Tokyo")
	cfg, err := config.Parse([]string{"--timezone", "America/New_York"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Location.String() != "America/New_York" {
		t.Errorf("Location = %q, want America/New_York", cfg.Location)
	}
}

func TestParseTimezoneFromTTEnv(t *testing.T) {
	t.Setenv("TT_TIMEZONE", "America/New_York")
	t.Setenv("TZ", "Asia/Tokyo")
	cfg, err := config.Parse([]string{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Location.String() != "America/New_York" {
		t.Errorf("Location = %q, want America/New_York (TT_TIMEZONE beats TZ)", cfg.Location)
	}
}

func TestParseTimezoneFromTZEnv(t *testing.T) {
	t.Setenv("TT_TIMEZONE", "")
	t.Setenv("TZ", "America/New_York")
	cfg, err := config.Parse([]string{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Location.String() != "America/New_York" {
		t.Errorf("Location = %q, want America/New_York", cfg.Location)
	}
}

func TestParseInvalidTimezone(t *testing.T) {
	_, err := config.Parse([]string{"--timezone", "Not/AZone"})
	if err == nil {
		t.Fatalf("Parse with invalid timezone returned nil error, want non-nil")
	}
}
