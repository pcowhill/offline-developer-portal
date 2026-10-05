// Package config loads and validates the local sources.yaml file.
//
// sources.yaml is runtime-only configuration. It lists the documentation
// sites the indexer may crawl and the explicit boundaries it must stay
// within. It is never needed to build the frontend and must never be
// committed to Git.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Hard limits that cannot be raised from configuration. They exist to
// prevent accidental, uncontrolled crawling.
const (
	MaxPagesHardLimit       = 20000
	MaxConcurrencyHardLimit = 16
	LargeSourceWarning      = 2000
)

// Defaults applied when a value is omitted.
const (
	DefaultMaxPages       = 100
	DefaultConcurrency    = 4
	DefaultRequestDelay   = 250 * time.Millisecond
	DefaultRequestTimeout = 20 * time.Second
	DefaultMaxPageBytes   = 5 * 1024 * 1024
)

// Config is the top-level structure of sources.yaml.
type Config struct {
	Settings Settings `yaml:"settings"`
	Sources  []Source `yaml:"sources"`
}

// Settings are global crawler settings. All fields are optional.
type Settings struct {
	// Concurrency is the number of pages fetched in parallel per source.
	Concurrency int `yaml:"concurrency"`
	// RequestDelayMS is the minimum delay between requests to the same host.
	RequestDelayMS *int `yaml:"request_delay_ms"`
	// RequestTimeoutSeconds bounds each HTTP request.
	RequestTimeoutSeconds int `yaml:"request_timeout_seconds"`
	// MaxPageBytes is the largest HTML body that will be read.
	MaxPageBytes int64 `yaml:"max_page_bytes"`
	// RespectRobotsTxt defaults to true. Setting it to false is an explicit,
	// local decision and is reported loudly at crawl time.
	RespectRobotsTxt *bool `yaml:"respect_robots_txt"`
	// UserAgentContact is appended to the User-Agent (for example an
	// e-mail address or intranet page) so site operators can reach you.
	UserAgentContact string `yaml:"user_agent_contact"`
}

// Source describes one documentation site and its crawl boundaries.
type Source struct {
	ID              string   `yaml:"id"`
	Name            string   `yaml:"name"`
	StartURLs       []string `yaml:"start_urls"`
	AllowedPrefixes []string `yaml:"allowed_prefixes"`
	AllowedHosts    []string `yaml:"allowed_hosts"`
	MaxPages        int      `yaml:"max_pages"`
	MaxDepth        int      `yaml:"max_depth"`
	Include         []string `yaml:"include_patterns"`
	Exclude         []string `yaml:"exclude_patterns"`
	KeepQueryString bool     `yaml:"keep_query_strings"`
	// RespectRobotsTxt overrides the global setting for this source only.
	RespectRobotsTxt *bool `yaml:"respect_robots_txt"`

	includeRE []*regexp.Regexp
	excludeRE []*regexp.Regexp
}

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

// Load reads and validates a configuration file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			hint := ""
			if _, statErr := os.Stat(path + ".txt"); statErr == nil {
				hint = fmt.Sprintf("\n  Found %q instead. Your editor probably added a hidden .txt extension; rename the file to remove it.", path+".txt")
			}
			return nil, fmt.Errorf("configuration file %q not found.\n  Copy sources.example.yaml to sources.yaml and edit it.%s", path, hint)
		}
		return nil, fmt.Errorf("reading %q: %w", path, err)
	}
	cfg, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// Parse decodes and validates YAML configuration. Unknown keys are rejected
// so that typos (for example "alowed_prefixes") cannot silently widen or
// remove crawl boundaries.
func Parse(data []byte) (*Config, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("configuration is empty; define at least one source")
		}
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ValidationError collects every problem found so the user can fix them
// all at once.
type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	return "invalid configuration:\n  - " + strings.Join(e.Problems, "\n  - ")
}

// Validate checks the configuration, applies defaults and compiles patterns.
func (c *Config) Validate() error {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	s := &c.Settings
	if s.Concurrency == 0 {
		s.Concurrency = DefaultConcurrency
	}
	if s.Concurrency < 1 || s.Concurrency > MaxConcurrencyHardLimit {
		add("settings.concurrency must be between 1 and %d", MaxConcurrencyHardLimit)
	}
	if s.RequestDelayMS != nil && (*s.RequestDelayMS < 0 || *s.RequestDelayMS > 60000) {
		add("settings.request_delay_ms must be between 0 and 60000")
	}
	if s.RequestTimeoutSeconds == 0 {
		s.RequestTimeoutSeconds = int(DefaultRequestTimeout / time.Second)
	}
	if s.RequestTimeoutSeconds < 1 || s.RequestTimeoutSeconds > 300 {
		add("settings.request_timeout_seconds must be between 1 and 300")
	}
	if s.MaxPageBytes == 0 {
		s.MaxPageBytes = DefaultMaxPageBytes
	}
	if s.MaxPageBytes < 1024 || s.MaxPageBytes > 100*1024*1024 {
		add("settings.max_page_bytes must be between 1024 and 104857600")
	}
	if strings.ContainsAny(s.UserAgentContact, "\r\n") {
		add("settings.user_agent_contact must be a single line")
	}

	if len(c.Sources) == 0 {
		add("no sources defined; add at least one entry under 'sources:'")
	}
	seen := map[string]bool{}
	for i := range c.Sources {
		src := &c.Sources[i]
		label := fmt.Sprintf("sources[%d]", i)
		if src.ID != "" {
			label = fmt.Sprintf("source %q", src.ID)
		}
		for _, p := range src.validate() {
			add("%s: %s", label, p)
		}
		if src.ID != "" {
			if seen[src.ID] {
				add("%s: duplicate id", label)
			}
			seen[src.ID] = true
		}
	}
	if len(problems) > 0 {
		return &ValidationError{Problems: problems}
	}
	return nil
}

