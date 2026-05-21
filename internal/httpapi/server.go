package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/srliao/tt/internal/script"
	"github.com/srliao/tt/internal/tag"
	"github.com/srliao/tt/internal/task"
)

// TaskService is the narrow consumer-side interface the HTTP layer needs
// from the task domain. Keeping this list explicit means the handler tests
// can ship a stub and the production *task.Impl satisfies it structurally.
type TaskService interface {
	Create(ctx context.Context, in task.CreateInput) (task.Task, error)
	Get(ctx context.Context, id int64) (task.Task, error)
	Update(ctx context.Context, id int64, in task.UpdateInput) (task.Task, error)
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, f task.FilterSort) ([]task.Task, error)
	SetState(ctx context.Context, id int64, st task.State) (task.Task, error)
	Stage(ctx context.Context, id int64) (task.Task, error)
	Unstage(ctx context.Context, id int64) (task.Task, error)
	ClearStage(ctx context.Context) error
	ClearFinishedFromStage(ctx context.Context) error
	ReorderMain(ctx context.Context, id int64, beforeID, afterID *int64) (task.Task, error)
	ReorderStage(ctx context.Context, id int64, beforeID, afterID *int64) (task.Task, error)
	ByScript(ctx context.Context, scriptID int64, limit, offset int) ([]task.Task, error)
	SetTagsByID(ctx context.Context, taskID int64, tagIDs []int64) error
}

// TagService is the narrow consumer-side interface for tags. The HTTP layer
// needs CRUD plus Resolve (for tag-name filters on GET /tasks and tag
// attachment on POST/PATCH /tasks).
type TagService interface {
	Create(ctx context.Context, name string) (tag.Tag, error)
	Rename(ctx context.Context, id int64, name string) (tag.Tag, error)
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context) ([]tag.Tag, error)
	Resolve(ctx context.Context, names []string, autoCreate bool) ([]int64, error)
}

// ScriptService is the narrow consumer-side interface for scripts. Includes
// the run-lifecycle methods the manual /scripts/:id/run handler uses to
// pre-create the run row before enqueuing.
type ScriptService interface {
	Create(ctx context.Context, in script.CreateInput) (script.Script, error)
	Update(ctx context.Context, id int64, in script.UpdateInput) (script.Script, error)
	Get(ctx context.Context, id int64) (script.Script, error)
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context) ([]script.Script, error)
	StartRun(ctx context.Context, scriptID int64, trigger script.Trigger) (script.Run, error)
	FinishRun(ctx context.Context, runID int64, status script.RunStatus, errMsg string, spawnedIDs []int64) error
	GetRun(ctx context.Context, runID int64) (script.Run, error)
	GetLogs(ctx context.Context, runID int64) ([]script.Log, error)
	ListRunsByScript(ctx context.Context, scriptID int64, limit, offset int) ([]script.Run, error)
	ListAllRuns(ctx context.Context, limit, offset int) ([]script.Run, error)
}

// ManualRunEnqueuer is the slice of the scheduler the HTTP layer uses to
// kick off manual runs. *scheduler.Scheduler satisfies this structurally.
type ManualRunEnqueuer interface {
	EnqueueManual(scriptID, runID int64) error
}

// ErrSchedulerBusy is the sentinel ManualRunEnqueuer implementations should
// return when the queue is full. Exposed in this package so the HTTP layer
// can map it to 503 without importing the scheduler package.
//
// Implementations that return a different error value will be classified as
// 500. The scheduler package re-exports its own ErrSchedulerBusy; the wiring
// code in cmd/tt simply uses the scheduler value directly.
var ErrSchedulerBusy = errSchedulerBusy{}

type errSchedulerBusy struct{}

func (errSchedulerBusy) Error() string { return "scheduler busy" }

// Pinger is the narrow interface health uses to verify DB connectivity. The
// production wiring passes an adapter around *sql.DB.PingContext.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Options bundles non-service-dependency configuration for New.
type Options struct {
	// Logger is the destination for middleware log output and panic traces.
	// Required.
	Logger *slog.Logger
	// Version is the build version string returned by /version.
	Version string
	// BuiltAt is the build timestamp string returned by /version. Optional.
	BuiltAt string
	// SPA is the handler that serves the front-end SPA. Phase 09 wires in
	// the embed.FS; phase 06 tests can pass a plain "404 not found" handler.
	SPA http.Handler
}

// Server is the assembled HTTP application. Build with New, mount via Routes.
type Server struct {
	tasks    TaskService
	tags     TagService
	scripts  ScriptService
	enqueuer ManualRunEnqueuer
	pinger   Pinger
	logger   *slog.Logger
	version  string
	builtAt  string
	spa      http.Handler
}

// New constructs a Server bound to the supplied services and options. The
// returned value is safe for concurrent use as the underlying services are.
func New(
	tasks TaskService,
	tags TagService,
	scripts ScriptService,
	enqueuer ManualRunEnqueuer,
	pinger Pinger,
	opts Options,
) *Server {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	spa := opts.SPA
	if spa == nil {
		spa = http.HandlerFunc(spaNotFound)
	}
	return &Server{
		tasks:    tasks,
		tags:     tags,
		scripts:  scripts,
		enqueuer: enqueuer,
		pinger:   pinger,
		logger:   logger,
		version:  opts.Version,
		builtAt:  opts.BuiltAt,
		spa:      spa,
	}
}

// Routes returns the chi router with every endpoint mounted. Call once at
// startup and pass the result to http.Server.Handler.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(requestID)
	r.Use(slogLog(s.logger))
	r.Use(recoverPanic(s.logger))

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", s.handleHealth)
		r.Get("/version", s.handleVersion)
		s.mountTaskRoutes(r)
		s.mountStageRoutes(r)
		s.mountTagRoutes(r)
		s.mountScriptRoutes(r)
		s.mountRunRoutes(r)
	})

	r.Handle("/*", s.spa)
	return r
}

// spaNotFound is the fallback used when no SPA handler is supplied (phase 06
// development; phase 09 swaps in the embed.FS-backed handler). Keeping the
// behavior here means tests that exercise Server.Routes without a real SPA
// fixture still see consistent 404 envelopes.
func spaNotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, CodeNotFound, "not found", nil)
}

// PingerFunc adapts a plain function into the Pinger interface so callers can
// pass *sql.DB.PingContext without writing a wrapper struct.
type PingerFunc func(ctx context.Context) error

// Ping satisfies the Pinger interface.
func (f PingerFunc) Ping(ctx context.Context) error { return f(ctx) }

// pingTimeout caps how long the health handler will wait for the DB. Long
// enough to swallow a normal hiccup, short enough that a wedged DB doesn't
// pile up health check requests.
const pingTimeout = 2 * time.Second
