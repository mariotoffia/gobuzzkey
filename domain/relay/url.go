// Package relay models what gobuzzkey knows about a Buzz relay: its HTTP base
// URL, the channels it exposes and the results of writes.
package relay

import (
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/mariotoffia/gobuzzkey/domain/shared"
)

// URL is a relay base URL: scheme and authority only, normalized to http or
// https. Buzz checks the NIP-98 "u" tag against scheme://<host>/<route>, so a
// relay URL with a path prefix could never authenticate and is rejected.
//
// value-object
type URL struct{ base string }

// ParseURL accepts ws, wss, http or https with a host and optional port. ws
// becomes http and wss becomes https; a single trailing slash is dropped.
// A path, query, fragment or user info is rejected, and the host may contain
// only letters, digits, '.' and '-' (or be a bracketed IPv6 literal).
func ParseURL(s string) (URL, error) {
	s = strings.TrimSpace(s)
	invalid := func(why string) error {
		return shared.New(shared.CodeInvalidRelayURL, "relay URL "+strconv.Quote(s)+" "+why)
	}
	u, err := url.Parse(s)
	if err != nil {
		return URL{}, shared.Wrap(shared.CodeInvalidRelayURL, "relay URL "+strconv.Quote(s), err)
	}
	scheme, ok := httpScheme(u.Scheme)
	if !ok {
		return URL{}, invalid("must start with ws://, wss://, http:// or https://")
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return URL{}, invalid("must not carry user info, a query or a fragment")
	}
	if u.Path != "" && u.Path != "/" {
		return URL{}, invalid("must not have a path: Buzz serves /query and /events at the host root")
	}
	if why := authorityProblem(u); why != "" {
		return URL{}, invalid(why)
	}
	return URL{base: scheme + "://" + u.Host}, nil
}

// httpScheme maps ws/wss/http/https (any case) to http or https.
func httpScheme(s string) (string, bool) {
	switch strings.ToLower(s) {
	case "wss", "https":
		return "https", true
	case "ws", "http":
		return "http", true
	default:
		return "", false
	}
}

// authorityProblem describes what is wrong with the host and port, if anything.
func authorityProblem(u *url.URL) string {
	if !validHost(u.Hostname()) {
		return "has an invalid host"
	}
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 || (len(p) > 1 && p[0] == '0') {
			return "has an invalid port"
		}
	}
	return ""
}

func validHost(h string) bool {
	if h == "" {
		return false
	}
	if strings.Contains(h, ":") { // IPv6 literal, brackets already removed by Hostname
		return net.ParseIP(h) != nil
	}
	for i := range len(h) {
		c := h[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '.' && c != '-' {
			return false
		}
	}
	return true
}

// String returns the normalized base URL.
func (u URL) String() string { return u.base }

// Endpoint returns base + path, e.g. Endpoint("/query").
func (u URL) Endpoint(path string) string { return u.base + path }

// IsZero reports whether u is unset.
func (u URL) IsZero() bool { return u.base == "" }
