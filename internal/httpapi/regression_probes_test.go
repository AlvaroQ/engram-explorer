package httpapi_test

// Regression probes from the T10 verification (S9, S12, S14). They pin
// behaviour that already holds, so a later change cannot silently reopen it.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
)

// rawRequest parses a raw HTTP/1.1 request, so the request-target reaches the
// handler exactly as a client sent it (httptest.NewRequest would normalise it).
func rawRequest(t *testing.T, target, remoteAddr string) *http.Request {
	t.Helper()
	const body = "value=dark"
	raw := fmt.Sprintf("POST %s HTTP/1.1\r\nHost: 127.0.0.1:8787\r\n"+
		"Content-Type: application/x-www-form-urlencoded\r\nContent-Length: %d\r\n\r\n%s",
		target, len(body), body)
	req, err := http.ReadRequest(bufio.NewReader(strings.NewReader(raw)))
	if err != nil {
		t.Fatalf("ReadRequest %q: %v", target, err)
	}
	req.RemoteAddr = remoteAddr
	return req
}

// TestUIRateLimit_PathTricksDoNotBypass (S12): once a client spent its
// POST /theme budget, no spelling of the same route reaches the handler: the
// limiter keys on the pattern the router dispatches to, and unclean paths are
// redirected by the router before any handler runs.
func TestUIRateLimit_PathTricksDoNotBypass(t *testing.T) {
	handler := httpapi.NewHandler(newWriteContainer(t))
	const client = "10.2.0.1:1111"
	exhaustTheme(t, handler, client)

	for i, tc := range []struct {
		target string
		// routed: the router dispatches it to the theme handler as is (the
		// control below proves the probe is not vacuous); the others are
		// redirected to the canonical path or match no route.
		routed bool
	}{
		{"/theme", true},
		{"//theme", false},
		{"/api/../theme", false},
		{"/api/%2e%2e/theme", false},
		{"/%74heme", true},
		{"http://127.0.0.1:8787/theme", true},
		{"/theme?x=1", true},
		{"/theme/", false},
		{"/./theme", false},
	} {
		t.Run(tc.target, func(t *testing.T) {
			fresh := httptest.NewRecorder()
			handler.ServeHTTP(fresh, rawRequest(t, tc.target, fmt.Sprintf("10.2.0.%d:1111", 100+i)))
			if got := themeCookieSet(fresh); got != tc.routed {
				t.Errorf("control: POST %s from a fresh client set the cookie=%v (status %d), want %v",
					tc.target, got, fresh.Code, tc.routed)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, rawRequest(t, tc.target, client))
			if themeCookieSet(rec) || rec.Code == http.StatusSeeOther {
				t.Errorf("POST %s after the budget was spent reached the theme handler (status %d)", tc.target, rec.Code)
			}
		})
	}
}

// TestRateLimit_ProductionEnvelope (S14): in production (no ExposeDetails) a
// limited request answers 429 with Retry-After and exactly
// {"error":{"code":"RATE_LIMITED","message":"Rate limit exceeded"}}.
func TestRateLimit_ProductionEnvelope(t *testing.T) {
	c := newWriteContainer(t)
	c.Config.Env = "production"
	c.Config.ExposeDetails = false
	handler := httpapi.NewHandler(c)
	const client = "10.2.1.1:1111"
	exhaustTheme(t, handler, client)

	rec := postTheme(t, handler, client)
	assertRateLimited(t, rec)
	var env map[string]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if len(env) != 1 || len(env["error"]) != 2 {
		t.Errorf("envelope = %s, want only error.code and error.message", rec.Body.String())
	}
	if _, ok := env["error"]["details"]; ok {
		t.Errorf("production 429 carries details: %s", rec.Body.String())
	}
}

// TestGuard_ViteOriginMatrix (S9): the Vite dev origins are trusted only in
// development, for a UI POST, the export and a CORS preflight. Sec-Fetch-Site
// none (a navigation the user started) is never cross-site; same-site and
// cross-site are blocked unless the origin is trusted. Port 5174 is never
// trusted.
func TestGuard_ViteOriginMatrix(t *testing.T) {
	origins := []string{"http://localhost:5173", "http://127.0.0.1:5173", "http://localhost:5174"}
	n := 0
	for _, env := range []string{"development", "production"} {
		t.Run(env, func(t *testing.T) {
			c := newWriteContainer(t)
			c.Config.Env = env
			h := httpapi.NewHandler(c)
			for _, origin := range origins {
				trusted := env == "development" && !strings.HasSuffix(origin, ":5174")
				for _, site := range []string{"none", "same-site", "cross-site"} {
					wantAllowed := trusted || site == "none"
					headers := map[string]string{"Origin": origin, "Sec-Fetch-Site": site}
					n++
					remote := fmt.Sprintf("10.3.%d.%d:1000", n/250, n%250) // one budget per request

					rec := sendWithHeaders(h, http.MethodPost, "/theme", "value=dark", headers, remote)
					if allowed := rec.Code == http.StatusSeeOther && themeCookieSet(rec); allowed != wantAllowed {
						t.Errorf("POST /theme origin=%s site=%s: status %d (%s), allowed=%v, want %v",
							origin, site, rec.Code, guardCode(t, rec), allowed, wantAllowed)
					}

					rec = sendWithHeaders(h, http.MethodGet, "/api/db/export", "", headers, remote)
					if allowed := rec.Code == http.StatusOK; allowed != wantAllowed {
						t.Errorf("GET /api/db/export origin=%s site=%s: status %d (%s), allowed=%v, want %v",
							origin, site, rec.Code, guardCode(t, rec), allowed, wantAllowed)
					}
					if !wantAllowed && guardCode(t, rec) != "CROSS_ORIGIN_REQUEST" {
						t.Errorf("blocked export origin=%s site=%s: code %q, want CROSS_ORIGIN_REQUEST", origin, site, guardCode(t, rec))
					}

					pre := map[string]string{"Origin": origin, "Sec-Fetch-Site": site,
						"Access-Control-Request-Method": "POST"}
					rec = sendWithHeaders(h, http.MethodOptions, "/api/projects/p/rename", "", pre, remote)
					acao := rec.Header().Get("Access-Control-Allow-Origin")
					if rec.Code != http.StatusNoContent || (acao == origin) != trusted || (acao != "" && acao != origin) {
						t.Errorf("OPTIONS preflight origin=%s site=%s: status %d, ACAO %q, want 204 with ACAO only when trusted (%v)",
							origin, site, rec.Code, acao, trusted)
					}
				}
			}
		})
	}
}

func sendWithHeaders(h http.Handler, method, target, form string, headers map[string]string, remoteAddr string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(form))
	if form != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Host = "127.0.0.1:8787"
	req.RemoteAddr = remoteAddr
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
