package crawler

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/pcowhill/offline-developer-portal/internal/config"
	"github.com/pcowhill/offline-developer-portal/internal/index"
	"github.com/pcowhill/offline-developer-portal/internal/testsite"
)

func parseConfig(t *testing.T, yaml string) *config.Config {
	t.Helper()
	cfg, err := config.Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

type logger struct {
	mu    sync.Mutex
	lines []string
}

func (l *logger) logf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func (l *logger) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.lines, "\n")
}

func crawlFixture(t *testing.T, extra string) (*Result, *testsite.Site, *logger) {
	t.Helper()
	site := testsite.New()
	t.Cleanup(site.Close)
	cfg := parseConfig(t, fmt.Sprintf(`
settings:
  concurrency: 3
  request_delay_ms: 0
sources:
  - id: fixture
    name: Fixture Docs
    start_urls: [%[1]s/docs/]
    allowed_prefixes: [%[1]s/docs/]
    max_pages: 50
%[2]s`, site.URL, extra))
	log := &logger{}
	res, err := Crawl(context.Background(), Options{Config: cfg, UserAgent: UserAgent("test", ""), Log: log.logf}, &cfg.Sources[0])
	if err != nil {
		t.Fatal(err)
	}
	return res, site, log
}

func docPaths(res *Result, base string) []string {
	var out []string
	for _, d := range res.Documents {
		out = append(out, strings.TrimPrefix(d.URL, base))
	}
	sort.Strings(out)
	return out
}

func TestCrawlStaysInBoundsAndExtracts(t *testing.T) {
	res, site, log := crawlFixture(t, "")
	got := docPaths(res, site.URL)
	want := []string{"/docs/", "/docs/api.html", "/docs/guide.html", "/docs/only-from-noindex.html"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("documents = %v, want %v\nlog:\n%s", got, want, log)
	}

	for _, r := range site.Requests() {
		if r.Method != "GET" {
			t.Errorf("non-GET request %s %s", r.Method, r.Path)
		}
		if !strings.Contains(r.UserAgent, RobotsToken) {
			t.Errorf("missing User-Agent token: %q", r.UserAgent)
		}
		if strings.HasPrefix(r.Path, "/blog/") {
			t.Errorf("crawler left its prefix: %s", r.Path)
		}
	}
	for _, p := range []string{"/docs/secret/hidden.html", "/docs/manual.pdf"} {
		if site.Requested(p) {
			t.Errorf("%s must not be requested", p)
		}
	}
	// Each URL is fetched once even though it is linked from many pages and
	// with query strings / fragments.
	counts := map[string]int{}
	for _, r := range site.Requests() {
		counts[r.Path]++
	}
	// (api.html is also reached once through /docs/redirect-in.)
	counts["/docs/api.html"]--
	for p, n := range counts {
		if n > 1 && p != "/docs/redirect-out" {
			t.Errorf("%s requested %d times", p, n)
		}
	}
	// Redirect leaving the boundaries is refused (and not retried into the blog).
	if !strings.Contains(log.String(), "refused") {
		t.Errorf("expected a refused redirect in the log:\n%s", log)
	}
	if res.Errors == 0 || res.PagesSkipped == 0 || res.OutOfScope == 0 {
		t.Errorf("stats not recorded: %+v", res)
	}
	for _, d := range res.Documents {
		if d.SourceID != "fixture" || d.SourceName != "Fixture Docs" || d.IndexedAt.IsZero() || d.ID != index.DocumentID("fixture", d.URL) {
			t.Errorf("bad provenance: %+v", d)
		}
	}
}

func TestCrawlSkipsNonHTMLAndNoIndex(t *testing.T) {
	_, _, log := crawlFixture(t, "")
	l := log.String()
	for _, want := range []string{"not HTML: application/octet-stream", "noindex", "duplicate content", "HTTP 404", "disallowed by robots.txt"} {
		if !strings.Contains(l, want) {
			t.Errorf("log does not mention %q:\n%s", want, l)
		}
	}
}

