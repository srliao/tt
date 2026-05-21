package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestWriteError_ShapeAndStatus(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	writeError(rec, 400, CodeValidation, "title is required", map[string]any{"field": "title"})

	if got := rec.Code; got != 400 {
		t.Fatalf("status = %d, want 400", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}

	var env errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if env.Error.Code != CodeValidation {
		t.Fatalf("code = %q, want %q", env.Error.Code, CodeValidation)
	}
	if env.Error.Message != "title is required" {
		t.Fatalf("message = %q", env.Error.Message)
	}
	if got := env.Error.Details["field"]; got != "title" {
		t.Fatalf("details.field = %v, want \"title\"", got)
	}
}

func TestWriteError_NilDetailsOmitted(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	writeError(rec, 500, CodeInternal, "boom", nil)

	// The raw JSON must not contain a "details" key when details is nil.
	body := rec.Body.String()
	if want := `"details"`; containsSubstr(body, want) {
		t.Fatalf("response should omit details, got: %s", body)
	}
}

func TestWriteJSON_ContentTypeAndStatus(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	writeJSON(rec, 201, map[string]string{"hello": "world"})

	if rec.Code != 201 {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	var m map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m["hello"] != "world" {
		t.Fatalf("body = %v", m)
	}
}

// containsSubstr is a tiny helper kept private to this test to avoid pulling
// in a stdlib package just for substring detection in literal JSON.
func containsSubstr(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
