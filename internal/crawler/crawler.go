// Package crawler fetches the pages of one configured source, staying
// strictly inside that source's configured boundaries.
//
// Safety properties:
//   - only GET requests are made (robots.txt and pages);
//   - every URL, including every redirect target, must pass
//     config.Source.Allows before it is requested;
//   - robots.txt is honoured unless explicitly disabled in sources.yaml;
//   - the number of page requests never exceeds max_pages;
//   - requests to one host are spaced by the configured delay (or the
//     site's Crawl-delay, if larger);
//   - JavaScript is never executed and credentials are never sent.
package crawler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/pcowhill/offline-developer-portal/internal/config"
	"github.com/pcowhill/offline-developer-portal/internal/extract"
	"github.com/pcowhill/offline-developer-portal/internal/index"
	"github.com/pcowhill/offline-developer-portal/internal/robots"
	"github.com/pcowhill/offline-developer-portal/internal/urlnorm"
)

// RobotsToken is the product token matched against robots.txt groups.
const RobotsToken = "OfflineDeveloperPortal"

const (
	maxRedirects    = 5
	maxCrawlDelay   = 30 * time.Second
	maxErrorSamples = 20
	retryDelay      = 2 * time.Second
)

// UserAgent builds the User-Agent header.
func UserAgent(version, contact string) string {
	ua := fmt.Sprintf("%s/%s (+offline documentation indexer; respects robots.txt)", RobotsToken, version)
	if contact = strings.TrimSpace(contact); contact != "" {
		ua = fmt.Sprintf("%s/%s (+offline documentation indexer; respects robots.txt; contact: %s)", RobotsToken, version, contact)
	}
	return ua
}

// Options configure a crawl.
type Options struct {
	Config    *config.Config
	UserAgent string
	// Log receives human-readable progress lines. May be nil.
	Log func(format string, args ...any)
	// Transport overrides the HTTP transport (used by tests).
	Transport http.RoundTripper
	// Now overrides the clock (used by tests).
	Now func() time.Time
}

// Result summarizes the crawl of one source.
type Result struct {
	Source       *config.Source
	Documents    []*index.Document
	PagesFetched int
	PagesSkipped int
	Errors       int
	ErrorSamples []string
	OutOfScope   int
	StartedAt    time.Time
	FinishedAt   time.Time
	RobotsTxt    bool
}

type queued struct {
	u     *url.URL
	depth int
}

type fetched struct {
	item      queued
	finalURL  *url.URL
	page      *extract.Page
	skipped   string
	err       error
	status    int
	robotsHit bool
}

type crawl struct {
	opts      Options
	src       *config.Source
	client    *http.Client
	urlOpts   urlnorm.Options
	respect   bool
	log       func(format string, args ...any)
	robots    map[string]*robots.Rules
	robotsMu  sync.Mutex
	ctx       context.Context
	addError  func(string)
	limiter   *hostLimiter
	maxBytes  int64
	userAgent string
}

