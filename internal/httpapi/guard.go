package httpapi

import (
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/AlvaroQ/engram-explorer/internal/web"
)

// NewHandler returns the complete production handler: the request guard in
// front of the embedded island bundles and the API/UI router. The router gets
// the same middleware stack as NewServeMux, plus the shared limiter for unsafe
// UI/doctor requests (withUnsafeUIRateLimit), which needs the bare router to
// resolve the routed pattern before dispatch.
func NewHandler(c *Container) http.Handler {
	var handler = withUnsafeUIRateLimit(newRouter(c), newUIRateLimiter(), c.Config.ExposeDetails)
	handler = corsMiddleware(handler, trustedDevOrigins(c.Config.Env))
	handler = requestLogger(c.Logger, handler)
	handler = secureHeaders(handler)
	return Guard(web.Handler(handler), c.Config.Host, trustedDevOrigins(c.Config.Env), c.Logger)
}

// sameOriginOnlyGETs are safe-method routes that must not be triggered by
// another site, not even by a top-level navigation.
var sameOriginOnlyGETs = map[string]bool{
	"/api/db/export": true,
}

// Guard protects every route, including the embedded bundles, against DNS
// rebinding and cross-site request forgery. The dashboard has no
// authentication, so the browser's same-origin policy is the security boundary:
//
//   - The Host header must name the dashboard: "localhost" or a *.localhost
//     name, an IP literal (no DNS name, so it cannot be rebound), or the
//     configured DASHBOARD_HOST. Anything else is refused with 403.
//   - Unsafe methods (POST, PUT, PATCH, DELETE) must be same-origin according
//     to Sec-Fetch-Site or Origin (net/http CrossOriginProtection). Requests
//     without either header (curl, scripts) are not browser CSRF and pass.
//   - GET /api/db/export additionally refuses cross-site and same-site fetches
//     and navigations.
//   - trustedOrigins (the Vite dev server, development only) count as
//     same-origin for both checks.
//   - Security headers are set on every response.
func Guard(next http.Handler, configuredHost string, trustedOrigins map[string]bool, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	cop := http.NewCrossOriginProtection()
	for origin := range trustedOrigins {
		if err := cop.AddTrustedOrigin(origin); err != nil {
			panic("httpapi: invalid trusted origin " + origin + ": " + err.Error())
		}
	}
	deny := func(w http.ResponseWriter, r *http.Request) {
		logger.Warn("blocked cross-origin request", "method", r.Method, "path", r.URL.Path,
			"origin", r.Header.Get("Origin"), "sec_fetch_site", r.Header.Get("Sec-Fetch-Site"))
		writeError(w, http.StatusForbidden, "CROSS_ORIGIN_REQUEST", "cross-origin request blocked", nil, false)
	}
	cop.SetDenyHandler(http.HandlerFunc(deny))
	protected := secureHeaders(cop.Handler(next))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hostAllowed(r.Host, configuredHost) {
			logger.Warn("blocked request with foreign Host", "host", r.Host, "path", r.URL.Path)
			writeError(w, http.StatusForbidden, "HOST_NOT_ALLOWED", "host not allowed", nil, false)
			return
		}
		if sameOriginOnlyGETs[path.Clean("/"+r.URL.Path)] && !sameOriginRequest(r, trustedOrigins) {
			deny(w, r)
			return
		}
		protected.ServeHTTP(w, r)
	})
}

// hostAllowed reports whether the request Host names this dashboard.
func hostAllowed(hostport, configuredHost string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	if host == "" {
		return false
	}
	lower := strings.ToLower(host)
	switch {
	case lower == "localhost", strings.HasSuffix(lower, ".localhost"):
		return true
	case net.ParseIP(host) != nil:
		return true
	case configuredHost != "" && strings.EqualFold(host, configuredHost):
		return true
	}
	return false
}

// sameOriginRequest reports whether a request is same-origin, a user-initiated
// navigation (Sec-Fetch-Site: none), a trusted dev origin, or carries no
// browser metadata at all (non-browser client).
func sameOriginRequest(r *http.Request, trustedOrigins map[string]bool) bool {
	origin := r.Header.Get("Origin")
	if trustedOrigins[origin] {
		return true
	}
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return true
	case "":
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		return err == nil && u.Host == r.Host
	default:
		return false
	}
}
