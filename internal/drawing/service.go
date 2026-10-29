package drawing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/srliao/tt/internal/db"
	sqlcgen "github.com/srliao/tt/internal/db/sqlc"
)

// sqliteTimeLayout matches the format SQLite's datetime('now') produces (UTC,
// second precision). RFC3339 is tried as a fallback.
const sqliteTimeLayout = "2006-01-02 15:04:05"

// Service is the drawing-domain API.
type Service interface {
	Create(ctx context.Context, name string, scene json.RawMessage) (Drawing, error)
	Get(ctx context.Context, id int64) (Drawing, error)
	List(ctx context.Context) ([]Summary, error)
	Update(ctx context.Context, id int64, in UpdateInput) (Drawing, error)
	Delete(ctx context.Context, id int64) error
}

// Impl is the concrete Service backed by a *db.Store.
type Impl struct {
	q *sqlcgen.Queries
}

// New constructs a Service bound to the supplied store.
func New(store *db.Store) *Impl {
	return &Impl{q: store.Queries()}
}

// Create inserts a drawing. The name is trimmed and must be non-empty. A nil
// scene starts the drawing empty; otherwise it must be a JSON object (used by
// import, so the drawing and its content land in one insert).
func (s *Impl) Create(ctx context.Context, name string, scene json.RawMessage) (Drawing, error) {
	name, err := validateName(name)
	if err != nil {
		return Drawing{}, err
	}
	stored := "{}"
	if scene != nil {
		v, err := validateScene(scene)
		if err != nil {
			return Drawing{}, err
		}
		stored = v
	}
	row, err := s.q.CreateDrawing(ctx, sqlcgen.CreateDrawingParams{Name: name, Scene: stored})
	if err != nil {
		return Drawing{}, fmt.Errorf("drawing: create: %w", err)
	}
	return rowToDrawing(row), nil
}

// Get returns the drawing including its scene. A missing id surfaces
// sql.ErrNoRows wrapped, which the HTTP layer maps to 404.
func (s *Impl) Get(ctx context.Context, id int64) (Drawing, error) {
	row, err := s.q.GetDrawing(ctx, id)
	if err != nil {
		return Drawing{}, fmt.Errorf("drawing: get %d: %w", id, err)
	}
	return rowToDrawing(row), nil
}

// List returns every drawing without its scene, most recently updated first.
func (s *Impl) List(ctx context.Context) ([]Summary, error) {
	rows, err := s.q.ListDrawings(ctx)
	if err != nil {
		return nil, fmt.Errorf("drawing: list: %w", err)
	}
	out := make([]Summary, 0, len(rows))
	for _, r := range rows {
		out = append(out, toSummary(r.ID, r.Name, r.CreatedAt, r.UpdatedAt))
	}
	return out, nil
}

// Update applies a partial update in one statement. Both fields are
// validated before anything is written, so a bad scene can't leave a
// half-applied rename behind.
func (s *Impl) Update(ctx context.Context, id int64, in UpdateInput) (Drawing, error) {
	var params sqlcgen.UpdateDrawingParams
	params.ID = id
	if in.Name != nil {
		name, err := validateName(*in.Name)
		if err != nil {
			return Drawing{}, err
		}
		params.Name = &name
	}
	if in.Scene != nil {
		scene, err := validateScene(in.Scene)
		if err != nil {
			return Drawing{}, err
		}
		params.Scene = &scene
	}
	row, err := s.q.UpdateDrawing(ctx, params)
	if err != nil {
		return Drawing{}, fmt.Errorf("drawing: update %d: %w", id, err)
	}
	return rowToDrawing(row), nil
}

// Delete removes a drawing.
func (s *Impl) Delete(ctx context.Context, id int64) error {
	if err := s.q.DeleteDrawing(ctx, id); err != nil {
		return fmt.Errorf("drawing: delete %d: %w", id, err)
	}
	return nil
}

// validateName trims the name and rejects it when empty.
func validateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("drawing: name is required")
	}
	return name, nil
}

// validateScene checks the scene is a JSON object and returns it trimmed.
func validateScene(scene json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(scene)
	if len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(trimmed) {
		return "", errors.New("drawing: scene must be a JSON object")
	}
	return string(trimmed), nil
}

func rowToDrawing(r sqlcgen.Drawing) Drawing {
	return Drawing{
		Summary: toSummary(r.ID, r.Name, r.CreatedAt, r.UpdatedAt),
		Scene:   json.RawMessage(r.Scene),
	}
}

func toSummary(id int64, name, createdAt, updatedAt string) Summary {
	return Summary{
		ID:        id,
		Name:      name,
		CreatedAt: parseSqliteTime(createdAt),
		UpdatedAt: parseSqliteTime(updatedAt),
	}
}

// parseSqliteTime accepts both SQLite's datetime('now') output and RFC3339.
func parseSqliteTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(sqliteTimeLayout, s); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC()
	}
	return time.Time{}
}

// Compile-time assertion that Impl satisfies the Service interface.
var _ Service = (*Impl)(nil)
