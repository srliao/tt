package task

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	sqlcgen "github.com/srliao/tt/internal/db/sqlc"
)

// NormalizeProject trims and lowercases a project name. Nil or blank means no
// project and returns nil. Names starting with @ (reserved for sentinels such
// as NoProjectToken) or containing a comma (the filter separator) are
// rejected.
func NormalizeProject(in *string) (*string, error) {
	if in == nil {
		return nil, nil
	}
	name := strings.ToLower(strings.TrimSpace(*in))
	if name == "" {
		return nil, nil
	}
	if strings.HasPrefix(name, "@") {
		return nil, fmt.Errorf("task: invalid project %q: reserved prefix @", name)
	}
	if strings.Contains(name, ",") {
		return nil, fmt.Errorf("task: invalid project %q: must not contain a comma", name)
	}
	return &name, nil
}

// SetProject sets project on every task in ids in one transaction; nil or
// blank clears it. An unknown id fails the whole call with sql.ErrNoRows.
// The reloaded tasks come back in ids order.
func (s *Impl) SetProject(ctx context.Context, ids []int64, project *string) ([]Task, error) {
	if len(ids) == 0 {
		return nil, errors.New("task: ids is required")
	}
	name, err := NormalizeProject(project)
	if err != nil {
		return nil, err
	}
	now := s.now.Stamp()
	err = s.inTx(ctx, "set project", func(q *sqlcgen.Queries) error {
		for _, id := range ids {
			if _, err := q.SetTaskProject(ctx, sqlcgen.SetTaskProjectParams{Project: name, UpdatedAt: now, ID: id}); err != nil {
				return fmt.Errorf("task: set project %d: %w", id, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.getMany(ctx, ids)
}

// ListProjects returns every project some task names, sorted by name.
func (s *Impl) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := s.q.ListProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("task: list projects: %w", err)
	}
	out := make([]Project, 0, len(rows))
	for _, r := range rows {
		out = append(out, Project{Name: r.Name, OpenCount: r.OpenCount, TotalCount: r.TotalCount})
	}
	return out, nil
}

// RenameProject moves every task in project from to project to, merging the
// two when to is already in use, and returns to with its new counts. An
// unused from fails with sql.ErrNoRows.
func (s *Impl) RenameProject(ctx context.Context, from, to string) (Project, error) {
	fromName, err := NormalizeProject(&from)
	if err != nil {
		return Project{}, err
	}
	toName, err := NormalizeProject(&to)
	if err != nil {
		return Project{}, err
	}
	if fromName == nil {
		return Project{}, errors.New("task: from is required")
	}
	if toName == nil {
		return Project{}, errors.New("task: to is required")
	}
	var row sqlcgen.GetProjectRow
	err = s.inTx(ctx, "rename project", func(q *sqlcgen.Queries) error {
		n, err := q.RenameProject(ctx, sqlcgen.RenameProjectParams{To: toName, UpdatedAt: s.now.Stamp(), From: fromName})
		if err != nil {
			return fmt.Errorf("task: rename project: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("task: rename project %q: %w", *fromName, sql.ErrNoRows)
		}
		row, err = q.GetProject(ctx, toName)
		if err != nil {
			return fmt.Errorf("task: rename project read %q: %w", *toName, err)
		}
		return nil
	})
	if err != nil {
		return Project{}, err
	}
	return Project{Name: row.Name, OpenCount: row.OpenCount, TotalCount: row.TotalCount}, nil
}
