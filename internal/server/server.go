// Package server serves the static portal (dist/) and the generated
// corpus on a local address. It is read-only: only GET and HEAD are
// accepted and nothing is fetched from the network.
package server

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// CorpusPrefix is the URL path under which the corpus directory is served.
const CorpusPrefix = "/corpus/"

// contentTypes are set explicitly because on Windows the MIME table is
// read from the registry and is sometimes wrong for .js (which breaks
// module scripts).
var contentTypes = map[string]string{
	".html":  "text/html; charset=utf-8",
	".js":    "text/javascript; charset=utf-8",
	".mjs":   "text/javascript; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".json":  "application/json; charset=utf-8",
	".svg":   "image/svg+xml",
	".png":   "image/png",
	".ico":   "image/x-icon",
	".txt":   "text/plain; charset=utf-8",
	".map":   "application/json; charset=utf-8",
	".woff2": "font/woff2",
}

// contentSecurityPolicy forbids loading anything from other origins,
// which also guarantees the portal works offline.
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; connect-src 'self'; font-src 'self'; object-src 'none'; " +
	"base-uri 'self'; form-action 'none'; frame-ancestors 'none'"

// Options configure the handler.
type Options struct {
	DistDir   string
	CorpusDir string
}

// Handler returns the HTTP handler for the portal.
func Handler(opts Options) http.Handler {
	mux := http.NewServeMux()
	mux.Handle(CorpusPrefix, http.StripPrefix(strings.TrimSuffix(CorpusPrefix, "/"), fileHandler(opts.CorpusDir, false, true)))
	mux.Handle("/", fileHandler(opts.DistDir, true, false))
	return secure(mux)
}

func secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

// fileHandler serves files from root without directory listings. When
// spaIndex is set, "/" (and any directory) serves index.html. noCache
// asks the browser to revalidate, so a re-indexed corpus shows up on reload.
func fileHandler(root string, spaIndex, noCache bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean("/" + r.URL.Path)
		if strings.Contains(p, "\x00") {
			http.NotFound(w, r)
			return
		}
		if spaIndex && strings.HasSuffix(p, "/") {
			p += "index.html"
		}
		full := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(p, "/")))
		if !within(root, full) {
			http.NotFound(w, r)
			return
		}
		fi, err := os.Stat(full)
		if err != nil || fi.IsDir() {
			if errors.Is(err, fs.ErrNotExist) || err == nil {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "unable to read file", http.StatusInternalServerError)
			return
		}
		f, err := os.Open(full)
		if err != nil {
			http.Error(w, "unable to read file", http.StatusInternalServerError)
			return
		}
		defer f.Close()
		if ct, ok := contentTypes[strings.ToLower(filepath.Ext(full))]; ok {
			w.Header().Set("Content-Type", ct)
		}
		switch {
		case noCache || strings.HasSuffix(full, "index.html"):
			w.Header().Set("Cache-Control", "no-cache")
		case strings.Contains(p, "/assets/"):
			// Vite fingerprints asset file names.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeContent(w, r, fi.Name(), fi.ModTime(), f)
	})
}

func within(root, p string) bool {
	rootAbs, err1 := filepath.Abs(root)
	pAbs, err2 := filepath.Abs(p)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, pAbs)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
