package httpapi_test

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/srliao/tt/internal/httpapi"
)

func TestSPA_ServesIndexNoCache(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"index.html":         {Data: []byte("<!doctype html><body>app</body>")},
		"assets/main-abc.js": {Data: []byte("console.log('hi')")},
	}

	ts := httptest.NewServer(httpapi.NewSPAHandler(fsys))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/index.html")
	if err != nil {
		t.Fatalf("GET /index.html: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("cache-control = %q", got)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "<!doctype html><body>app</body>" {
		t.Fatalf("body = %s", string(body))
	}
}

func TestSPA_ServesAssetWithLongCache(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"index.html":         {Data: []byte("idx")},
		"assets/main-abc.js": {Data: []byte("js content")},
	}

	ts := httptest.NewServer(httpapi.NewSPAHandler(fsys))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/assets/main-abc.js")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("cache-control = %q", got)
	}
	if got := resp.Header.Get("Content-Type"); got == "" || got[:len("application/javascript")] != "application/javascript" {
		t.Fatalf("content-type = %q", got)
	}
}

func TestSPA_MissingAssetIs404(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"index.html": {Data: []byte("idx")},
	}

	ts := httptest.NewServer(httpapi.NewSPAHandler(fsys))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/assets/missing.js")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestSPA_SPAFallbackServesIndex(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"index.html": {Data: []byte("spa root")},
	}

	ts := httptest.NewServer(httpapi.NewSPAHandler(fsys))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/some/spa/route")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "spa root" {
		t.Fatalf("body = %q", string(body))
	}
}

// TestSPA_FSSubMirrorsEmbed exercises the same shape cmd/tt uses in prod:
// the //go:embed directive produces a tree rooted at "dist/", which web.Dist
// strips via fs.Sub. The handler should not care about the original prefix.
func TestSPA_FSSubMirrorsEmbed(t *testing.T) {
	t.Parallel()

	raw := fstest.MapFS{
		"dist/index.html":         {Data: []byte("embedded idx")},
		"dist/assets/app-xyz.js":  {Data: []byte("embedded js")},
		"dist/assets/app-xyz.css": {Data: []byte("embedded css")},
	}
	sub, err := fs.Sub(raw, "dist")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}

	ts := httptest.NewServer(httpapi.NewSPAHandler(sub))
	defer ts.Close()

	cases := []struct {
		path        string
		wantStatus  int
		wantBody    string
		wantCache   string
		contentType string
	}{
		{"/", http.StatusOK, "embedded idx", "no-cache", "text/html"},
		{"/some/deep/route", http.StatusOK, "embedded idx", "no-cache", "text/html"},
		{"/assets/app-xyz.js", http.StatusOK, "embedded js", "public, max-age=31536000, immutable", "application/javascript"},
		{"/assets/app-xyz.css", http.StatusOK, "embedded css", "public, max-age=31536000, immutable", "text/css"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.path, func(t *testing.T) {
			resp, err := http.Get(ts.URL + tc.path)
			if err != nil {
				t.Fatalf("GET %s: %v", tc.path, err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
			if got := resp.Header.Get("Cache-Control"); got != tc.wantCache {
				t.Fatalf("cache-control = %q, want %q", got, tc.wantCache)
			}
			if got := resp.Header.Get("Content-Type"); len(got) < len(tc.contentType) || got[:len(tc.contentType)] != tc.contentType {
				t.Fatalf("content-type = %q, want prefix %q", got, tc.contentType)
			}
			body, _ := io.ReadAll(resp.Body)
			if string(body) != tc.wantBody {
				t.Fatalf("body = %q, want %q", string(body), tc.wantBody)
			}
		})
	}
}

func TestSPA_NilFsAlways404(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(httpapi.NewSPAHandler(nil))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/anything")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
