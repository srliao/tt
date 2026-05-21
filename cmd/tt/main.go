// Command tt is the local-only single-user task tracker binary.
package main

import (
	"log/slog"
	"os"

	"github.com/srliao/tt/internal/config"
)

// Version is overridden at build time via -ldflags "-X main.Version=...".
var Version = "dev"

func main() {
	cfg, err := config.Parse(os.Args[1:])
	if err != nil {
		os.Stderr.WriteString("config error: " + err.Error() + "\n")
		os.Exit(2)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	logger.Info("starting tt",
		slog.String("version", Version),
		slog.Int("port", cfg.Port),
		slog.String("data_dir", cfg.DataDir),
		slog.String("db_path", cfg.DBPath),
	)
	logger.Info("nothing to do yet; exiting")
}
