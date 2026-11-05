package cli

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/srliao/tt/internal/tag"
)

// parseID parses a positional task/tag id.
func parseID(name, raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, usageErr(name+" must be a positive integer", map[string]any{"value": raw})
	}
	return id, nil
}

// checkDate enforces YYYY-MM-DD; relative dates are deliberately rejected.
func checkDate(flag, raw string) error {
	if _, err := time.Parse(time.DateOnly, raw); err != nil {
		return validationErr(flag+" must be a YYYY-MM-DD date", map[string]any{"field": flag, "value": raw})
	}
	return nil
}

// requireTags errors when any name is not an existing tag. The server
// auto-creates unknown tags on task writes; the CLI refuses so a typo never
// mints a new tag. details.existing lists every tag name.
func (s *session) requireTags(ctx context.Context, names []string) error {
	if len(names) == 0 {
		return nil
	}
	c, err := s.api()
	if err != nil {
		return err
	}
	var tags []tag.Tag
	if _, err := c.Do(ctx, http.MethodGet, "/tags", nil, nil, &tags); err != nil {
		return err
	}
	existing := make([]string, 0, len(tags))
	for _, t := range tags {
		existing = append(existing, t.Name)
	}
	var unknown []string
	for _, n := range names {
		if !slices.Contains(existing, strings.ToLower(strings.TrimSpace(n))) {
			unknown = append(unknown, n)
		}
	}
	if len(unknown) > 0 {
		return validationErr("unknown tags: "+strings.Join(unknown, ", ")+" (create them with `tt-cli tag create`)",
			map[string]any{"unknown": unknown, "existing": existing})
	}
	return nil
}