// Crawl fetches the pages of src. It returns when the frontier is empty,
// max_pages requests have been made, or ctx is cancelled (in which case
// ctx.Err() is returned alongside the partial result).
func Crawl(ctx context.Context, opts Options, src *config.Source) (*Result, error) {
	cfg := opts.Config
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	logf := opts.Log
	if logf == nil {
		logf = func(string, ...any) {}
	}
	c := &crawl{
		opts:      opts,
		src:       src,
		urlOpts:   urlnorm.Options{KeepQuery: src.KeepQueryString},
		respect:   cfg.RespectsRobots(src),
		log:       logf,
		robots:    map[string]*robots.Rules{},
		limiter:   newHostLimiter(cfg.RequestDelay()),
		maxBytes:  cfg.Settings.MaxPageBytes,
		userAgent: opts.UserAgent,
	}
	transport := opts.Transport
	if transport == nil {
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.MaxIdleConnsPerHost = cfg.Settings.Concurrency
		t.ResponseHeaderTimeout = cfg.RequestTimeout()
		transport = t
	}
	c.client = &http.Client{
		Transport:     transport,
		Timeout:       cfg.RequestTimeout(),
		CheckRedirect: c.checkRedirect,
	}

	res := &Result{Source: src, StartedAt: now(), RobotsTxt: c.respect}
	defer func() { res.FinishedAt = now() }()
	addError := func(msg string) {
		res.Errors++
		if len(res.ErrorSamples) < maxErrorSamples {
			res.ErrorSamples = append(res.ErrorSamples, msg)
		}
	}

	if !c.respect {
		logf("[%s] WARNING: robots.txt is NOT being respected for this source (explicitly disabled in sources.yaml).", src.ID)
	}

	c.ctx, c.addError = ctx, addError
	// Fetch robots.txt up front for the origins of the start URLs and
	// prefixes; other allowed hosts are handled lazily on first use.
	for _, origin := range originsOf(src) {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		c.robotsFor(origin)
	}

	seen := map[string]bool{}
	contentSeen := map[[32]byte]bool{}
	docURLs := map[string]bool{}
	var queue []queued
	for _, raw := range src.StartURLs {
		u, err := urlnorm.Parse(raw, c.urlOpts)
		if err != nil {
			addError(fmt.Sprintf("start URL %s: %v", raw, err))
			continue
		}
		if !seen[u.String()] {
			seen[u.String()] = true
			queue = append(queue, queued{u: u})
		}
	}

	results := make(chan fetched)
	inflight, requested := 0, 0
	conc := cfg.Settings.Concurrency
	var cancelled error
	for {
		for cancelled == nil && len(queue) > 0 && inflight < conc && requested < src.MaxPages {
			it := queue[0]
			queue = queue[1:]
			if !c.robotsAllowed(it.u) {
				res.PagesSkipped++
				logf("[%s] SKIP  %s (disallowed by robots.txt)", src.ID, it.u)
				continue
			}
			requested++
			inflight++
			go func(it queued) { results <- c.fetch(ctx, it) }(it)
		}
		if inflight == 0 {
			break
		}
		r := <-results
		inflight--
		if cancelled == nil && ctx.Err() != nil {
			cancelled = ctx.Err()
		}
		progress := fmt.Sprintf("[%s] %d/%d", src.ID, requested-inflight, src.MaxPages)
		switch {
		case r.err != nil:
			if cancelled != nil && errors.Is(r.err, context.Canceled) {
				continue
			}
			addError(fmt.Sprintf("%s: %v", r.item.u, r.err))
			logf("%s ERROR %s: %v", progress, r.item.u, r.err)
			continue
		case r.skipped != "":
			res.PagesSkipped++
			logf("%s SKIP  %s (%s)", progress, r.item.u, r.skipped)
			continue
		}
		res.PagesFetched++
		final := r.finalURL.String()
		if final != r.item.u.String() {
			if docURLs[final] {
				res.PagesSkipped++
				logf("%s SKIP  %s (redirects to already indexed %s)", progress, r.item.u, final)
				continue
			}
			seen[final] = true
		}

		page := r.page
		if !(c.respect && page.NoFollow) {
			for _, link := range page.Links {
				key := link.String()
				if seen[key] {
					continue
				}
				if ok, _ := src.Allows(link); !ok {
					res.OutOfScope++
					continue
				}
				if urlnorm.LikelyNonHTML(link) {
					continue
				}
				if src.MaxDepth > 0 && r.item.depth+1 > src.MaxDepth {
					continue
				}
				seen[key] = true
				queue = append(queue, queued{u: link, depth: r.item.depth + 1})
			}
		}

		if c.respect && page.NoIndex {
			res.PagesSkipped++
			logf("%s SKIP  %s (page asks not to be indexed: meta robots noindex)", progress, final)
			continue
		}
		if len(page.Blocks) == 0 {
			res.PagesSkipped++
			logf("%s SKIP  %s (no readable content)", progress, final)
			continue
		}
		hash := contentHash(page)
		if contentSeen[hash] {
			res.PagesSkipped++
			logf("%s SKIP  %s (duplicate content)", progress, final)
			continue
		}
		contentSeen[hash] = true
		docURLs[final] = true

		doc := &index.Document{
			ID:          index.DocumentID(src.ID, final),
			SourceID:    src.ID,
			SourceName:  src.Name,
			URL:         final,
			Title:       page.Title,
			Description: page.Description,
			IndexedAt:   now().UTC().Truncate(time.Second),
			Headings:    page.Headings,
			Blocks:      page.Blocks,
		}
		if doc.Headings == nil {
			doc.Headings = []index.Heading{}
		}
		res.Documents = append(res.Documents, doc)
		logf("%s OK    %s - %s", progress, final, page.Title)
	}
	if cancelled != nil {
		return res, cancelled
	}
	if requested >= src.MaxPages && len(queue) > 0 {
		logf("[%s] Reached max_pages (%d); %d more in-scope links were not fetched. Raise max_pages to index more.", src.ID, src.MaxPages, len(queue))
	}
	return res, nil
}