func (src *Source) validate() []string {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	if src.ID == "" {
		add("id is required")
	} else if !idPattern.MatchString(src.ID) {
		add("id %q must be 1-63 characters of lowercase letters, digits, '-' or '_' and start with a letter or digit", src.ID)
	}
	src.Name = strings.TrimSpace(src.Name)
	if src.Name == "" {
		src.Name = src.ID
	}
	if src.MaxPages == 0 {
		src.MaxPages = DefaultMaxPages
	}
	if src.MaxPages < 1 || src.MaxPages > MaxPagesHardLimit {
		add("max_pages must be between 1 and %d", MaxPagesHardLimit)
	}
	if src.MaxDepth < 0 {
		add("max_depth must not be negative (0 means unlimited)")
	}

	if len(src.AllowedPrefixes) == 0 && len(src.AllowedHosts) == 0 {
		add("at least one of allowed_prefixes or allowed_hosts is required; the crawler never guesses its boundaries")
	}
	for j, p := range src.AllowedPrefixes {
		u, err := parseAbsoluteHTTP(p)
		if err != nil {
			add("allowed_prefixes[%d] %q: %v", j, p, err)
			continue
		}
		src.AllowedPrefixes[j] = canonicalPrefix(u)
	}
	for j, h := range src.AllowedHosts {
		norm, err := normalizeHostEntry(h)
		if err != nil {
			add("allowed_hosts[%d] %q: %v", j, h, err)
			continue
		}
		src.AllowedHosts[j] = norm
	}

	src.includeRE = src.includeRE[:0]
	for j, p := range src.Include {
		re, err := regexp.Compile(p)
		if err != nil {
			add("include_patterns[%d] %q is not a valid regular expression: %v", j, p, err)
			continue
		}
		src.includeRE = append(src.includeRE, re)
	}
	src.excludeRE = src.excludeRE[:0]
	for j, p := range src.Exclude {
		re, err := regexp.Compile(p)
		if err != nil {
			add("exclude_patterns[%d] %q is not a valid regular expression: %v", j, p, err)
			continue
		}
		src.excludeRE = append(src.excludeRE, re)
	}

	if len(src.StartURLs) == 0 {
		add("start_urls must contain at least one URL")
	}
	if len(problems) > 0 {
		return problems
	}
	for j, raw := range src.StartURLs {
		u, err := parseAbsoluteHTTP(raw)
		if err != nil {
			add("start_urls[%d] %q: %v", j, raw, err)
			continue
		}
		if ok, reason := src.Allows(u); !ok {
			add("start_urls[%d] %q is outside this source's boundaries (%s)", j, raw, reason)
		}
	}
	return problems
}

// RespectsRobots reports whether robots.txt should be honoured for src.
func (c *Config) RespectsRobots(src *Source) bool {
	if src.RespectRobotsTxt != nil {
		return *src.RespectRobotsTxt
	}
	if c.Settings.RespectRobotsTxt != nil {
		return *c.Settings.RespectRobotsTxt
	}
	return true
}

// RequestDelay returns the minimum delay between requests to one host.
func (c *Config) RequestDelay() time.Duration {
	if c.Settings.RequestDelayMS == nil {
		return DefaultRequestDelay
	}
	return time.Duration(*c.Settings.RequestDelayMS) * time.Millisecond
}

// RequestTimeout returns the per-request timeout.
func (c *Config) RequestTimeout() time.Duration {
	return time.Duration(c.Settings.RequestTimeoutSeconds) * time.Second
}

// Source returns the source with the given id, or nil.
func (c *Config) Source(id string) *Source {
	for i := range c.Sources {
		if c.Sources[i].ID == id {
			return &c.Sources[i]
		}
	}
	return nil
}

func parseAbsoluteHTTP(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("must not be empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("not a valid URL: %v", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("must start with http:// or https://")
	}
	if u.Host == "" || u.Hostname() == "" {
		return nil, errors.New("must include a host name")
	}
	if u.User != nil {
		return nil, errors.New("must not contain credentials (user:password@); authentication is not supported")
	}
	return u, nil
}

// canonicalPrefix normalizes an allowed prefix so that scheme and host
// comparisons are exact. The query and fragment are dropped.
func canonicalPrefix(u *url.URL) string {
	n := *u
	n.Scheme = strings.ToLower(n.Scheme)
	n.Host = canonicalHost(n.Scheme, n.Host)
	n.RawQuery = ""
	n.Fragment = ""
	n.RawFragment = ""
	if n.Path == "" {
		n.Path = "/"
		n.RawPath = ""
	}
	return n.String()
}

func normalizeHostEntry(h string) (string, error) {
	h = strings.TrimSpace(strings.ToLower(h))
	if h == "" {
		return "", errors.New("must not be empty")
	}
	if strings.Contains(h, "://") || strings.ContainsAny(h, "/?#@") {
		return "", errors.New("must be a bare host name such as docs.example.com (optionally with :port), not a URL")
	}
	if strings.Contains(h, "*") {
		return "", errors.New("wildcards are not supported; list each host explicitly")
	}
	host := h
	if hh, port, err := net.SplitHostPort(h); err == nil {
		if port == "" {
			return "", errors.New("empty port")
		}
		host = hh
	}
	if host == "" {
		return "", errors.New("must include a host name")
	}
	return h, nil
}

// canonicalHost lower-cases the host and strips the scheme's default port.
func canonicalHost(scheme, host string) string {
	host = strings.ToLower(host)
	if h, port, err := net.SplitHostPort(host); err == nil {
		if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
			if strings.Contains(h, ":") {
				return "[" + h + "]"
			}
			return h
		}
	}
	return host
}
