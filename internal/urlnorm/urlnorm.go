// Package urlnorm normalizes URLs so that trivially different spellings of
// the same page are crawled and indexed only once.
package urlnorm

import (
	"errors"
	"net"
	"net/url"
	"path"
	"strings"
)

// Options control normalization.
type Options struct {
	// KeepQuery keeps the query string (minus tracking parameters, sorted).
	// By default query strings are dropped entirely, which avoids crawling
	// endless ?sort=, ?page= or session variants of the same document.
	KeepQuery bool
}

// Parse parses an absolute http(s) URL and normalizes it.
func Parse(raw string, opts Options) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if !u.IsAbs() {
		return nil, errors.New("URL is not absolute")
	}
	return Normalize(u, opts)
}

// Resolve resolves href relative to base and normalizes the result. It
// returns an error for links that can never be crawled (mailto:, javascript:,
// data:, empty or malformed references).
func Resolve(base *url.URL, href string, opts Options) (*url.URL, error) {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") {
		return nil, errors.New("same-page reference")
	}
	ref, err := url.Parse(href)
	if err != nil {
		return nil, err
	}
	return Normalize(base.ResolveReference(ref), opts)
}

// Normalize returns a canonical copy of u:
//   - scheme and host are lower-cased, default ports removed, trailing dot
//     in the host removed;
//   - the fragment is removed;
//   - "." and ".." segments and duplicate slashes are removed, and an empty
//     path becomes "/";
//   - unnecessary percent-encoding is normalized;
//   - the query is dropped unless opts.KeepQuery is set, in which case
//     tracking parameters are removed and the remainder sorted.
func Normalize(in *url.URL, opts Options) (*url.URL, error) {
	if in == nil {
		return nil, errors.New("nil URL")
	}
	u := *in
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("unsupported scheme " + u.Scheme)
	}
	if u.Opaque != "" {
		return nil, errors.New("opaque URL")
	}
	if u.User != nil {
		return nil, errors.New("URL contains credentials")
	}
	host := strings.ToLower(u.Host)
	hostname, port := host, ""
	if h, p, err := net.SplitHostPort(host); err == nil {
		hostname, port = h, p
	}
	hostname = strings.TrimSuffix(hostname, ".")
	if hostname == "" {
		return nil, errors.New("URL has no host")
	}
	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		u.Host = net.JoinHostPort(hostname, port)
	} else if strings.Contains(hostname, ":") {
		u.Host = "[" + hostname + "]"
	} else {
		u.Host = hostname
	}

	u.Fragment, u.RawFragment = "", ""
	u.Path = cleanPath(u.Path)
	// Keep an explicit RawPath only when it carries meaning that Path cannot
	// (an encoded slash inside a segment); otherwise let Go choose the
	// canonical encoding.
	if !strings.Contains(strings.ToLower(u.RawPath), "%2f") {
		u.RawPath = ""
	} else {
		u.RawPath = cleanPath(u.RawPath)
	}

	u.ForceQuery = false
	if !opts.KeepQuery {
		u.RawQuery = ""
	} else if u.RawQuery != "" {
		q, err := url.ParseQuery(u.RawQuery)
		if err != nil {
			u.RawQuery = ""
		} else {
			for k := range q {
				lk := strings.ToLower(k)
				if strings.HasPrefix(lk, "utm_") || lk == "fbclid" || lk == "gclid" || lk == "ref_src" {
					q.Del(k)
				}
			}
			u.RawQuery = q.Encode()
		}
	}
	return &u, nil
}

func cleanPath(p string) string {
	if p == "" {
		return "/"
	}
	trailing := strings.HasSuffix(p, "/")
	c := path.Clean("/" + p)
	if trailing && c != "/" {
		c += "/"
	}
	return c
}

// nonHTMLExtensions are file types that are never documentation pages.
// Links ending in these are skipped without making a request. Content-Type
// is still checked for every fetched page.
var nonHTMLExtensions = map[string]bool{
	".7z": true, ".apk": true, ".avi": true, ".bin": true, ".bmp": true, ".bz2": true,
	".css": true, ".csv": true, ".deb": true, ".dmg": true, ".doc": true, ".docx": true,
	".eot": true, ".epub": true, ".exe": true, ".gif": true, ".gz": true, ".ico": true,
	".iso": true, ".jar": true, ".jpeg": true, ".jpg": true, ".js": true, ".json": true,
	".map": true, ".mjs": true, ".mov": true, ".mp3": true, ".mp4": true, ".msi": true,
	".ogg": true, ".otf": true, ".pdf": true, ".png": true, ".ppt": true, ".pptx": true,
	".rar": true, ".rpm": true, ".rss": true, ".svg": true, ".tar": true, ".tgz": true,
	".ttf": true, ".txt": true, ".wasm": true, ".wav": true, ".webm": true, ".webp": true,
	".woff": true, ".woff2": true, ".xls": true, ".xlsx": true, ".xml": true, ".xz": true,
	".zip": true, ".atom": true, ".whl": true, ".egg": true, ".py": true, ".ipynb": true,
}

// LikelyNonHTML reports whether the URL path has a file extension that is
// almost certainly not an HTML page.
func LikelyNonHTML(u *url.URL) bool {
	ext := strings.ToLower(path.Ext(u.Path))
	return nonHTMLExtensions[ext]
}
