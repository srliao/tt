package cli_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHeaders_NotSentWhenUnconfigured(t *testing.T) {
	f := newFixture(t)
	f.run("tag", "list").ok(t, nil)

	h := f.lastHeaders()
	if _, ok := h["Cf-Access-Client-Id"]; ok {
		t.Fatalf("CF-Access-Client-Id sent without config: %v", h)
	}
	if _, ok := h["Cf-Access-Client-Secret"]; ok {
		t.Fatalf("CF-Access-Client-Secret sent without config: %v", h)
	}
}

func TestConfig_FileProvidesURLAndCredentials(t *testing.T) {
	f := newFixture(t)
	f.requireAccess("file-id", "file-secret")
	f.env = map[string]string{}
	f.writeConfig(fmt.Sprintf("url = %q\ncf_client_id = \"file-id\"\ncf_client_secret = \"file-secret\"\n", f.server.URL), 0o600)

	f.run("tag", "list").ok(t, nil)
	h := f.lastHeaders()
	if h.Get("CF-Access-Client-Id") != "file-id" || h.Get("CF-Access-Client-Secret") != "file-secret" {
		t.Fatalf("headers = %v", h)
	}
}

func TestConfig_EnvOverridesFile(t *testing.T) {
	f := newFixture(t)
	f.requireAccess("file-id", "env-secret")
	f.writeConfig("url = \"https://tt.example.com\"\ncf_client_id = \"file-id\"\ncf_client_secret = \"file-secret\"\n", 0o600)
	f.env["TT_CF_CLIENT_SECRET"] = "env-secret"

	f.run("tag", "list").ok(t, nil)
	h := f.lastHeaders()
	if h.Get("CF-Access-Client-Id") != "file-id" || h.Get("CF-Access-Client-Secret") != "env-secret" {
		t.Fatalf("headers = %v", h)
	}
}

func TestConfig_MissingURLIsUsageError(t *testing.T) {
	f := newFixture(t)
	f.env = map[string]string{}
	f.run("tag", "list").fails(t, 1, "config_missing")
}

func TestAuthFailure_AccessRedirectIsExit4(t *testing.T) {
	f := newFixture(t)
	f.requireAccess("id", "secret")

	f.run("task", "list").fails(t, 4, "auth_failed")

	f.env["TT_CF_CLIENT_ID"] = "id"
	f.env["TT_CF_CLIENT_SECRET"] = "wrong"
	e := f.run("task", "get", "1").fails(t, 4, "auth_failed")
	if !strings.Contains(fmt.Sprint(e.Details["location"]), "tt.example.com") {
		t.Fatalf("details = %v", e.Details)
	}
}

func TestAuthFailure_HTMLPageIsExit4(t *testing.T) {
	f := newFixture(t)
	html := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("<html>Forbidden</html>"))
	}))
	t.Cleanup(html.Close)
	f.env["TT_URL"] = html.URL

	f.run("tag", "list").fails(t, 4, "auth_failed")
}

func TestNetworkFailure_IsExit4(t *testing.T) {
	f := newFixture(t)
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	f.env["TT_URL"] = dead.URL

	f.run("tag", "list").fails(t, 4, "network_error")
}

type doctorOut struct {
	OK     bool `json:"ok"`
	Config struct {
		Path      string            `json:"path"`
		FileFound bool              `json:"file_found"`
		URL       string            `json:"url"`
		Sources   map[string]string `json:"sources"`
		CFAccess  bool              `json:"cf_access"`
	} `json:"config"`
	Server struct {
		Reachable bool   `json:"reachable"`
		AuthOK    bool   `json:"auth_ok"`
		Version   string `json:"version"`
		Timezone  string `json:"timezone"`
		Time      string `json:"time"`
	} `json:"server"`
	Warnings []string `json:"warnings"`
}

func TestDoctor_ReportsServer(t *testing.T) {
	f := newFixture(t)
	f.requireAccess("id", "secret")
	f.writeConfig("cf_client_id = \"id\"\ncf_client_secret = \"secret\"\n", 0o600)

	var out doctorOut
	f.run("doctor").ok(t, &out)
	if !out.OK || !out.Server.Reachable || !out.Server.AuthOK {
		t.Fatalf("out = %+v", out)
	}
	if out.Server.Version != "test-1.2.3" || out.Server.Timezone != "America/New_York" {
		t.Fatalf("server = %+v", out.Server)
	}
	if _, err := time.Parse(time.RFC3339, out.Server.Time); err != nil {
		t.Fatalf("server time %q: %v", out.Server.Time, err)
	}
	if !out.Config.FileFound || out.Config.Path != f.config || !out.Config.CFAccess {
		t.Fatalf("config = %+v", out.Config)
	}
	if out.Config.Sources["url"] != "env" || out.Config.Sources["cf_client_id"] != "file" {
		t.Fatalf("sources = %v", out.Config.Sources)
	}
	if len(out.Warnings) != 0 {
		t.Fatalf("warnings = %v", out.Warnings)
	}
}

func TestDoctor_WarnsOnLooseConfigMode(t *testing.T) {
	f := newFixture(t)
	f.writeConfig("", 0o644)

	var out doctorOut
	f.run("doctor").ok(t, &out)
	if len(out.Warnings) != 1 || !strings.Contains(out.Warnings[0], "0600") {
		t.Fatalf("warnings = %v", out.Warnings)
	}
}

func TestDoctor_AuthFailure(t *testing.T) {
	f := newFixture(t)
	f.requireAccess("id", "secret")

	e := f.run("doctor").fails(t, 4, "auth_failed")
	server, _ := e.Details["server"].(map[string]any)
	if server["reachable"] != true || server["auth_ok"] != false {
		t.Fatalf("details = %v", e.Details)
	}
}
