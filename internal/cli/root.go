// Package cli implements tt-cli, the agent-facing command line for a tt
// instance. stdout carries JSON only; errors go to stderr as
// {code, message, details} with the exit codes in errors.go.
package cli

import (
	"context"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/srliao/tt/internal/ttclient"
)

// App holds the process boundary so tests can drive Run in-process.
type App struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Getenv func(string) string
	// ConfigPath overrides DefaultConfigPath.
	ConfigPath string
	// HTTP overrides the client's transport settings when non-nil.
	HTTP *http.Client
	// Version is the CLI build version.
	Version string
	// Now is the CLI's clock (snapshot pulled_at, simulate's default range).
	// Defaults to time.Now.
	Now func() time.Time
}

// session is what every command closes over: the App plus the lazily loaded
// config and client, so commands that never touch the network never read
// the config file.
type session struct {
	App
	cfg    *Config
	client *ttclient.Client
}

func (s *session) config() (Config, error) {
	if s.cfg != nil {
		return *s.cfg, nil
	}
	cfg, err := loadConfig(s.ConfigPath, s.Getenv)
	if err != nil {
		return cfg, err
	}
	s.cfg = &cfg
	return cfg, nil
}

func (s *session) api() (*ttclient.Client, error) {
	if s.client != nil {
		return s.client, nil
	}
	cfg, err := s.config()
	if err != nil {
		return nil, err
	}
	if cfg.URL == "" {
		return nil, &Error{
			Code:    "config_missing",
			Message: "no server url configured: set url in the config file or TT_URL",
			Details: map[string]any{"path": cfg.Path},
			Exit:    ExitUsage,
		}
	}
	c := ttclient.New(cfg.URL, cfg.CFClientID, cfg.CFClientSecret)
	if s.HTTP != nil {
		c.HTTP = s.HTTP
	}
	s.client = c
	return c, nil
}

// out writes v as JSON to stdout.
func (s *session) out(v any) error {
	return writeJSON(s.Stdout, v)
}

// Main runs tt-cli against the real process environment and returns the
// exit code.
func Main(version string) int {
	return Run(context.Background(), App{
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Getenv:  os.Getenv,
		Version: version,
	}, os.Args[1:])
}

// Run executes one tt-cli invocation and returns its exit code.
func Run(ctx context.Context, app App, args []string) int {
	if app.Getenv == nil {
		app.Getenv = os.Getenv
	}
	if app.ConfigPath == "" {
		app.ConfigPath = DefaultConfigPath()
	}
	if app.Stdin == nil {
		app.Stdin = os.Stdin
	}
	if app.Now == nil {
		app.Now = time.Now
	}
	s := &session{App: app}
	root := newRootCmd(s)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		e := asError(err)
		writeError(app.Stderr, e)
		return e.Exit
	}
	return ExitOK
}

func newRootCmd(s *session) *cobra.Command {
	root := &cobra.Command{
		Use:           "tt-cli",
		Short:         "Agent-facing CLI for a tt task tracker instance",
		Version:       s.Version,
		SilenceErrors: true,
		SilenceUsage:  true,
		// A bare `tt-cli` prints help instead of erroring.
		Run: func(cmd *cobra.Command, _ []string) { _ = cmd.Help() },
	}
	root.SetOut(s.Stdout)
	root.SetErr(s.Stderr)
	root.SetIn(s.Stdin)
	root.CompletionOptions.DisableDefaultCmd = true

	root.AddCommand(
		newTaskCmd(s),
		newDepsCmd(s),
		newStageCmd(s),
		newTagCmd(s),
		newPlanCmd(s),
		newScriptCmd(s),
		newSnapshotCmd(s),
		newDoctorCmd(s),
	)
	return root
}

// group builds a parent command that only hosts subcommands. Running it
// bare (or with an unknown subcommand) is a usage error.
func group(use, short string, children ...*cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return usageErr("missing subcommand; run `tt-cli "+cmd.Name()+" --help`", nil)
		},
	}
	cmd.AddCommand(children...)
	return cmd
}
