package web

import (
	"net/http"
	"net/http/cookiejar"
)

// newCookieJar returns a default cookie jar; pulled out so tests can
// share a single creation path. Errors from cookiejar.New are
// extremely rare (only on bad PublicSuffixList configs) so callers
// rarely need to handle them.
func newCookieJar() (http.CookieJar, error) {
	return cookiejar.New(nil)
}
