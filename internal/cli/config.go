package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the resolved connection settings. Source records where each
// field came from ("env", "file" or "" when unset) so doctor can report it.
type Config struct {
	URL            string            `toml:"url"`
	CFClientID     string            `toml:"cf_client_id"`
	CFClientSecret string            `toml:"cf_client_secret"`
	Path           string            `toml:"-"`
	FileFound      bool              `toml:"-"`
	FileMode       fs.FileMode       `toml:"-"`
	Source         map[string]string `toml:"-"`
}

// DefaultConfigPath is ~/.config/tt-cli/config.toml.
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "tt-cli", "config.toml")
	}
	return filepath.Join(home, ".config", "tt-cli", "config.toml")
}

// loadConfig reads the file at path (a missing file is fine) and lets the
// TT_URL, TT_CF_CLIENT_ID and TT_CF_CLIENT_SECRET env vars override it.
func loadConfig(path string, getenv func(string) string) (Config, error) {
	cfg := Config{Path: path, Source: map[string]string{}}
	info, err := os.Stat(path)
	switch {
	case err == nil:
		if _, err := toml.DecodeFile(path, &cfg); err != nil {
			return cfg, &Error{Code: CodeConfigInvalid, Message: "parse config: " + err.Error(), Details: map[string]any{"path": path}, Exit: ExitUsage}
		}
		cfg.FileFound = true
		cfg.FileMode = info.Mode().Perm()
	case !errors.Is(err, fs.ErrNotExist):
		return cfg, &Error{Code: CodeConfigInvalid, Message: "read config: " + err.Error(), Details: map[string]any{"path": path}, Exit: ExitUsage}
	}

	for _, f := range []struct {
		key, env string
		dst      *string
	}{
		{"url", "TT_URL", &cfg.URL},
		{"cf_client_id", "TT_CF_CLIENT_ID", &cfg.CFClientID},
		{"cf_client_secret", "TT_CF_CLIENT_SECRET", &cfg.CFClientSecret},
	} {
		if v := getenv(f.env); v != "" {
			*f.dst = v
			cfg.Source[f.key] = "env"
		} else if *f.dst != "" {
			cfg.Source[f.key] = "file"
		}
	}
	return cfg, nil
}
