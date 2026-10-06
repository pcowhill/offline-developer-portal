package urlnorm

import (
	"net/url"
	"testing"
)

func TestNormalize(t *testing.T) {
	cases := []struct {
		in, want string
		keep     bool
	}{
		{"HTTPS://Example.COM:443/Docs/", "https://example.com/Docs/", false},
		{"http://example.com:80", "http://example.com/", false},
		{"http://example.com:8080/a", "http://example.com:8080/a", false},
		{"https://example.com/a/./b/../c", "https://example.com/a/c", false},
		{"https://example.com/a//b///c/", "https://example.com/a/b/c/", false},
		{"https://example.com/a#section", "https://example.com/a", false},
		{"https://example.com/a?b=1&a=2", "https://example.com/a", false},
		{"https://example.com/a?b=1&a=2&utm_source=x#f", "https://example.com/a?a=2&b=1", true},
		{"https://example.com/a?", "https://example.com/a", true},
		{"https://example.com./x", "https://example.com/x", false},
		{"https://example.com/%7Euser/", "https://example.com/~user/", false},
		{"https://example.com/a%2Fb", "https://example.com/a%2Fb", false},
		{"https://example.com/with space", "https://example.com/with%20space", false},
		{"https://[::1]:443/x", "https://[::1]/x", false},
	}
	for _, tc := range cases {
		got, err := Parse(tc.in, Options{KeepQuery: tc.keep})
		if err != nil {
			t.Errorf("Parse(%q): %v", tc.in, err)
			continue
		}
		if got.String() != tc.want {
			t.Errorf("Parse(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeRejects(t *testing.T) {
	for _, in := range []string{"mailto:a@example.com", "javascript:alert(1)", "ftp://example.com/", "https://user:pw@example.com/", "/relative", "data:text/html,hi"} {
		if u, err := Parse(in, Options{}); err == nil {
			t.Errorf("Parse(%q) = %v, want error", in, u)
		}
	}
}

func TestResolve(t *testing.T) {
	base, _ := url.Parse("https://example.com/docs/guide/page.html")
	cases := map[string]string{
		"other.html":            "https://example.com/docs/guide/other.html",
		"../index.html#top":     "https://example.com/docs/index.html",
		"/root":                 "https://example.com/root",
		"//cdn.example.org/x":   "https://cdn.example.org/x",
		"https://example.com/z": "https://example.com/z",
		"?page=2":               "https://example.com/docs/guide/page.html",
	}
	for href, want := range cases {
		got, err := Resolve(base, href, Options{})
		if err != nil {
			t.Errorf("Resolve(%q): %v", href, err)
			continue
		}
		if got.String() != want {
			t.Errorf("Resolve(%q) = %q, want %q", href, got, want)
		}
	}
	for _, href := range []string{"", "#frag", "mailto:x@y", "javascript:void(0)", "tel:123"} {
		if _, err := Resolve(base, href, Options{}); err == nil {
			t.Errorf("Resolve(%q) should fail", href)
		}
	}
}

func TestLikelyNonHTML(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://e.com/a.pdf":      true,
		"https://e.com/a.PNG":      true,
		"https://e.com/a.tar.gz":   true,
		"https://e.com/a.html":     false,
		"https://e.com/a/":         false,
		"https://e.com/a":          false,
		"https://e.com/v1.2/guide": false,
	} {
		u, _ := url.Parse(raw)
		if got := LikelyNonHTML(u); got != want {
			t.Errorf("LikelyNonHTML(%s) = %v, want %v", raw, got, want)
		}
	}
}
