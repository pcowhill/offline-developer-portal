// Package testsite provides a small fake documentation website for tests.
// It records every request so tests can assert that the crawler stayed
// within its boundaries.
package testsite

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// Request is one recorded request.
type Request struct {
	Method    string
	Path      string
	UserAgent string
}

// Site is a running fake website.
type Site struct {
	*httptest.Server
	mu       sync.Mutex
	requests []Request
}

// Requests returns a copy of the recorded requests.
func (s *Site) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

// Requested reports whether path was requested.
func (s *Site) Requested(path string) bool {
	for _, r := range s.Requests() {
		if r.Path == path {
			return true
		}
	}
	return false
}

// PageRequests counts requests other than /robots.txt.
func (s *Site) PageRequests() int {
	n := 0
	for _, r := range s.Requests() {
		if r.Path != "/robots.txt" {
			n++
		}
	}
	return n
}

func page(title, body string) string {
	return fmt.Sprintf(`<!doctype html><html><head><title>%s</title></head><body>
<nav class="sidebar"><a href="/docs/">Home</a> <a href="/docs/guide.html">Guide</a> <a href="/docs/api.html">API</a></nav>
<main>%s</main><footer>footer</footer></body></html>`, title, body)
}

// Pages is the content of the fake site, keyed by path.
var Pages = map[string]string{
	"/docs/": page("Home - Fixture Docs", `<h1>Fixture Docs</h1>
<p>Welcome to the fixture documentation. Start with the <a href="guide.html">guide</a>.</p>
<ul>
<li><a href="api.html">API reference</a></li>
<li><a href="secret/hidden.html">Secret (robots disallowed)</a></li>
<li><a href="/blog/post.html">Blog (outside prefix)</a></li>
<li><a href="http://outside.invalid/docs/">Other host</a></li>
<li><a href="manual.pdf">PDF manual</a></li>
<li><a href="download">Binary download</a></li>
<li><a href="redirect-out">Redirect leaving the site</a></li>
<li><a href="redirect-in">Redirect inside</a></li>
<li><a href="missing.html">Broken link</a></li>
<li><a href="copy.html">Duplicate of guide</a></li>
<li><a href="noindex.html">No-index page</a></li>
<li><a href="guide.html?utm_source=x#install">Guide with query and fragment</a></li>
</ul>`),
	"/docs/guide.html": page("Guide - Fixture Docs", `<h1>User Guide</h1>
<p>The frobnicator turns widgets into gadgets.</p>
<h2 id="install">Installing the frobnicator</h2>
<pre class="language-sh">$ fixture install frobnicator</pre>
<h2>Configuration</h2><p>Set <code>FROB_LEVEL</code> to configure verbosity.</p>`),
	"/docs/copy.html": page("Guide - Fixture Docs", `<h1>User Guide</h1>
<p>The frobnicator turns widgets into gadgets.</p>
<h2 id="install">Installing the frobnicator</h2>
<pre class="language-sh">$ fixture install frobnicator</pre>
<h2>Configuration</h2><p>Set <code>FROB_LEVEL</code> to configure verbosity.</p>`),
	"/docs/api.html": page("API - Fixture Docs", `<h1>API Reference</h1>
<h2 id="new">func New</h2><pre><code class="language-go">func New(level int) *Frobnicator</code></pre>
<p>New creates a frobnicator. See the <a href="guide.html#install">installation guide</a>.</p>
<table><tr><th>Level</th><th>Meaning</th></tr><tr><td>0</td><td>quiet</td></tr></table>`),
	"/docs/noindex.html":           `<html><head><title>Private</title><meta name="robots" content="noindex"></head><body><main><p>Do not index this page please, it is private.</p><a href="/docs/only-from-noindex.html">x</a></main></body></html>`,
	"/docs/only-from-noindex.html": page("Reachable via noindex", `<p>Links on noindex pages are still followed.</p>`),
	"/docs/secret/hidden.html":     page("Secret", `<p>robots.txt forbids this.</p>`),
	"/blog/post.html":              page("Blog", `<p>outside the prefix</p>`),
	"/blog/elsewhere.html":         page("Blog", `<p>outside the prefix</p>`),
}

// New starts the fake site.
func New() *Site {
	s := &Site{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests = append(s.requests, Request{Method: r.Method, Path: r.URL.Path, UserAgent: r.UserAgent()})
		s.mu.Unlock()
		switch r.URL.Path {
		case "/robots.txt":
			fmt.Fprint(w, "User-agent: *\nDisallow: /docs/secret/\n")
			return
		case "/docs/download":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write([]byte{0, 1, 2, 3})
			return
		case "/docs/redirect-out":
			http.Redirect(w, r, "/blog/elsewhere.html", http.StatusFound)
			return
		case "/docs/redirect-in":
			http.Redirect(w, r, "/docs/api.html", http.StatusMovedPermanently)
			return
		case "/docs/manual.pdf":
			w.Header().Set("Content-Type", "application/pdf")
			w.Write([]byte("%PDF-1.4"))
			return
		}
		body, ok := Pages[r.URL.Path]
		if !ok {
			if strings.HasPrefix(r.URL.Path, "/many/") {
				n := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/many/p"), ".html")
				fmt.Fprint(w, page("Page "+n, fmt.Sprintf(`<p>Generated page %s with unique text %s.</p><a href="p%s1.html">next a</a> <a href="p%s2.html">next b</a>`, n, n, n, n)))
				return
			}
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, body)
	}))
	return s
}
