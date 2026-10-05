package indexer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pcowhill/offline-developer-portal/internal/index"
	"github.com/pcowhill/offline-developer-portal/internal/testsite"
)

func writeConfig(t *testing.T, dir, body string) string {
	t.Helper()
	p := filepath.Join(dir, "sources.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func twoSources(base string) string {
	return fmt.Sprintf(`
settings: {request_delay_ms: 0}
sources:
  - id: docs
    name: Fixture Docs
    start_urls: [%[1]s/docs/]
    allowed_prefixes: [%[1]s/docs/]
  - id: many
    name: Generated Pages
    start_urls: [%[1]s/many/p1.html]
    allowed_prefixes: [%[1]s/many/]
    max_pages: 3
`, base)
}

func TestRunWritesCompleteCorpus(t *testing.T) {
	site := testsite.New()
	defer site.Close()
	dir := t.TempDir()
	cfgPath := writeConfig(t, dir, twoSources(site.URL))
	corpus := filepath.Join(dir, "corpus")
	var out bytes.Buffer
	fixed := time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)

	sum, err := Run(context.Background(), Options{ConfigPath: cfgPath, CorpusDir: corpus, Version: "test", Out: &out, Now: func() time.Time { return fixed }})
	if err != nil {
		t.Fatalf("Run: %v\n%s", err, out.String())
	}
	m, err := index.ReadManifest(corpus)
	if err != nil {
		t.Fatal(err)
	}
	if m.DocumentCount != 7 || len(m.Sources) != 2 || !m.GeneratedAt.Equal(fixed) || m.Generator != "offline-docs test" {
		t.Fatalf("manifest = %+v", m)
	}
	if sum.Manifest.DocumentCount != 7 {
		t.Error("summary mismatch")
	}
	byID := map[string]index.SourceManifest{}
	for _, s := range m.Sources {
		byID[s.ID] = s
	}
	if d := byID["docs"]; d.DocumentCount != 4 || d.Status != "partial" || d.Errors == 0 || len(d.StartURLs) != 1 || !d.RobotsTxt {
		t.Errorf("docs source manifest = %+v", d)
	}
	if g := byID["many"]; g.DocumentCount != 3 || g.Status != "ok" {
		t.Errorf("many source manifest = %+v", g)
	}

	si, err := index.ReadSearchIndex(corpus)
	if err != nil {
		t.Fatal(err)
	}
	res := si.Search("frobnicator install", index.SearchOptions{})
	if len(res) == 0 || !strings.HasSuffix(res[0].Doc.URL, "/docs/guide.html") {
		t.Fatalf("search results = %+v", res)
	}
	doc, err := index.ReadDocument(corpus, res[0].Doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.SourceName != "Fixture Docs" || doc.URL != site.URL+"/docs/guide.html" || !doc.IndexedAt.Equal(fixed) {
		t.Errorf("provenance = %+v", doc)
	}
	for _, want := range []string{"The crawler only follows links inside the boundaries", "prefix:  " + site.URL + "/docs/", "Corpus written to"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q", want)
		}
	}
	if _, err := os.Stat(corpus + ".tmp"); err == nil {
		t.Error("temporary directory left behind")
	}

	// The corpus works without the original site.
	site.Close()
	if got := si.Search("gadgets", index.SearchOptions{Sources: []string{"docs"}}); len(got) == 0 {
		t.Error("offline search failed")
	}
}

func TestRunOnlyKeepsOtherSources(t *testing.T) {
	site := testsite.New()
	defer site.Close()
	dir := t.TempDir()
	cfgPath := writeConfig(t, dir, twoSources(site.URL))
	corpus := filepath.Join(dir, "corpus")
	if _, err := Run(context.Background(), Options{ConfigPath: cfgPath, CorpusDir: corpus, Version: "t"}); err != nil {
		t.Fatal(err)
	}
	before := len(site.Requests())
	if _, err := Run(context.Background(), Options{ConfigPath: cfgPath, CorpusDir: corpus, Version: "t", Only: []string{"many"}}); err != nil {
		t.Fatal(err)
	}
	for _, r := range site.Requests()[before:] {
		if strings.HasPrefix(r.Path, "/docs/") {
			t.Errorf("source 'docs' was re-crawled: %s", r.Path)
		}
	}
	m, err := index.ReadManifest(corpus)
	if err != nil {
		t.Fatal(err)
	}
	if m.DocumentCount != 7 || len(m.Sources) != 2 {
		t.Fatalf("manifest after partial re-index = %+v", m)
	}

	if _, err := Run(context.Background(), Options{ConfigPath: cfgPath, CorpusDir: corpus, Only: []string{"nope"}}); err == nil || !strings.Contains(err.Error(), "unknown source id") {
		t.Fatalf("expected unknown source error, got %v", err)
	}
}

func TestRunWithNoDocumentsKeepsExistingCorpus(t *testing.T) {
	site := testsite.New()
	dir := t.TempDir()
	cfgPath := writeConfig(t, dir, twoSources(site.URL))
	corpus := filepath.Join(dir, "corpus")
	if _, err := Run(context.Background(), Options{ConfigPath: cfgPath, CorpusDir: corpus, Version: "t"}); err != nil {
		t.Fatal(err)
	}
	site.Close() // the sites become unreachable

	cfg := strings.Replace(twoSources(site.URL), "settings: {request_delay_ms: 0}", "settings: {request_delay_ms: 0, request_timeout_seconds: 2}", 1)
	writeConfig(t, dir, cfg)
	var out bytes.Buffer
	_, err := Run(context.Background(), Options{ConfigPath: cfgPath, CorpusDir: corpus, Version: "t", Out: &out})
	if err == nil || !strings.Contains(err.Error(), "existing corpus was left unchanged") {
		t.Fatalf("expected failure, got %v\n%s", err, out.String())
	}
	m, err := index.ReadManifest(corpus)
	if err != nil || m.DocumentCount != 7 {
		t.Fatalf("existing corpus damaged: %+v %v", m, err)
	}
}

func TestRunInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	if _, err := Run(context.Background(), Options{ConfigPath: filepath.Join(dir, "sources.yaml"), CorpusDir: filepath.Join(dir, "c")}); err == nil || !strings.Contains(err.Error(), "sources.example.yaml") {
		t.Fatalf("missing config error = %v", err)
	}
	p := writeConfig(t, dir, "sources:\n  - id: x\n    start_urls: [https://e.com/]\n")
	if _, err := Run(context.Background(), Options{ConfigPath: p, CorpusDir: filepath.Join(dir, "c")}); err == nil || !strings.Contains(err.Error(), "allowed_prefixes") {
		t.Fatalf("invalid config error = %v", err)
	}
}
