package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
)

type guardReq struct {
	name    string
	method  string
	path    string
	host    string
	headers map[string]string
}

func serveGuard(t *testing.T, h http.Handler, r guardReq) *httptest.ResponseRecorder {
	t.Helper()
	var body *strings.Reader
	if r.method == http.MethodPost {
		body = strings.NewReader(`{}`)
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(r.method, r.path, body)
	req.Host = r.host
	for k, v := range r.headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func guardCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var b struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
		Code string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &b)
	if b.Error.Code != "" {
		return b.Error.Code
	}
	return b.Code
}

// TestGuard_RejectsForeignHost blocks DNS rebinding: a browser tricked into
// resolving attacker.example to 127.0.0.1 still sends Host: attacker.example.
func TestGuard_RejectsForeignHost(t *testing.T) {
	h := httpapi.NewHandler(newEngramContainer(t))
	for _, host := range []string{"attacker.example", "attacker.example:8787", "localhost.attacker.example:8787", ""} {
		rec := serveGuard(t, h, guardReq{method: http.MethodGet, path: "/api/health", host: host})
		if rec.Code != http.StatusForbidden || guardCode(t, rec) != "HOST_NOT_ALLOWED" {
			t.Errorf("Host %q: got %d %q, want 403 HOST_NOT_ALLOWED; body %s", host, rec.Code, guardCode(t, rec), rec.Body.String())
		}
	}
	// The export endpoint is the highest-value rebinding target.
	rec := serveGuard(t, h, guardReq{method: http.MethodGet, path: "/api/db/export", host: "attacker.example:8787"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("export with foreign Host: got %d, want 403", rec.Code)
	}
}

func TestGuard_AllowsLoopbackAndIPHosts(t *testing.T) {
	h := httpapi.NewHandler(newEngramContainer(t))
	for _, host := range []string{"127.0.0.1:8787", "localhost:8787", "LOCALHOST", "[::1]:8787", "app.localhost:8787", "192.168.1.5:8787"} {
		rec := serveGuard(t, h, guardReq{method: http.MethodGet, path: "/api/health", host: host})
		if rec.Code != http.StatusOK {
			t.Errorf("Host %q: got %d, want 200; body %s", host, rec.Code, rec.Body.String())
		}
	}
}

func TestGuard_AllowsConfiguredHost(t *testing.T) {
	c := newEngramContainer(t)
	c.Config.Host = "dash.lan"
	h := httpapi.NewHandler(c)
	rec := serveGuard(t, h, guardReq{method: http.MethodGet, path: "/api/health", host: "dash.lan:8787"})
	if rec.Code != http.StatusOK {
		t.Errorf("configured host: got %d, want 200; body %s", rec.Code, rec.Body.String())
	}
}

// TestGuard_CrossOriginMutations rejects browser cross-site requests on
// unsafe methods (CSRF) while leaving non-browser clients (no fetch metadata,
// no Origin) and same-origin requests alone.
func TestGuard_CrossOriginMutations(t *testing.T) {
	h := httpapi.NewHandler(newEngramContainer(t))
	const host = "127.0.0.1:8787"
	blocked := []guardReq{
		{name: "sec-fetch cross-site", headers: map[string]string{"Sec-Fetch-Site": "cross-site"}},
		{name: "sec-fetch same-site", headers: map[string]string{"Sec-Fetch-Site": "same-site"}},
		{name: "foreign origin", headers: map[string]string{"Origin": "http://attacker.example"}},
		{name: "other port origin", headers: map[string]string{"Origin": "http://127.0.0.1:9999"}},
	}
	for _, path := range []string{"/api/projects/p/rename", "/settings/theme", "/doctor/sync/p/delete"} {
		for _, b := range blocked {
			rec := serveGuard(t, h, guardReq{method: http.MethodPost, path: path, host: host, headers: b.headers})
			if rec.Code != http.StatusForbidden || guardCode(t, rec) != "CROSS_ORIGIN_REQUEST" {
				t.Errorf("POST %s %s: got %d %q, want 403 CROSS_ORIGIN_REQUEST", path, b.name, rec.Code, guardCode(t, rec))
			}
		}
		allowed := []guardReq{
			{name: "no metadata (cli)"},
			{name: "same-origin", headers: map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "http://" + host}},
			{name: "matching origin", headers: map[string]string{"Origin": "http://" + host}},
			{name: "vite dev origin", headers: map[string]string{"Origin": "http://localhost:5173", "Sec-Fetch-Site": "same-site"}},
		}
		for _, a := range allowed {
			rec := serveGuard(t, h, guardReq{method: http.MethodPost, path: path, host: host, headers: a.headers})
			if code := guardCode(t, rec); code == "CROSS_ORIGIN_REQUEST" || code == "HOST_NOT_ALLOWED" {
				t.Errorf("POST %s %s: blocked by guard (%d %q)", path, a.name, rec.Code, code)
			}
		}
	}
}

// TestGuard_ExportRequiresSameOrigin: GET /api/db/export downloads the whole
// database, so cross-site navigations and subresource loads are refused too.
func TestGuard_ExportRequiresSameOrigin(t *testing.T) {
	h := httpapi.NewHandler(newEngramContainer(t))
	const host = "127.0.0.1:8787"
	for _, hdr := range []map[string]string{
		{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "navigate"},
		{"Sec-Fetch-Site": "same-site"},
		{"Origin": "http://attacker.example"},
	} {
		rec := serveGuard(t, h, guardReq{method: http.MethodGet, path: "/api/db/export", host: host, headers: hdr})
		if rec.Code != http.StatusForbidden || guardCode(t, rec) != "CROSS_ORIGIN_REQUEST" {
			t.Errorf("export %v: got %d %q, want 403 CROSS_ORIGIN_REQUEST", hdr, rec.Code, guardCode(t, rec))
		}
	}
	for _, hdr := range []map[string]string{nil, {"Sec-Fetch-Site": "same-origin"}, {"Sec-Fetch-Site": "none"}} {
		rec := serveGuard(t, h, guardReq{method: http.MethodGet, path: "/api/db/export", host: host, headers: hdr})
		if code := guardCode(t, rec); code == "CROSS_ORIGIN_REQUEST" {
			t.Errorf("export %v: blocked by guard", hdr)
		}
	}
	// Ordinary cross-site navigation to a UI page stays allowed (links into the dashboard).
	rec := serveGuard(t, h, guardReq{method: http.MethodGet, path: "/api/health", host: host,
		headers: map[string]string{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "navigate"}})
	if rec.Code != http.StatusOK {
		t.Errorf("cross-site navigation to /api/health: got %d, want 200", rec.Code)
	}
}

// TestGuard_SecurityHeadersOnBundles: embedded /islands/ and /assets/ files
// used to bypass the security-header middleware.
func TestGuard_SecurityHeadersOnBundles(t *testing.T) {
	h := httpapi.NewHandler(newEngramContainer(t))
	for _, path := range []string{"/islands/loader.js", "/assets/app.js", "/api/health"} {
		rec := serveGuard(t, h, guardReq{method: http.MethodGet, path: path, host: "127.0.0.1:8787"})
		for _, k := range []string{"Content-Security-Policy", "X-Content-Type-Options", "X-Frame-Options"} {
			if rec.Header().Get(k) == "" {
				t.Errorf("%s: missing %s header", path, k)
			}
		}
	}
}
