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

// TestGuard_EdgeCases pins bypass attempts probed during the phase
// verification: trailing-dot and non-canonical hosts, and export path variants
// and HEAD requests from another site.
func TestGuard_EdgeCases(t *testing.T) {
	h := httpapi.NewHandler(newEngramContainer(t))
	for _, host := range []string{"localhost.", "localhost.:8787", "0x7f.1", "127.0.0.1.nip.io:8787"} {
		rec := serveGuard(t, h, guardReq{method: http.MethodGet, path: "/api/health", host: host})
		if rec.Code != http.StatusForbidden || guardCode(t, rec) != "HOST_NOT_ALLOWED" {
			t.Errorf("Host %q: got %d %q, want 403 HOST_NOT_ALLOWED", host, rec.Code, guardCode(t, rec))
		}
	}
	cross := map[string]string{"Sec-Fetch-Site": "cross-site"}
	for _, r := range []guardReq{
		{method: http.MethodGet, path: "//api/db/export"},
		{method: http.MethodGet, path: "/api/db/export/"},
		{method: http.MethodGet, path: "/api/db/./export"},
		{method: http.MethodGet, path: "/api/db/%65xport"},
		{method: http.MethodHead, path: "/api/db/export"},
	} {
		r.host, r.headers = "127.0.0.1:8787", cross
		rec := serveGuard(t, h, r)
		if rec.Code != http.StatusForbidden || guardCode(t, rec) != "CROSS_ORIGIN_REQUEST" {
			t.Errorf("%s %s cross-site: got %d %q, want 403 CROSS_ORIGIN_REQUEST", r.method, r.path, rec.Code, guardCode(t, rec))
		}
	}
}

// TestGuard_ViteOriginsOnlyInDevelopment: the Vite dev server origins
// (localhost/127.0.0.1:5173) are trusted for CORS, CSRF and the export route
// only when ENGRAM_DASH_ENV=development; in production they are foreign.
func TestGuard_ViteOriginsOnlyInDevelopment(t *testing.T) {
	const host = "127.0.0.1:8787"
	for _, env := range []string{"development", "production"} {
		t.Run(env, func(t *testing.T) {
			c := newEngramContainer(t)
			c.Config.Env = env
			h := httpapi.NewHandler(c)
			dev := env == "development"
			for _, origin := range []string{"http://localhost:5173", "http://127.0.0.1:5173"} {
				vite := map[string]string{"Origin": origin, "Sec-Fetch-Site": "same-site"}

				rec := serveGuard(t, h, guardReq{method: http.MethodPost, path: "/api/projects/p/rename", host: host, headers: vite})
				blocked := guardCode(t, rec) == "CROSS_ORIGIN_REQUEST"
				if blocked == dev {
					t.Errorf("POST from %s: blocked=%v (%d), want blocked=%v", origin, blocked, rec.Code, !dev)
				}

				rec = serveGuard(t, h, guardReq{method: http.MethodGet, path: "/api/db/export", host: host, headers: vite})
				blocked = guardCode(t, rec) == "CROSS_ORIGIN_REQUEST"
				if blocked == dev {
					t.Errorf("export from %s: blocked=%v (%d), want blocked=%v", origin, blocked, rec.Code, !dev)
				}

				rec = serveGuard(t, h, guardReq{method: http.MethodGet, path: "/api/health", host: host, headers: map[string]string{"Origin": origin}})
				acao := rec.Header().Get("Access-Control-Allow-Origin")
				if dev && acao != origin {
					t.Errorf("CORS for %s in development: got %q, want %q", origin, acao, origin)
				}
				if !dev && acao != "" {
					t.Errorf("CORS for %s in production: got %q, want none", origin, acao)
				}
			}
		})
	}
}

// TestGuard_ProjectExportRequiresSameOrigin: GET /api/projects/{project}/export
// downloads a project's data, so Guard itself refuses cross-site and same-site
// fetches, like /api/db/export; other project routes keep the default rule.
func TestGuard_ProjectExportRequiresSameOrigin(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := httpapi.Guard(ok, "", nil, nil)
	const host = "127.0.0.1:8787"
	crossSite := map[string]string{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "navigate"}
	for _, p := range []string{"/api/projects/foo/export", "/api/projects/a%2Fb/export", "//api/projects/foo/export", "/api/projects/foo/%65xport"} {
		rec := serveGuard(t, h, guardReq{method: http.MethodGet, path: p, host: host, headers: crossSite})
		if rec.Code != http.StatusForbidden {
			t.Errorf("GET %s cross-site: got %d, want 403", p, rec.Code)
		}
	}
	for _, p := range []string{"/api/projects/foo", "/api/projects/foo/export/extra"} {
		rec := serveGuard(t, h, guardReq{method: http.MethodGet, path: p, host: host, headers: crossSite})
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s cross-site: got %d, want 200 (not an export)", p, rec.Code)
		}
	}
	rec := serveGuard(t, h, guardReq{method: http.MethodGet, path: "/api/projects/foo/export", host: host,
		headers: map[string]string{"Sec-Fetch-Site": "same-origin"}})
	if rec.Code != http.StatusOK {
		t.Errorf("same-origin project export: got %d, want 200", rec.Code)
	}
}
