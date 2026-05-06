package web

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// csrfProtect rejects state-changing requests using two defenses:
//
//  1. A Host header allowlist (only enforced when the server is
//     bound to loopback). The browser must address the request to a
//     loopback hostname (127.0.0.1 / localhost / ::1). This blocks
//     DNS rebinding: a malicious page hosted on attacker.com that
//     rebinds its DNS to 127.0.0.1 and then has the browser POST
//     to attacker.com:<port> would arrive here with Host=attacker.com,
//     which fails the allowlist. Without this check the rest of the
//     CSRF logic would happily pass the request because Origin and
//     Host both name attacker.com.
//
//  2. A same-origin Origin/Referer check. Browsers always include
//     at least one of these headers on POSTs from page contexts;
//     comparing them to r.Host blocks classic CSRF (a malicious
//     page on a *different* origin that POSTs to klim).
//
// Non-loopback binds (--insecure-bind) skip the Host allowlist
// because the user is intentionally exposing the server to a LAN
// hostname; the auto-generated bearer token is the real auth there.
// The Origin/Referer check still applies on every bind type.
func csrfProtect(s *Server, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.opts.Bind == "" || isLoopback(s.opts.Bind) {
			if !isLoopbackHostHeader(r.Host) {
				s.opts.Logger.Warn("web: rejected request with non-loopback Host header (possible DNS rebinding)",
					"path", r.URL.Path,
					"host", r.Host,
				)
				http.Error(w, "host not allowed", http.StatusForbidden)
				return
			}
		}
		if !sameOriginRequest(requestOrigin(r), r) {
			s.opts.Logger.Warn("web: rejected cross-origin request",
				"path", r.URL.Path,
				"host", r.Host,
				"origin", r.Header.Get("Origin"),
				"referer", r.Header.Get("Referer"),
			)
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isLoopbackHostHeader parses an HTTP Host header (which may include
// a port and IPv6 brackets) and reports whether the hostname portion
// is a loopback address. Used to gate the DNS-rebinding mitigation —
// see csrfProtect.
func isLoopbackHostHeader(hostHeader string) bool {
	host, _, err := net.SplitHostPort(hostHeader)
	if err != nil {
		// No port: the whole string is the host.
		host = hostHeader
	}
	return isLoopback(host)
}

// requestOrigin reconstructs the browser-facing origin from the
// request's Host header. Scheme is best-guessed via TLS; "http" on
// loopback is the common case for klim.
func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	return scheme + "://" + r.Host
}

// sameOriginRequest reports whether r's Origin or Referer points at
// expected (the origin the request is actually addressed to).
// Either header matching is enough, because some browsers omit
// Origin on same-origin requests.
func sameOriginRequest(expected string, r *http.Request) bool {
	want, err := url.Parse(expected)
	if err != nil {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "null" {
		got, err := url.Parse(origin)
		if err == nil && originHostMatches(got, want) {
			return true
		}
	}
	if ref := r.Header.Get("Referer"); ref != "" {
		got, err := url.Parse(ref)
		if err == nil && originHostMatches(got, want) {
			return true
		}
	}
	return false
}

// originHostMatches treats 127.0.0.1, localhost, and ::1 as equivalent
// since the user can reach the server via any of those three names
// even when bound to one of them.
func originHostMatches(got, want *url.URL) bool {
	if got == nil || want == nil {
		return false
	}
	if got.Scheme != want.Scheme {
		return false
	}
	if !portMatches(got, want) {
		return false
	}
	gh := strings.ToLower(got.Hostname())
	wh := strings.ToLower(want.Hostname())
	if gh == wh {
		return true
	}
	loops := map[string]bool{"127.0.0.1": true, "localhost": true, "::1": true, "[::1]": true}
	return loops[gh] && loops[wh]
}

func portMatches(got, want *url.URL) bool {
	gp := got.Port()
	wp := want.Port()
	// Strict equality. We never need to treat default ports as
	// equivalent because klim always binds to an explicit, non-
	// standard port (the listener Addr ports are always present in
	// both Origin headers and r.Host), so any mismatch here is
	// genuinely a different origin.
	return gp == wp
}
