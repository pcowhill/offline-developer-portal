// Package robots implements a small, conservative robots.txt parser.
//
// It supports User-agent groups, Allow and Disallow rules with the "*"
// and "$" wildcards (longest match wins, Allow wins ties), and Crawl-delay.
package robots

import (
	"bufio"
	"io"
	"strconv"
	"strings"
	"time"
)

// Rules are the robots.txt rules that apply to one user agent.
type Rules struct {
	rules       []rule
	CrawlDelay  time.Duration
	disallowAll bool
}

type rule struct {
	allow   bool
	pattern string
}

// AllowAll returns rules that permit everything (used when a site has no
// robots.txt).
func AllowAll() *Rules { return &Rules{} }

// DisallowAll returns rules that forbid everything.
func DisallowAll() *Rules { return &Rules{disallowAll: true} }

type group struct {
	agents     []string
	rules      []rule
	crawlDelay time.Duration
}

// Parse parses robots.txt content and returns the rules for the given
// product token (for example "OfflineDeveloperPortal"). The most specific
// matching group is used; otherwise the "*" group; otherwise allow all.
func Parse(r io.Reader, token string) *Rules {
	token = strings.ToLower(token)
	var groups []*group
	var cur *group
	lastWasAgent := false

	sc := bufio.NewScanner(io.LimitReader(r, 512*1024))
	sc.Buffer(make([]byte, 64*1024), 64*1024)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "user-agent":
			if cur == nil || !lastWasAgent {
				cur = &group{}
				groups = append(groups, cur)
			}
			cur.agents = append(cur.agents, strings.ToLower(value))
			lastWasAgent = true
		case "allow", "disallow":
			lastWasAgent = false
			if cur == nil {
				continue
			}
			if key == "disallow" && value == "" {
				continue // "Disallow:" with no value allows everything
			}
			cur.rules = append(cur.rules, rule{allow: key == "allow", pattern: value})
		case "crawl-delay":
			lastWasAgent = false
			if cur == nil {
				continue
			}
			if f, err := strconv.ParseFloat(value, 64); err == nil && f > 0 {
				cur.crawlDelay = time.Duration(f * float64(time.Second))
			}
		default:
			lastWasAgent = false
		}
	}

	var best *group
	bestLen := -1
	for _, g := range groups {
		for _, a := range g.agents {
			switch {
			case a == "*":
				if bestLen < 0 {
					best, bestLen = g, 0
				}
			case a != "" && strings.Contains(token, a):
				if len(a) > bestLen {
					best, bestLen = g, len(a)
				}
			}
		}
	}
	if best == nil {
		return AllowAll()
	}
	return &Rules{rules: best.rules, CrawlDelay: best.crawlDelay}
}

// Allowed reports whether the given path (with optional query string) may
// be fetched.
func (r *Rules) Allowed(pathAndQuery string) bool {
	if r == nil {
		return true
	}
	if r.disallowAll {
		return false
	}
	if pathAndQuery == "" {
		pathAndQuery = "/"
	}
	if pathAndQuery == "/robots.txt" {
		return true
	}
	bestLen := -1
	allowed := true
	for _, rl := range r.rules {
		if !match(rl.pattern, pathAndQuery) {
			continue
		}
		l := len(rl.pattern)
		if l > bestLen || (l == bestLen && rl.allow) {
			bestLen = l
			allowed = rl.allow
		}
	}
	return allowed
}

// match implements robots.txt pattern matching with "*" (any sequence)
// and a trailing "$" (end anchor).
func match(pattern, p string) bool {
	anchored := strings.HasSuffix(pattern, "$")
	if anchored {
		pattern = strings.TrimSuffix(pattern, "$")
	}
	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(p, parts[0]) {
		return false
	}
	pos := len(parts[0])
	for i := 1; i < len(parts); i++ {
		part := parts[i]
		if i == len(parts)-1 && anchored {
			return len(p)-pos >= len(part) && strings.HasSuffix(p, part)
		}
		idx := strings.Index(p[pos:], part)
		if idx < 0 {
			return false
		}
		pos += idx + len(part)
	}
	if anchored {
		return pos == len(p)
	}
	return true
}
