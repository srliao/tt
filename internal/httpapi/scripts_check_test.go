package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/srliao/tt/internal/httpapi"
	"github.com/srliao/tt/internal/script"
)

func scriptPayload(code string) map[string]any {
	return map[string]any{
		"name":     "s",
		"code":     code,
		"enabled":  true,
		"schedule": map[string]any{"kind": "daily"},
	}
}

func expectSyntaxDetails(t *testing.T, details map[string]any) {
	t.Helper()
	if details["field"] != "code" {
		t.Errorf("details.field = %v, want code", details["field"])
	}
	if details["line"] != float64(2) {
		t.Errorf("details.line = %v, want 2", details["line"])
	}
	if details["column"] != float64(11) {
		t.Errorf("details.column = %v, want 11", details["column"])
	}
}

const badCode = "const a = 1;\nconst b = ;\n"

func TestScripts_CreateRejectsSyntaxError(t *testing.T) {
	t.Parallel()
	fx := newTestServer(t, nil)

	resp := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/scripts", scriptPayload(badCode))
	expectSyntaxDetails(t, expectError(t, resp, http.StatusBadRequest, httpapi.CodeValidation))

	list := doJSON(t, http.MethodGet, fx.server.URL+"/api/v1/scripts", nil)
	defer func() { _ = list.Body.Close() }()
	var scripts []script.Script
	if err := json.NewDecoder(list.Body).Decode(&scripts); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(scripts) != 0 {
		t.Errorf("scripts = %v, want none saved", scripts)
	}
}

func TestScripts_PatchRejectsSyntaxError(t *testing.T) {
	t.Parallel()
	fx := newTestServer(t, nil)

	resp := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/scripts", scriptPayload("ctx.log('ok');"))
	expectOK(t, resp)
	created := decodeScript(t, resp)
	_ = resp.Body.Close()

	url := fmt.Sprintf("%s/api/v1/scripts/%d", fx.server.URL, created.ID)
	patch := doJSON(t, http.MethodPatch, url, scriptPayload(badCode))
	expectSyntaxDetails(t, expectError(t, patch, http.StatusBadRequest, httpapi.CodeValidation))

	get := doJSON(t, http.MethodGet, url, nil)
	defer func() { _ = get.Body.Close() }()
	if got := decodeScript(t, get); got.Code != "ctx.log('ok');" {
		t.Errorf("code = %q, want unchanged", got.Code)
	}
}
