// Command tt is the local-only single-user task tracker binary.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/srliao/tt/internal/config"
	"github.com/srliao/tt/internal/db"
	"github.com/srliao/tt/internal/httpapi"
	"github.com/srliao/tt/internal/runtime"
	"github.com/srliao/tt/internal/scheduler"
	"github.com/srliao/tt/internal/script"
	"github.com/srliao/tt/internal/tag"
	"github.com/srliao/tt/internal/task"
	"github.com/srliao/tt/internal/web"
)

// Version is overridden at build time via -ldflags "-X main.Version=...".
var Version = "dev"

// BuiltAt is overridden at build time via -ldflags "-X main.BuiltAt=...".
var BuiltAt = ""

// shutdownTimeout caps how long the HTTP server has to finish in-flight
// requests after we receive SIGINT/SIGTERM. The scheduler then drains
// (synchronously) and the store closes; combined these complete a graceful
// shutdown well inside any reasonable supervisor's kill timer.
const shutdownTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "tt: %v\n", err)
		os.Exit(1)
	}
}

// run is the testable body of main. It returns an error on startup failure
// and nil after a clean shutdown.
func run() error {
	cfg, err := config.Parse(os.Args[1:])
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	logger.Info("starting tt",
		slog.String("version", Version),
		slog.Int("port", cfg.Port),
		slog.String("data_dir", cfg.DataDir),
		slog.String("db_path", cfg.DBPath),
		slog.String("timezone", cfg.Location.String()),
	)

	// SIGINT/SIGTERM cancel this context, which propagates into the
	// scheduler ticker/worker and aborts any in-flight DB calls.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Ensure the data directory exists. The config layer resolves the path
	// but does not create it; doing so here keeps the binary self-sufficient
	// on first boot.
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	store, err := db.Open(ctx, cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			logger.Error("close store", "err", err)
		}
	}()

	tasks := task.New(store)
	tags := tag.New(store)
	scripts := script.New(store)

	// cfg.Location decides when a calendar day starts for both the ctx.*
	// date helpers and schedule matching; the two must agree or a daily
	// script and the ctx.today() it calls would disagree about the date.
	runner := runtime.New(tasks, tags, scripts, logger, runtime.WithLocation(cfg.Location))
	sched := scheduler.New(runner, scripts, logger, scheduler.WithLocation(cfg.Location))
	if err := sched.Start(ctx); err != nil {
		return fmt.Errorf("start scheduler: %w", err)
	}
	defer sched.Stop()

	// Load the embedded SPA bundle. //go:embed always resolves (the package
	// ships a sentinel .gitkeep so the directive is happy on a clean
	// checkout), but a binary built without `just build` will contain only
	// that sentinel and respond 404 to SPA routes — intentional.
	distFS, err := web.Dist()
	if err != nil {
		return fmt.Errorf("load embedded dist: %w", err)
	}
	spaHandler := httpapi.NewSPAHandler(distFS)

	server := httpapi.New(
		tasks, tags, scripts, sched,
		httpapi.PingerFunc(store.DB().PingContext),
		httpapi.Options{
			Logger:   logger,
			Version:  Version,
			BuiltAt:  BuiltAt,
			Timezone: cfg.Location.String(),
			SPA:      spaHandler,
		},
	)

	httpSrv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           server.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", slog.String("addr", httpSrv.Addr))
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	// Wait for either a fatal listen error or a signal-driven shutdown.
	select {
	case err := <-serverErr:
		if err != nil {
			return fmt.Errorf("http listen: %w", err)
		}
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	// Graceful HTTP shutdown first so in-flight requests finish. We give it
	// a fresh context with a hard deadline; the parent context may have
	// already been cancelled by the signal.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http shutdown", "err", err)
	}

	// Scheduler.Stop is called via defer; store.Close likewise. Both run
	// before run() returns so the binary doesn't exit until everything is
	// flushed.
	logger.Info("shutdown complete")
	return nil
}
