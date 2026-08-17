// Package config parses CLI flags and resolves default data/database paths.
package config

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	// Compiles the IANA time zone database into the binary so
	// time.LoadLocation resolves named zones even on hosts without a system
	// tzdata package — notably the alpine-based container image.
	_ "time/tzdata"
)

// Config holds runtime configuration for the tt binary.
type Config struct {
	Port    int
	DataDir string
	DBPath  string

	// Timezone is the raw zone name as supplied by flag or environment
	// ("" when nothing was set). Location is its resolved form and is
	// never nil — it defaults to time.UTC.
	//
	// This zone decides when a calendar day starts for everything that
	// reasons about days rather than instants: daily/weekly/monthly
	// schedule matching and the ctx.* date helpers. Stored timestamps
	// remain UTC.
	Timezone string
	Location *time.Location
}

// Parse parses the given CLI arguments and returns a populated Config.
// It resolves defaults for DataDir and DBPath when those flags are empty.
func Parse(args []string) (Config, error) {
	fs := flag.NewFlagSet("tt", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var cfg Config
	fs.IntVar(&cfg.Port, "port", 8080, "HTTP listen port")
	fs.StringVar(&cfg.DataDir, "data-dir", "", "data directory (defaults to $XDG_DATA_HOME/tt or $HOME/.local/share/tt)")
	fs.StringVar(&cfg.DBPath, "db", "", "SQLite database path (defaults to <data-dir>/db.sqlite)")
	fs.StringVar(&cfg.Timezone, "timezone", "", "IANA time zone deciding when a day starts, e.g. America/New_York (defaults to $TT_TIMEZONE, $TZ, then UTC)")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	if cfg.DataDir == "" {
		cfg.DataDir = resolveDataDir()
	}
	if cfg.DBPath == "" {
		cfg.DBPath = filepath.Join(cfg.DataDir, "db.sqlite")
	}

	if cfg.Timezone == "" {
		cfg.Timezone = resolveTimezone()
	}
	loc, err := loadLocation(cfg.Timezone)
	if err != nil {
		return Config{}, err
	}
	cfg.Location = loc

	return cfg, nil
}

// resolveTimezone returns the zone name from the environment, preferring the
// app-specific TT_TIMEZONE over the conventional TZ so a container can carry
// a system TZ without silently deciding when tt's day starts.
func resolveTimezone() string {
	if tz := os.Getenv("TT_TIMEZONE"); tz != "" {
		return tz
	}
	return os.Getenv("TZ")
}

// loadLocation resolves an IANA zone name, defaulting to UTC when empty. An
// unrecognised name is a hard error rather than a silent UTC fallback: a
// typo'd zone that quietly reverts to UTC is exactly the failure this
// setting exists to prevent.
func loadLocation(name string) (*time.Location, error) {
	if name == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("unknown time zone %q: %w", name, err)
	}
	return loc, nil
}

// resolveDataDir returns the default data directory using, in order:
// $XDG_DATA_HOME/tt, $HOME/.local/share/tt, or ".tt-data".
func resolveDataDir() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "tt")
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".local", "share", "tt")
	}
	return ".tt-data"
}
