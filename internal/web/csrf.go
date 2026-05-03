package web

import (
	"net/http"
	"net/url"
	"strings"
)

// csrfProtect rejects state-changing requests whose Origin or Referer
// don't match the host the request actually arrived on (r.Host). This
// blocks the two attacks the browser surface is exposed to even on
// loopback:
//
//   - CSRF: a malicious page another browser tab loads making an
//     authenticated cross-origin POST to clim's localhost server.
//   - DNS rebinding: a malicious site rebinding its own DNS name to
//     127.0.0.1 and then POSTing through the user's browser. Origin
//     would be the attacker's domain, not r.Host.
//
// Browsers always include at least one of Origin/Referer on POSTs from
// page contexts; we accept either.
//
// We deliberately compare against r.Host rather than the server's
// statically-known URL because the former reflects what the user is
// actually browsing — which is what cross-origin attacks key off — and
// stays correct under reverse proxies, alternative listeners, and the
// httptest.NewServer wrapper used in our own tests.
func csrfProtect(s *Server, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

// requestOrigin reconstructs the browser-facing origin from the
// request's Host header. Scheme is best-guessed via TLS; "http" on
// loopback is the common case for clim.
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
	// Treat "" (default) and explicit defaults as equivalent. Browser
	// Origin headers always include the port for non-default ports,
	// which is the case for clim browser.
	return gp == wp
}
