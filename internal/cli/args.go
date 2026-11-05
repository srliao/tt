package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
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

// readInput returns the contents of path, or stdin when path is "" or "-".
// Complex bodies (dep batches, plans, script code) come in this way.
func (s *session) readInput(path string) ([]byte, error) {
	var (
		b   []byte
		err error
	)
	if path == "" || path == "-" {
		b, err = io.ReadAll(s.Stdin)
	} else {
		b, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, usageErr("read input: "+err.Error(), map[string]any{"path": path})
	}
	return b, nil
}

// decodeDoc strictly decodes a JSON input document into v.
func decodeDoc(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return validationErr("invalid JSON document: "+err.Error(), nil)
	}
	return nil
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
		if !slices.Contains(existing, tag.NormalizeName(n)) {
			unknown = append(unknown, n)
		}
	}
	if len(unknown) > 0 {
		return validationErr("unknown tags: "+strings.Join(unknown, ", ")+" (create them with `tt-cli tag create`)",
			map[string]any{"unknown": unknown, "existing": existing})
	}
	return nil
}