func TestCrawlMaxPages(t *testing.T) {
	site := testsite.New()
	defer site.Close()
	cfg := parseConfig(t, fmt.Sprintf(`
settings: {concurrency: 4, request_delay_ms: 0}
sources:
  - id: many
    start_urls: [%[1]s/many/p1.html]
    allowed_prefixes: [%[1]s/many/]
    max_pages: 7
`, site.URL))
	res, err := Crawl(context.Background(), Options{Config: cfg, UserAgent: "t"}, &cfg.Sources[0])
	if err != nil {
		t.Fatal(err)
	}
	if n := site.PageRequests(); n != 7 {
		t.Errorf("page requests = %d, want exactly max_pages (7)", n)
	}
	if len(res.Documents) != 7 {
		t.Errorf("documents = %d", len(res.Documents))
	}
}

func TestCrawlMaxDepth(t *testing.T) {
	site := testsite.New()
	defer site.Close()
	cfg := parseConfig(t, fmt.Sprintf(`
settings: {request_delay_ms: 0}
sources:
  - id: many
    start_urls: [%[1]s/many/p1.html]
    allowed_prefixes: [%[1]s/many/]
    max_depth: 2
`, site.URL))
	res, err := Crawl(context.Background(), Options{Config: cfg, UserAgent: "t"}, &cfg.Sources[0])
	if err != nil {
		t.Fatal(err)
	}
	// depth 0: p1, depth 1: p11 p12, depth 2: p111 p112 p121 p122
	if len(res.Documents) != 7 {
		t.Errorf("documents = %d, want 7", len(res.Documents))
	}
}

func TestCrawlExcludeAndIncludePatterns(t *testing.T) {
	res, site, _ := crawlFixture(t, "    exclude_patterns: ['api\\.html$']\n")
	if site.Requested("/docs/api.html") {
		t.Error("excluded page was requested")
	}
	for _, d := range res.Documents {
		if strings.HasSuffix(d.URL, "api.html") {
			t.Error("excluded page indexed")
		}
	}
	res, site, _ = crawlFixture(t, "    include_patterns: ['/docs/$', 'guide']\n")
	if site.Requested("/docs/api.html") {
		t.Error("page not matching include_patterns was requested")
	}
	if got := docPaths(res, site.URL); strings.Join(got, ",") != "/docs/,/docs/guide.html" {
		t.Errorf("documents = %v", got)
	}
}

func TestRobotsOverride(t *testing.T) {
	res, site, log := crawlFixture(t, "    respect_robots_txt: false\n")
	if !site.Requested("/docs/secret/hidden.html") {
		t.Error("with robots disabled the secret page should be fetched")
	}
	if site.Requested("/robots.txt") {
		t.Error("robots.txt should not be fetched when disabled")
	}
	if !strings.Contains(log.String(), "NOT being respected") {
		t.Error("override must be reported loudly")
	}
	found := false
	for _, d := range res.Documents {
		found = found || strings.HasSuffix(d.URL, "/docs/noindex.html")
	}
	if !found {
		t.Error("meta noindex is only honoured while respecting robots rules")
	}
}

func TestCrawlCancelled(t *testing.T) {
	site := testsite.New()
	defer site.Close()
	cfg := parseConfig(t, fmt.Sprintf(`
sources:
  - id: many
    start_urls: [%[1]s/many/p1.html]
    allowed_prefixes: [%[1]s/many/]
`, site.URL))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Crawl(ctx, Options{Config: cfg, UserAgent: "t"}, &cfg.Sources[0]); err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestIsHTML(t *testing.T) {
	cases := []struct {
		ct   string
		head string
		want bool
	}{
		{"text/html; charset=utf-8", "", true},
		{"application/xhtml+xml", "", true},
		{"application/json", "<html>", false},
		{"", "<!DOCTYPE html><html>", true},
		{"", "%PDF-1.4", false},
	}
	for _, c := range cases {
		if got := isHTML(c.ct, []byte(c.head)); got != c.want {
			t.Errorf("isHTML(%q, %q) = %v", c.ct, c.head, got)
		}
	}
}
