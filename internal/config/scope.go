package config

import (
	"net/url"
	"strings"
)

// Allows reports whether the crawler may fetch u for this source, and if
// not, a short human-readable reason. u should already be normalized (see
// package urlnorm); Allows re-canonicalizes scheme and host defensively.
//
// The rules are, in order:
//  1. only http and https URLs without embedded credentials;
//  2. the URL must be inside at least one allowed_prefixes entry (compared
//     on the full scheme://host/path string, so the host must match
//     exactly) OR its host must exactly equal an allowed_hosts entry;
//  3. if include_patterns are configured, at least one must match;
//  4. no exclude_patterns entry may match.
func (src *Source) Allows(u *url.URL) (bool, string) {
	if u == nil {
		return false, "empty URL"
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return false, "not an http(s) URL"
	}
	if u.User != nil {
		return false, "URL contains credentials"
	}
	if u.Hostname() == "" {
		return false, "URL has no host"
	}
	host := canonicalHost(scheme, u.Host)

	c := *u
	c.Scheme = scheme
	c.Host = host
	c.Fragment, c.RawFragment = "", ""
	full := c.String()
	c.RawQuery, c.ForceQuery = "", false
	if c.Path == "" {
		c.Path, c.RawPath = "/", ""
	}
	noQuery := c.String()

	inside := false
	for _, h := range src.AllowedHosts {
		if h == host {
			inside = true
			break
		}
	}
	if !inside {
		for _, p := range src.AllowedPrefixes {
			if strings.HasPrefix(noQuery, p) {
				inside = true
				break
			}
		}
	}
	if !inside {
		return false, "not under any allowed_prefixes or allowed_hosts entry"
	}
	if len(src.includeRE) > 0 {
		matched := false
		for _, re := range src.includeRE {
			if re.MatchString(full) {
				matched = true
				break
			}
		}
		if !matched {
			return false, "does not match any include_patterns entry"
		}
	}
	for _, re := range src.excludeRE {
		if re.MatchString(full) {
			return false, "matches exclude pattern " + re.String()
		}
	}
	return true, ""
}
