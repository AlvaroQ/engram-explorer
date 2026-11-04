package httpapi_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
)

// uiWriteLimit is the per-client, per-method, per-route budget of the shared
// limiter for unsafe UI/doctor requests (same as the API write routes).
const uiWriteLimit = 120

// sendFrom sends a request through the production handler with a loopback Host
// (so the Guard lets it through) from the given client address.
func sendFrom(t *testing.T, handler http.Handler, method, target string, form url.Values, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	req.Host = "127.0.0.1:8787"
	req.RemoteAddr = remoteAddr
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func themeCookieSet(rec *httptest.ResponseRecorder) bool {
	for _, c := range rec.Result().Cookies() {
		if c.Name == "theme" {
			return true
		}
	}
	return false
}

// postTheme sends POST /theme value=dark and returns the response.
func postTheme(t *testing.T, handler http.Handler, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()
	return sendFrom(t, handler, http.MethodPost, "/theme", url.Values{"value": {"dark"}}, remoteAddr)
}

// exhaustTheme spends the whole POST /theme budget of one client, checking
// that every request inside the budget reaches the handler.
func exhaustTheme(t *testing.T, handler http.Handler, remoteAddr string) {
	t.Helper()
	for i := 0; i < uiWriteLimit; i++ {
		rec := postTheme(t, handler, remoteAddr)
		if rec.Code != http.StatusSeeOther || !themeCookieSet(rec) {
			t.Fatalf("POST /theme %d: got %d (cookie set: %v), want 303 with theme cookie; body %s",
				i+1, rec.Code, themeCookieSet(rec), rec.Body.String())
		}
	}
}

// UI form POSTs are limited per client: the 121st POST /theme answers 429
// RATE_LIMITED without reaching the handler, another client keeps its own
// budget, and another UI route keeps its own budget for the same client.
func TestUIRateLimit_ThemePost_PerClient(t *testing.T) {
	handler := httpapi.NewHandler(newWriteContainer(t))
	const clientA = "10.1.0.1:1111"
	exhaustTheme(t, handler, clientA)

	rec := postTheme(t, handler, clientA)
	assertRateLimited(t, rec)
	if themeCookieSet(rec) {
		t.Error("rate-limited POST /theme set the theme cookie: the handler ran")
	}

	// Same IP, different source port: still the same client.
	assertRateLimited(t, postTheme(t, handler, "10.1.0.1:2222"))

	// HTMX requests get the same 429 JSON envelope.
	req := httptest.NewRequest(http.MethodPost, "/theme", strings.NewReader("value=dark"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.Host = "127.0.0.1:8787"
	req.RemoteAddr = clientA
	hx := httptest.NewRecorder()
	handler.ServeHTTP(hx, req)
	assertRateLimited(t, hx)
	if hx.Header().Get("HX-Redirect") != "" {
		t.Error("rate-limited HTMX POST /theme carries HX-Redirect: the handler ran")
	}

	// A different client keeps its own budget.
	if rec := postTheme(t, handler, "10.1.0.2:1111"); rec.Code != http.StatusSeeOther || !themeCookieSet(rec) {
		t.Errorf("other client: got %d (cookie set: %v), want 303 with theme cookie", rec.Code, themeCookieSet(rec))
	}

	// The same client still has budget on another UI route pattern.
	if rec := sendFrom(t, handler, http.MethodPost, "/lang", url.Values{"value": {"es"}}, clientA); rec.Code != http.StatusSeeOther {
		t.Errorf("POST /lang after /theme exhausted: got %d, want 303", rec.Code)
	}
}

// Doctor writes are limited per routed pattern: different path parameters of
// POST /doctor/sync/{project}/delete share one budget, and the rejected
// request does not reach the handler (the seeded project survives, while the
// same request from another client deletes it).
func TestUIRateLimit_DoctorSyncDelete_SharedAcrossPathParams(t *testing.T) {
	handler := httpapi.NewHandler(newWriteContainer(t))
	const client = "10.1.1.1:1111"

	// Make the seeded project deletable: remove its only live observation.
	if rec := sendFrom(t, handler, http.MethodDelete, "/api/observations/1", nil, "10.1.1.9:1111"); rec.Code != http.StatusOK {
		t.Fatalf("setup DELETE /api/observations/1: got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	for i := 0; i < uiWriteLimit; i++ {
		target := fmt.Sprintf("/doctor/sync/ghost-%d/delete", i)
		if rec := sendFrom(t, handler, http.MethodPost, target, nil, client); rec.Code == http.StatusTooManyRequests {
			t.Fatalf("POST %s (request %d) limited too early", target, i+1)
		}
	}

	assertRateLimited(t, sendFrom(t, handler, http.MethodPost, "/doctor/sync/myproject/delete", nil, client))
	if rec := sendFrom(t, handler, http.MethodGet, "/api/projects/myproject/overview", nil, "10.1.1.9:1111"); rec.Code != http.StatusOK {
		t.Errorf("seeded project after rate-limited delete: got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}

	// A different client keeps its own budget on the same pattern, and its
	// request does reach the handler.
	if rec := sendFrom(t, handler, http.MethodPost, "/doctor/sync/myproject/delete", nil, "10.1.1.2:1111"); rec.Code != http.StatusOK {
		t.Errorf("other client: got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if rec := sendFrom(t, handler, http.MethodGet, "/api/projects/myproject/overview", nil, "10.1.1.9:1111"); rec.Code != http.StatusNotFound {
		t.Errorf("seeded project after the other client's delete: got %d, want 404", rec.Code)
	}

	// DELETE on another doctor pattern has its own budget for the same client.
	if rec := sendFrom(t, handler, http.MethodDelete, "/doctor/orphans/observations/99999", nil, client); rec.Code == http.StatusTooManyRequests {
		t.Error("DELETE /doctor/orphans/... limited by the POST /doctor/sync budget")
	}
}

// Doctor DELETE routes are limited too.
func TestUIRateLimit_DoctorOrphansDelete_Limited(t *testing.T) {
	handler := httpapi.NewHandler(newWriteContainer(t))
	const client = "10.1.2.1:1111"
	for i := 0; i < uiWriteLimit; i++ {
		target := fmt.Sprintf("/doctor/orphans/observations/%d", 50000+i)
		if rec := sendFrom(t, handler, http.MethodDelete, target, nil, client); rec.Code == http.StatusTooManyRequests {
			t.Fatalf("DELETE %s (request %d) limited too early", target, i+1)
		}
	}
	assertRateLimited(t, sendFrom(t, handler, http.MethodDelete, "/doctor/orphans/observations/1", nil, client))
}

// GET requests are never limited by the UI limiter, even after the client
// exhausted its POST budget.
func TestUIRateLimit_GETNeverLimited(t *testing.T) {
	handler := httpapi.NewHandler(newWriteContainer(t))
	const client = "10.1.3.1:1111"
	exhaustTheme(t, handler, client)
	for i := 0; i < uiWriteLimit+5; i++ {
		if rec := sendFrom(t, handler, http.MethodGet, "/settings", nil, client); rec.Code != http.StatusOK {
			t.Fatalf("GET /settings %d: got %d, want 200", i+1, rec.Code)
		}
	}
}

// /api/ write routes keep only their own per-route limiter: an exhausted UI
// budget does not touch them, and their 121st request is the first 429.
func TestUIRateLimit_APIWriteRouteUsesOnlyItsOwnLimiter(t *testing.T) {
	handler := httpapi.NewHandler(newWriteContainer(t))
	const client = "10.1.4.1:1111"
	exhaustTheme(t, handler, client)
	for i := 0; i < 120; i++ {
		target := fmt.Sprintf("/api/observations/%d", 10000+i)
		if rec := sendFrom(t, handler, http.MethodDelete, target, nil, client); rec.Code != http.StatusNotFound {
			t.Fatalf("DELETE %s (request %d): got %d, want 404", target, i+1, rec.Code)
		}
	}
	assertRateLimited(t, sendFrom(t, handler, http.MethodDelete, "/api/observations/99999", nil, client))
}