func contentHash(p *extract.Page) [32]byte {
	b, _ := json.Marshal(p.Blocks)
	return sha256.Sum256(b)
}

// originsOf returns the scheme://host origins that a source can reach.
func originsOf(src *config.Source) []string {
	seen := map[string]bool{}
	var out []string
	add := func(o string) {
		if !seen[o] {
			seen[o] = true
			out = append(out, o)
		}
	}
	for _, p := range src.AllowedPrefixes {
		if u, err := url.Parse(p); err == nil {
			add(u.Scheme + "://" + u.Host)
		}
	}
	for _, raw := range src.StartURLs {
		if u, err := urlnorm.Parse(raw, urlnorm.Options{}); err == nil {
			add(u.Scheme + "://" + u.Host)
		}
	}
	return out
}

// robotsFor returns the robots.txt rules for origin, fetching them on
// first use. It is safe for concurrent use.
func (c *crawl) robotsFor(origin string) *robots.Rules {
	c.robotsMu.Lock()
	defer c.robotsMu.Unlock()
	if r, ok := c.robots[origin]; ok {
		return r
	}
	if !c.respect {
		c.robots[origin] = robots.AllowAll()
		return c.robots[origin]
	}
	rules, err := c.fetchRobots(c.ctx, origin)
	if err != nil {
		c.log("[%s] robots.txt for %s could not be read (%v); not crawling that host. Set respect_robots_txt: false for this source only if you are authorized to crawl it.", c.src.ID, origin, err)
		c.addError(fmt.Sprintf("%s/robots.txt: %v", origin, err))
		rules = robots.DisallowAll()
	}
	if rules.CrawlDelay > 0 {
		d := rules.CrawlDelay
		if d > maxCrawlDelay {
			d = maxCrawlDelay
		}
		c.limiter.setMinDelay(hostOf(origin), d)
		c.log("[%s] %s asks for a crawl delay of %s", c.src.ID, origin, d)
	}
	c.robots[origin] = rules
	return rules
}

func hostOf(origin string) string {
	if u, err := url.Parse(origin); err == nil {
		return u.Host
	}
	return origin
}

func (c *crawl) fetchRobots(ctx context.Context, origin string) (*robots.Rules, error) {
	u := origin + "/robots.txt"
	c.limiter.wait(ctx, hostOf(origin))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	// robots.txt redirects are followed only within the same host.
	client := *c.client
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects || r.URL.Host != via[0].URL.Host {
			return http.ErrUseLastResponse
		}
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return robots.AllowAll(), nil
		}
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return robots.Parse(resp.Body, RobotsToken), nil
	case resp.StatusCode >= 300 && resp.StatusCode < 500:
		// No usable robots.txt: everything is allowed.
		return robots.AllowAll(), nil
	default:
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
}

func (c *crawl) robotsAllowed(u *url.URL) bool {
	if !c.respect {
		return true
	}
	rules := c.robotsFor(u.Scheme + "://" + u.Host)
	p := u.EscapedPath()
	if u.RawQuery != "" {
		p += "?" + u.RawQuery
	}
	return rules.Allowed(p)
}

