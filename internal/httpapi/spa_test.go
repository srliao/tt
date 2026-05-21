package httpapi_test

import (
	"io"
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
	defer resp.Body.Close()
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
	defer resp.Body.Close()
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
	defer resp.Body.Close()
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
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "spa root" {
		t.Fatalf("body = %q", string(body))
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
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
