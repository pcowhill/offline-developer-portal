package config

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validYAML = `
sources:
  - id: example-docs
    name: Example Documentation
    start_urls:
      - https://example.com/docs/
    allowed_prefixes:
      - https://example.com/docs/
    max_pages: 50
`

func TestParseValidAppliesDefaults(t *testing.T) {
	cfg, err := Parse([]byte(validYAML))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Settings.Concurrency != DefaultConcurrency {
		t.Errorf("concurrency = %d, want default %d", cfg.Settings.Concurrency, DefaultConcurrency)
	}
	if cfg.RequestDelay() != DefaultRequestDelay {
		t.Errorf("delay = %v", cfg.RequestDelay())
	}
	if cfg.RequestTimeout() != DefaultRequestTimeout {
		t.Errorf("timeout = %v", cfg.RequestTimeout())
	}
	src := cfg.Source("example-docs")
	if src == nil || src.MaxPages != 50 || src.Name != "Example Documentation" {
		t.Fatalf("unexpected source: %+v", src)
	}
	if !cfg.RespectsRobots(src) {
		t.Error("robots.txt must be respected by default")
	}
}

func TestExampleConfigIsValid(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "sources.example.yaml"))
	if err != nil {
		t.Fatalf("sources.example.yaml must be valid: %v", err)
	}
	if len(cfg.Sources) == 0 {
		t.Fatal("example config has no sources")
	}
	for _, s := range cfg.Sources {
		if s.MaxPages > 200 {
			t.Errorf("example source %s should stay small (max_pages %d)", s.ID, s.MaxPages)
		}
		if len(s.AllowedHosts) > 0 {
			t.Errorf("example source %s should use narrow allowed_prefixes, not allowed_hosts", s.ID)
		}
		if !cfg.RespectsRobots(&s) {
			t.Errorf("example source %s must respect robots.txt", s.ID)
		}
	}
}

func TestDefaultMaxPages(t *testing.T) {
	cfg, err := Parse([]byte(strings.Replace(validYAML, "    max_pages: 50\n", "", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Sources[0].MaxPages; got != DefaultMaxPages {
		t.Errorf("max_pages = %d, want %d", got, DefaultMaxPages)
	}
}

func TestValidationErrors(t *testing.T) {
	cases := []struct {
		name, yaml, want string
	}{
		{"empty", ``, "empty"},
		{"no sources", "sources: []\n", "no sources defined"},
		{"unknown key (typo)", strings.Replace(validYAML, "allowed_prefixes", "alowed_prefixes", 1), "alowed_prefixes"},
		{"missing boundaries", `
sources:
  - id: a
    start_urls: [https://example.com/]
`, "at least one of allowed_prefixes or allowed_hosts"},
		{"bad id", strings.Replace(validYAML, "example-docs", "Example Docs", 1), "must be 1-63 characters"},
		{"duplicate id", validYAML + `
  - id: example-docs
    start_urls: [https://example.com/docs/]
    allowed_prefixes: [https://example.com/docs/]
`, "duplicate id"},
		{"start outside", `
sources:
  - id: a
    start_urls: [https://other.example.org/]
    allowed_prefixes: [https://example.com/docs/]
`, "outside this source's boundaries"},
		{"ftp scheme", `
sources:
  - id: a
    start_urls: [ftp://example.com/]
    allowed_hosts: [example.com]
`, "must start with http:// or https://"},
		{"credentials", `
sources:
  - id: a
    start_urls: [https://user:pw@example.com/]
    allowed_hosts: [example.com]
`, "must not contain credentials"},
		{"host given as URL", `
sources:
  - id: a
    start_urls: [https://example.com/]
    allowed_hosts: [https://example.com/]
`, "bare host name"},
		{"wildcard host", `
sources:
  - id: a
    start_urls: [https://example.com/]
    allowed_hosts: ["*.example.com"]
`, "wildcards are not supported"},
		{"too many pages", strings.Replace(validYAML, "max_pages: 50", "max_pages: 999999", 1), "max_pages must be between"},
		{"bad regex", validYAML + "    exclude_patterns: ['(']\n", "not a valid regular expression"},
		{"concurrency", "settings:\n  concurrency: 100\n" + validYAML, "concurrency must be between"},
		{"no start urls", `
sources:
  - id: a
    allowed_hosts: [example.com]
`, "start_urls must contain at least one URL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml))
			if err == nil {
				t.Fatalf("expected error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

func TestLoadMissingFileHintsAboutTxtExtension(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sources.yaml")
	if err := os.WriteFile(p+".txt", []byte(validYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(p)
	if err == nil || !strings.Contains(err.Error(), "sources.example.yaml") || !strings.Contains(err.Error(), ".txt") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRobotsOverride(t *testing.T) {
	cfg, err := Parse([]byte("settings:\n  respect_robots_txt: false\n" + validYAML))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RespectsRobots(&cfg.Sources[0]) {
		t.Error("explicit global override should disable robots.txt")
	}
	cfg, err = Parse([]byte(validYAML + "    respect_robots_txt: false\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RespectsRobots(&cfg.Sources[0]) {
		t.Error("explicit per-source override should disable robots.txt")
	}
}

func mustSource(t *testing.T, yaml string) *Source {
	t.Helper()
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	return &cfg.Sources[0]
}

func TestAllowsPrefixBoundaries(t *testing.T) {
	src := mustSource(t, `
sources:
  - id: a
    start_urls: [https://Example.com:443/docs/]
    allowed_prefixes: [HTTPS://EXAMPLE.COM/docs/]
    exclude_patterns: ['/docs/private/', '\.zip$']
`)
	cases := map[string]bool{
		"https://example.com/docs/":                  true,
		"https://example.com/docs/guide/intro.html":  true,
		"https://EXAMPLE.com/docs/a":                 true,
		"https://example.com:443/docs/a":             true,
		"https://example.com/docs":                   false, // prefix ends with "/"
		"https://example.com/blog/":                  false,
		"http://example.com/docs/":                   false, // different scheme
		"https://example.com:8443/docs/":             false, // different port
		"https://example.com.evil.net/docs/":         false, // host suffix attack
		"https://evil.net/https://example.com/docs/": false,
		"https://user@example.com/docs/":             false,
		"https://example.com/docs/private/x":         false, // excluded
		"https://example.com/docs/file.zip":          false, // excluded
		"ftp://example.com/docs/":                    false,
		"https://sub.example.com/docs/":              false,
	}
	for raw, want := range cases {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got, reason := src.Allows(u); got != want {
			t.Errorf("Allows(%s) = %v (%s), want %v", raw, got, reason, want)
		}
	}
}

func TestAllowsHostsAndInclude(t *testing.T) {
	src := mustSource(t, `
sources:
  - id: a
    start_urls: [http://docs.internal:8080/start]
    allowed_hosts: [docs.internal:8080, wiki.internal]
    include_patterns: ['^https?://[^/]+/(start|api/)']
`)
	cases := map[string]bool{
		"http://docs.internal:8080/start":  true,
		"http://docs.internal:8080/api/v1": true,
		"http://docs.internal:8080/other":  false, // fails include
		"http://docs.internal/api/v1":      false, // port differs
		"https://wiki.internal/api/":       true,
		"https://wiki.internal:443/api/":   true,
		"https://wiki.internal.evil/api/":  false,
		"https://other.internal/api/":      false,
	}
	for raw, want := range cases {
		u, _ := url.Parse(raw)
		if got, reason := src.Allows(u); got != want {
			t.Errorf("Allows(%s) = %v (%s), want %v", raw, got, reason, want)
		}
	}
}
