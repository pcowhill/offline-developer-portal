package robots

import (
	"strings"
	"testing"
	"time"
)

const sample = `
# comment
User-agent: Googlebot
Disallow: /

User-agent: *
Disallow: /private/
Disallow: /*.json$
Allow: /private/public/
Crawl-delay: 2

User-agent: OfflineDeveloperPortal
User-agent: other-bot
Disallow: /no-portal/
Disallow:
`

func TestSpecificGroupWins(t *testing.T) {
	r := Parse(strings.NewReader(sample), "OfflineDeveloperPortal")
	if r.Allowed("/no-portal/x") {
		t.Error("/no-portal/ should be disallowed for our agent")
	}
	if !r.Allowed("/private/x") {
		t.Error("the * group must not apply when a specific group matches")
	}
}

func TestWildcardGroup(t *testing.T) {
	r := Parse(strings.NewReader(sample), "SomeOtherCrawler")
	cases := map[string]bool{
		"/":                    true,
		"/docs/":               true,
		"/private/":            false,
		"/private/secret.html": false,
		"/private/public/a":    true, // longer Allow wins
		"/data/file.json":      false,
		"/data/file.json?x=1":  true, // $ anchors the end
		"/robots.txt":          true,
	}
	for p, want := range cases {
		if got := r.Allowed(p); got != want {
			t.Errorf("Allowed(%q) = %v, want %v", p, got, want)
		}
	}
	if r.CrawlDelay != 2*time.Second {
		t.Errorf("CrawlDelay = %v", r.CrawlDelay)
	}
}

func TestNoMatchingGroupAllowsAll(t *testing.T) {
	r := Parse(strings.NewReader("User-agent: Googlebot\nDisallow: /\n"), "OfflineDeveloperPortal")
	if !r.Allowed("/anything") {
		t.Error("expected allow")
	}
}

func TestEmptyAndSpecialRules(t *testing.T) {
	if !Parse(strings.NewReader(""), "x").Allowed("/a") {
		t.Error("empty robots.txt allows everything")
	}
	if DisallowAll().Allowed("/a") {
		t.Error("DisallowAll must disallow")
	}
	if !AllowAll().Allowed("/a") {
		t.Error("AllowAll must allow")
	}
	r := Parse(strings.NewReader("User-agent: *\nDisallow: /\nAllow: /docs/$\n"), "x")
	if !r.Allowed("/docs/") || r.Allowed("/docs/a") || r.Allowed("/") {
		t.Error("anchored allow handling is wrong")
	}
}