func (c *crawl) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	u, err := urlnorm.Normalize(req.URL, c.urlOpts)
	if err != nil {
		return fmt.Errorf("redirect to unsupported URL %s", req.URL.Redacted())
	}
	if ok, reason := c.src.Allows(u); !ok {
		return &refusedError{fmt.Sprintf("redirect to %s refused: %s", u, reason)}
	}
	if !c.robotsAllowed(u) {
		return &refusedError{fmt.Sprintf("redirect to %s refused: disallowed by robots.txt", u)}
	}
	req.Header.Del("Authorization")
	req.Header.Del("Cookie")
	return nil
}

func (c *crawl) fetch(ctx context.Context, it queued) fetched {
	r := c.fetchOnce(ctx, it)
	if r.err != nil && ctx.Err() == nil && retryable(r) {
		select {
		case <-time.After(retryDelay):
		case <-ctx.Done():
			return r
		}
		r = c.fetchOnce(ctx, it)
	}
	return r
}

func retryable(r fetched) bool {
	if r.status == http.StatusTooManyRequests || r.status >= 500 {
		return true
	}
	if r.status == 0 {
		var refused *refusedError
		return !errors.As(r.err, &refused)
	}
	return false
}

// refusedError marks a request the crawler declined to make.
type refusedError struct{ msg string }

func (e *refusedError) Error() string { return e.msg }

func (c *crawl) fetchOnce(ctx context.Context, it queued) fetched {
	out := fetched{item: it}
	c.limiter.wait(ctx, it.u.Host)
	if err := ctx.Err(); err != nil {
		out.err = err
		return out
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, it.u.String(), nil)
	if err != nil {
		out.err = err
		return out
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.1")
	resp, err := c.client.Do(req)
	if err != nil {
		out.err = cleanErr(err)
		return out
	}
	defer resp.Body.Close()
	out.status = resp.StatusCode
	if resp.StatusCode != http.StatusOK {
		out.err = fmt.Errorf("HTTP %s", resp.Status)
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
		return out
	}
	final, err := urlnorm.Normalize(resp.Request.URL, c.urlOpts)
	if err != nil {
		out.err = err
		return out
	}
	out.finalURL = final

	ct := resp.Header.Get("Content-Type")
	head := make([]byte, 512)
	n, _ := io.ReadFull(resp.Body, head)
	head = head[:n]
	if !isHTML(ct, head) {
		out.skipped = "not HTML: " + firstNonEmpty(ct, http.DetectContentType(head))
		return out
	}
	rest, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBytes-int64(n)+1))
	if err != nil {
		out.err = cleanErr(err)
		return out
	}
	body := append(head, rest...)
	if int64(len(body)) > c.maxBytes {
		out.skipped = fmt.Sprintf("larger than max_page_bytes (%d bytes)", c.maxBytes)
		return out
	}
	page, err := extract.Extract(body, final, extract.Options{URLOptions: c.urlOpts})
	if err != nil {
		out.err = fmt.Errorf("parsing HTML: %w", err)
		return out
	}
	out.page = page
	return out
}

func cleanErr(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

func isHTML(contentType string, head []byte) bool {
	if contentType != "" {
		mt, _, err := mime.ParseMediaType(contentType)
		if err == nil {
			return mt == "text/html" || mt == "application/xhtml+xml"
		}
	}
	return strings.HasPrefix(http.DetectContentType(head), "text/html")
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

// hostLimiter spaces requests to the same host.
type hostLimiter struct {
	mu       sync.Mutex
	delay    time.Duration
	minDelay map[string]time.Duration
	next     map[string]time.Time
}

func newHostLimiter(d time.Duration) *hostLimiter {
	return &hostLimiter{delay: d, minDelay: map[string]time.Duration{}, next: map[string]time.Time{}}
}

func (l *hostLimiter) setMinDelay(host string, d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.minDelay[host] = d
}

func (l *hostLimiter) wait(ctx context.Context, host string) {
	l.mu.Lock()
	d := l.delay
	if md := l.minDelay[host]; md > d {
		d = md
	}
	now := time.Now()
	at := l.next[host]
	if at.Before(now) {
		at = now
	}
	l.next[host] = at.Add(d)
	l.mu.Unlock()
	if wait := time.Until(at); wait > 0 {
		t := time.NewTimer(wait)
		defer t.Stop()
		select {
		case <-t.C:
		case <-ctx.Done():
		}
	}
}
