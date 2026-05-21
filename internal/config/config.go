// Package config parses CLI flags and resolves default data/database paths.
package config

import (
	"flag"
	"io"
	"os"
	"path/filepath"
)

// Config holds runtime configuration for the tt binary.
type Config struct {
	Port    int
	DataDir string
	DBPath  string
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

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	if cfg.DataDir == "" {
		cfg.DataDir = resolveDataDir()
	}
	if cfg.DBPath == "" {
		cfg.DBPath = filepath.Join(cfg.DataDir, "db.sqlite")
	}

	return cfg, nil
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
