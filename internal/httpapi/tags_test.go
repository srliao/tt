package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/srliao/tt/internal/tag"
	"github.com/srliao/tt/internal/task"
)

func decodeTags(t *testing.T, resp *http.Response) []tag.Tag {
	t.Helper()
	var out []tag.Tag
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode tags: %v", err)
	}
	return out
}

func decodeTagSingle(t *testing.T, resp *http.Response) tag.Tag {
	t.Helper()
	var out tag.Tag
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode tag: %v", err)
	}
	return out
}

func TestTags_CreateListRenameDelete(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)

	// Create
	resp := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/tags", map[string]any{"name": "work"})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create status = %d, body = %s", resp.StatusCode, string(body))
	}
	created := decodeTagSingle(t, resp)
	if created.Name != "work" {
		t.Fatalf("name = %q", created.Name)
	}

	// List
	resp2 := doJSON(t, http.MethodGet, fx.server.URL+"/api/v1/tags", nil)
	defer func() { _ = resp2.Body.Close() }()
	tags := decodeTags(t, resp2)
	if len(tags) != 1 || tags[0].ID != created.ID {
		t.Fatalf("list = %+v", tags)
	}

	// Rename
	resp3 := doJSON(t, http.MethodPatch, fmt.Sprintf("%s/api/v1/tags/%d", fx.server.URL, created.ID),
		map[string]any{"name": "career"})
	defer func() { _ = resp3.Body.Close() }()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("rename status = %d", resp3.StatusCode)
	}
	renamed := decodeTagSingle(t, resp3)
	if renamed.Name != "career" {
		t.Fatalf("renamed name = %q", renamed.Name)
	}

	// Delete
	resp4 := doJSON(t, http.MethodDelete, fmt.Sprintf("%s/api/v1/tags/%d", fx.server.URL, created.ID), nil)
	defer func() { _ = resp4.Body.Close() }()
	if resp4.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", resp4.StatusCode)
	}

	resp5 := doJSON(t, http.MethodGet, fx.server.URL+"/api/v1/tags", nil)
	defer func() { _ = resp5.Body.Close() }()
	tags2 := decodeTags(t, resp5)
	if len(tags2) != 0 {
		t.Fatalf("after delete tags = %+v", tags2)
	}
}

func TestTags_CreateEmptyNameIs400(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	resp := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/tags", map[string]any{"name": "  "})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

// TestTags_CreateReservedAtPrefixIs400 verifies that names beginning with the
// reserved `@` sentinel prefix (used by tokens like `@untagged` in the
// tag_filter URL schema) cannot be persisted as real tags.
func TestTags_CreateReservedAtPrefixIs400(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	resp := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/tags", map[string]any{"name": "@foo"})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestTags_RenameDuplicateIs409(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	ctx := context.Background()
	t1, err := fx.tags.Create(ctx, "alpha")
	if err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	if _, err := fx.tags.Create(ctx, "beta"); err != nil {
		t.Fatalf("create beta: %v", err)
	}

	resp := doJSON(t, http.MethodPatch, fmt.Sprintf("%s/api/v1/tags/%d", fx.server.URL, t1.ID),
		map[string]any{"name": "beta"})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, string(body))
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if env.Error.Code != "conflict" {
		t.Fatalf("code = %q", env.Error.Code)
	}
}

func TestTags_ListWithCounts(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	ctx := context.Background()

	tagWork, err := fx.tags.Create(ctx, "work")
	if err != nil {
		t.Fatalf("create work: %v", err)
	}
	tagHome, err := fx.tags.Create(ctx, "home")
	if err != nil {
		t.Fatalf("create home: %v", err)
	}
	// "lonely" gets created but never attached — must come back with count 0.
	tagLonely, err := fx.tags.Create(ctx, "lonely")
	if err != nil {
		t.Fatalf("create lonely: %v", err)
	}

	t1, err := fx.tasks.Create(ctx, task.CreateInput{Title: "t1"})
	if err != nil {
		t.Fatalf("create task 1: %v", err)
	}
	t2, err := fx.tasks.Create(ctx, task.CreateInput{Title: "t2"})
	if err != nil {
		t.Fatalf("create task 2: %v", err)
	}
	if err := fx.tasks.SetTagsByID(ctx, t1.ID, []int64{tagWork.ID, tagHome.ID}); err != nil {
		t.Fatalf("attach to t1: %v", err)
	}
	if err := fx.tasks.SetTagsByID(ctx, t2.ID, []int64{tagWork.ID}); err != nil {
		t.Fatalf("attach to t2: %v", err)
	}

	resp := doJSON(t, http.MethodGet, fx.server.URL+"/api/v1/tags?counts=1", nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, string(body))
	}
	var out []tag.TagWithCount
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("expected 3 tags, got %d (%+v)", len(out), out)
	}
	counts := map[string]int64{}
	for _, r := range out {
		counts[r.Name] = r.Count
	}
	if counts["work"] != 2 {
		t.Errorf("work count = %d, want 2", counts["work"])
	}
	if counts["home"] != 1 {
		t.Errorf("home count = %d, want 1", counts["home"])
	}
	if counts["lonely"] != 0 {
		t.Errorf("lonely count = %d, want 0", counts["lonely"])
	}
	// Sanity: ids round-trip too.
	for _, r := range out {
		if r.Name == "lonely" && r.ID != tagLonely.ID {
			t.Errorf("lonely id = %d, want %d", r.ID, tagLonely.ID)
		}
	}
}

func TestTags_DeleteCascadesFromTasks(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	ctx := context.Background()
	tagRow, err := fx.tags.Create(ctx, "later")
	if err != nil {
		t.Fatalf("create tag: %v", err)
	}
	tsk, err := fx.tasks.Create(ctx, task.CreateInput{Title: "do stuff"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := fx.tasks.SetTagsByID(ctx, tsk.ID, []int64{tagRow.ID}); err != nil {
		t.Fatalf("set tags: %v", err)
	}

	resp := doJSON(t, http.MethodDelete, fmt.Sprintf("%s/api/v1/tags/%d", fx.server.URL, tagRow.ID), nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	reloaded, err := fx.tasks.Get(ctx, tsk.ID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if len(reloaded.Tags) != 0 {
		t.Fatalf("expected cascade, task tags = %v", reloaded.Tags)
	}
}
