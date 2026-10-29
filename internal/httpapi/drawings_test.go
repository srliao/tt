package httpapi_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/srliao/tt/internal/drawing"
)

func decodeDrawing(t *testing.T, resp *http.Response) drawing.Drawing {
	t.Helper()
	var out drawing.Drawing
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode drawing: %v", err)
	}
	return out
}

func TestDrawings_CreateSaveGetListDelete(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	base := fx.server.URL + "/api/v1/drawings"

	resp := doJSON(t, http.MethodPost, base, map[string]any{"name": "Sketch"})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create status = %d, body = %s", resp.StatusCode, body)
	}
	created := decodeDrawing(t, resp)
	item := fmt.Sprintf("%s/%d", base, created.ID)

	scene := map[string]any{"elements": []any{map[string]any{"id": "a"}}, "appState": map[string]any{}}
	resp2 := doJSON(t, http.MethodPatch, item, map[string]any{"name": "Renamed", "scene": scene})
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("patch status = %d, body = %s", resp2.StatusCode, body)
	}

	resp3 := doJSON(t, http.MethodGet, item, nil)
	defer func() { _ = resp3.Body.Close() }()
	got := decodeDrawing(t, resp3)
	if got.Name != "Renamed" {
		t.Errorf("name = %q, want Renamed", got.Name)
	}
	var gotScene map[string]any
	if err := json.Unmarshal(got.Scene, &gotScene); err != nil || len(gotScene["elements"].([]any)) != 1 {
		t.Errorf("scene = %s, want one element", got.Scene)
	}

	resp4 := doJSON(t, http.MethodGet, base, nil)
	defer func() { _ = resp4.Body.Close() }()
	var list []map[string]any
	if err := json.NewDecoder(resp4.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list = %+v, want 1", list)
	}
	if _, has := list[0]["scene"]; has {
		t.Error("list item includes scene; listing should stay lightweight")
	}

	resp5 := doJSON(t, http.MethodDelete, item, nil)
	defer func() { _ = resp5.Body.Close() }()
	if resp5.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", resp5.StatusCode)
	}
	resp6 := doJSON(t, http.MethodGet, item, nil)
	defer func() { _ = resp6.Body.Close() }()
	if resp6.StatusCode != http.StatusNotFound {
		t.Fatalf("get after delete status = %d, want 404", resp6.StatusCode)
	}
}

func TestDrawings_PatchValidation(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	resp := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/drawings", map[string]any{"name": "x"})
	defer func() { _ = resp.Body.Close() }()
	item := fmt.Sprintf("%s/api/v1/drawings/%d", fx.server.URL, decodeDrawing(t, resp).ID)

	for name, body := range map[string]any{
		"empty":       map[string]any{},
		"array scene": map[string]any{"scene": []any{}},
		"blank name":  map[string]any{"name": "  "},
		"bad scene":   map[string]any{"name": "renamed", "scene": []any{}},
	} {
		r := doJSON(t, http.MethodPatch, item, body)
		_ = r.Body.Close()
		if r.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, r.StatusCode)
		}
	}

	// A rejected PATCH must not have applied its name.
	got := doJSON(t, http.MethodGet, item, nil)
	defer func() { _ = got.Body.Close() }()
	if d := decodeDrawing(t, got); d.Name != "x" {
		t.Errorf("name = %q after rejected PATCHes, want unchanged %q", d.Name, "x")
	}
}

func TestDrawings_CreateWithScene(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	base := fx.server.URL + "/api/v1/drawings"

	scene := map[string]any{"type": "excalidraw", "elements": []any{map[string]any{"id": "a"}}}
	resp := doJSON(t, http.MethodPost, base, map[string]any{"name": "Imported", "scene": scene})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create status = %d, body = %s", resp.StatusCode, body)
	}
	var got map[string]any
	if err := json.Unmarshal(decodeDrawing(t, resp).Scene, &got); err != nil || got["type"] != "excalidraw" {
		t.Errorf("scene = %v, want the posted scene", got)
	}

	bad := doJSON(t, http.MethodPost, base, map[string]any{"name": "Bad", "scene": "nope"})
	_ = bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Errorf("non-object scene status = %d, want 400", bad.StatusCode)
	}
}
