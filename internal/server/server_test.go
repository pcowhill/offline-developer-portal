package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setup(t *testing.T) http.Handler {
	t.Helper()
	root := t.TempDir()
	dist := filepath.Join(root, "dist")
	corpus := filepath.Join(root, "corpus")
	files := map[string]string{
		filepath.Join(dist, "index.html"):               "<!doctype html><title>portal</title>",
		filepath.Join(dist, "assets", "app-abc.js"):     "console.log(1)",
		filepath.Join(dist, "assets", "app-abc.css"):    "body{}",
		filepath.Join(corpus, "manifest.json"):          `{"format":"x"}`,
		filepath.Join(corpus, "docs", "ab", "ab1.json"): `{"id":"ab1"}`,
		filepath.Join(root, "secret.txt"):               "secret",
	}
	for p, c := range files {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return Handler(Options{DistDir: dist, CorpusDir: corpus})
}

func get(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "http://localhost"+path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestServesPortalAndCorpus(t *testing.T) {
	h := setup(t)
	cases := []struct {
		path, ctype, body string
	}{
		{"/", "text/html; charset=utf-8", "portal"},
		{"/index.html", "text/html; charset=utf-8", "portal"},
		{"/assets/app-abc.js", "text/javascript; charset=utf-8", "console.log"},
		{"/assets/app-abc.css", "text/css; charset=utf-8", "body"},
		{"/corpus/manifest.json", "application/json; charset=utf-8", `"format"`},
		{"/corpus/docs/ab/ab1.json", "application/json; charset=utf-8", `"ab1"`},
	}
	for _, c := range cases {
		rec := get(t, h, http.MethodGet, c.path)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status %d", c.path, rec.Code)
			continue
		}
		if got := rec.Header().Get("Content-Type"); got != c.ctype {
			t.Errorf("%s: content type %q, want %q", c.path, got, c.ctype)
		}
		if !strings.Contains(rec.Body.String(), c.body) {
			t.Errorf("%s: body %q", c.path, rec.Body.String())
		}
		if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
			t.Errorf("%s: missing CSP", c.path)
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: missing nosniff", c.path)
		}
	}
	if cc := get(t, h, "GET", "/corpus/manifest.json").Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("corpus cache-control = %q", cc)
	}
	if cc := get(t, h, "GET", "/assets/app-abc.js").Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("asset cache-control = %q", cc)
	}
	if rec := get(t, h, http.MethodHead, "/"); rec.Code != http.StatusOK {
		t.Errorf("HEAD status %d", rec.Code)
	}
}

func TestRejectsTraversalListingsAndWrites(t *testing.T) {
	h := setup(t)
	for _, p := range []string{"/../secret.txt", "/corpus/../secret.txt", "/%2e%2e/secret.txt", "/corpus/docs/", "/corpus/", "/assets/", "/missing.js", "/corpus/missing.json"} {
		rec := get(t, h, http.MethodGet, p)
		body, _ := io.ReadAll(rec.Body)
		if rec.Code == http.StatusOK && !strings.Contains(string(body), "portal") {
			t.Errorf("%s: unexpectedly served %q", p, body)
		}
		if strings.Contains(string(body), "TOP-SECRET-CONTENT") {
			t.Errorf("%s: leaked file outside the served directories", p)
		}
	}
	if rec := get(t, h, http.MethodGet, "/corpus/docs/"); rec.Code != http.StatusNotFound {
		t.Errorf("directory listing status = %d, want 404", rec.Code)
	}
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		if rec := get(t, h, m, "/"); rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s status = %d", m, rec.Code)
		}
	}
}

func TestMissingCorpusIs404(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "dist"), 0o755)
	os.WriteFile(filepath.Join(root, "dist", "index.html"), []byte("x"), 0o644)
	h := Handler(Options{DistDir: filepath.Join(root, "dist"), CorpusDir: filepath.Join(root, "nope")})
	if rec := get(t, h, "GET", "/corpus/manifest.json"); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 so the UI can show setup instructions", rec.Code)
	}
}
