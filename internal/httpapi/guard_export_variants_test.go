package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Regression: Guard refuses cross-site and same-site GET/HEAD on the project
// export for every encoded, slash-doubled or dot-segment spelling of the path,
// while unrelated project routes and same-origin requests still pass.
func TestGuard_ProjectExportPathVariants(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	g := Guard(ok, "", nil, nil)
	serve := func(method, path, site string) int {
		req := httptest.NewRequest(method, "http://127.0.0.1:1"+path, nil)
		if site != "" {
			req.Header.Set("Sec-Fetch-Site", site)
		}
		rec := httptest.NewRecorder()
		g.ServeHTTP(rec, req)
		return rec.Code
	}

	exports := []string{
		"/api/projects/x/export",
		"/api/projects/a%2Fb/export",
		"/api/projects/x/%65xport",
		"/api/projects/x/export/",
		"//api/projects/x/export",
		"/api/projects/x/./export",
		"/api/projects/%2E%2E/export",
		"/api/projects/x%2F..%2F/export",
	}
	others := []string{"/api/projects/x", "/api/projects/x/export/more"}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		for _, site := range []string{"cross-site", "same-site"} {
			for _, p := range exports {
				if got := serve(method, p, site); got != http.StatusForbidden {
					t.Errorf("%s %s Sec-Fetch-Site=%s: got %d, want 403", method, p, site, got)
				}
			}
			for _, p := range others {
				if got := serve(method, p, site); got != http.StatusOK {
					t.Errorf("%s %s Sec-Fetch-Site=%s: got %d, want 200", method, p, site, got)
				}
			}
		}
		if got := serve(method, "/api/projects/x/export", "same-origin"); got != http.StatusOK {
			t.Errorf("%s same-origin export: got %d, want 200", method, got)
		}
	}
}

// Regression: the backup file name part never carries header-breaking or
// path characters.
func TestBackupNamePart_Sanitizes(t *testing.T) {
	for in, want := range map[string]string{
		"a\r\nSet-Cookie: x=1": "a-Set-Cookie-x-1",
		"../../etc/passwd":     "etc-passwd",
	} {
		if got := backupNamePart(in); got != want {
			t.Errorf("backupNamePart(%q) = %q, want %q", in, got, want)
		}
	}
}
