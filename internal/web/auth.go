package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
)

// authCookieName is the cookie key under which the bearer token is
// stored once the user has presented it via the URL.
const authCookieName = "clim_auth"

// GenerateAuthToken returns a fresh 32-byte hex-encoded token. Used
// only when the server is bound to a non-loopback address so anyone
// with network access can't reach the UI without first knowing the
// token printed at startup.
func GenerateAuthToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// authMiddleware enforces token authentication when s.opts.AuthToken
// is non-empty. The token can be presented as:
//
//   - ?token=<token> query param (first-visit flow; we set a cookie
//     and redirect to the same URL without the param so the token
//     doesn't leak via Referer headers or browser history).
//   - clim_auth cookie (subsequent requests once the cookie is set).
//   - Authorization: Bearer <token> header (for scripts hitting /api/*).
//
// Loopback binds skip auth — that's the default and the threat model
// there is the same as the regular TUI.
func authMiddleware(s *Server, next http.Handler) http.Handler {
	if s.opts.AuthToken == "" {
		return next
	}
	want := s.opts.AuthToken
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /healthz remains unauthenticated so liveness probes work.
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		// Query token: set cookie and redirect away from the URL so it
		// doesn't sit in Referer headers or browser history.
		if q := r.URL.Query().Get("token"); q != "" {
			if !constEq(q, want) {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name:     authCookieName,
				Value:    want,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
				Secure:   r.TLS != nil,
			})
			cleaned := *r.URL
			values := cleaned.Query()
			values.Del("token")
			cleaned.RawQuery = values.Encode()
			http.Redirect(w, r, cleaned.RequestURI(), http.StatusSeeOther)
			return
		}
		// Cookie or Authorization header: accept either.
		if c, err := r.Cookie(authCookieName); err == nil && constEq(c.Value, want) {
			next.ServeHTTP(w, r)
			return
		}
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			if constEq(strings.TrimPrefix(h, "Bearer "), want) {
				next.ServeHTTP(w, r)
				return
			}
		}
		// Browser-friendly response: directly returning 401 leaves the
		// user staring at a blank page. Redirect them to the same path
		// once we know they're hitting an HTML route, so they can paste
		// the token in their URL bar; for /api/*, return JSON 401.
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"missing or invalid token"}` + "\n"))
			return
		}
		// HTML: minimal paste-the-token page that POSTs to /healthz
		// would be silly; instead, just instruct the user.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><title>klim — auth required</title>` +
			`<body style="font-family:system-ui;padding:2rem;max-width:42rem;margin:0 auto">` +
			`<h1>Authentication required</h1>` +
			`<p>Append <code>?token=&lt;your-token&gt;</code> to the URL once. ` +
			`The token was printed to your terminal when <code>klim browser</code> started` +
			`with <code>--insecure-bind</code>.</p>` +
			`<p>Once authenticated, the token is stored in a cookie for this session.</p>` +
			`</body>`))
	})
}

// constEq is a constant-time string comparison. Required because token
// validation runs on every request and timing-leaks would let a network
// attacker brute-force the token byte-by-byte.
func constEq(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// authedURL returns the URL the CLI should print/open: includes
// ?token=<token> when an auth token is required, plain URL otherwise.
func authedURL(base, token string) string {
	if token == "" {
		return base
	}
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return u.String()
}
