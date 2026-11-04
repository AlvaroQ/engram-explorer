package httpapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
)

func doFrom(t *testing.T, handler http.Handler, method, path, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = remoteAddr
	handler.ServeHTTP(rec, req)
	return rec
}

// assertRateLimited checks the 429 status, the Retry-After header (positive
// delay-seconds within the 60 s window) and the JSON error envelope.
func assertRateLimited(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status: got %d, want 429 (body: %s)", rec.Code, rec.Body.String())
	}
	ra := rec.Header().Get("Retry-After")
	secs, err := strconv.Atoi(ra)
	if err != nil || secs < 1 || secs > 60 {
		t.Errorf("Retry-After: got %q, want delay-seconds in [1,60]", ra)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q, want application/json", ct)
	}
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error envelope: %v (body: %s)", err, rec.Body.String())
	}
	if env.Error.Code != "RATE_LIMITED" {
		t.Errorf("error.code: got %q, want RATE_LIMITED", env.Error.Code)
	}
	if env.Error.Message != "Rate limit exceeded" {
		t.Errorf("error.message: got %q, want %q", env.Error.Message, "Rate limit exceeded")
	}
}

func emptyImport(t *testing.T, handler http.Handler, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "notadb.db")
	fw.Write([]byte("not a sqlite file"))
	mw.Close()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/db/import", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.RemoteAddr = remoteAddr
	handler.ServeHTTP(rec, req)
	return rec
}

// POST /api/db/import allows 5 requests per minute per client.
func TestRateLimit_Import_FivePerMinutePerClient(t *testing.T) {
	handler := httpapi.NewServeMux(newWriteContainer(t))
	const clientA = "10.0.0.1:1111"
	for i := 0; i < 5; i++ {
		if rec := emptyImport(t, handler, clientA); rec.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d limited too early", i+1)
		}
	}
	assertRateLimited(t, emptyImport(t, handler, clientA))

	// Same IP, different source port: still the same client.
	assertRateLimited(t, emptyImport(t, handler, "10.0.0.1:2222"))

	// A different client keeps its own budget.
	if rec := emptyImport(t, handler, "10.0.0.2:1111"); rec.Code != http.StatusBadRequest {
		t.Errorf("other client: got %d, want 400 (handler reached)", rec.Code)
	}
}

// X-Forwarded-For is not trusted to pick the client.
func TestRateLimit_IgnoresXForwardedFor(t *testing.T) {
	handler := httpapi.NewServeMux(newWriteContainer(t))
	for i := 0; i < 6; i++ {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		mw.Close()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/db/import", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("203.0.113.%d", i))
		req.RemoteAddr = "10.0.0.9:1234"
		handler.ServeHTTP(rec, req)
		if i < 5 && rec.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d limited too early", i+1)
		}
		if i == 5 {
			assertRateLimited(t, rec)
		}
	}
}

// GET /api/db/export allows 10 requests per minute per client.
func TestRateLimit_Export_TenPerMinute(t *testing.T) {
	handler := httpapi.NewServeMux(newWriteContainer(t))
	const client = "10.0.1.1:1111"
	for i := 0; i < 10; i++ {
		if rec := doFrom(t, handler, http.MethodGet, "/api/db/export", client); rec.Code != http.StatusOK {
			t.Fatalf("export %d: got %d, want 200", i+1, rec.Code)
		}
	}
	assertRateLimited(t, doFrom(t, handler, http.MethodGet, "/api/db/export", client))
}

// Other write routes allow 120 requests per minute per client and route
// pattern: different ids share one budget, different patterns do not.
func TestRateLimit_WriteRoute_SharedPerPattern(t *testing.T) {
	handler := httpapi.NewServeMux(newWriteContainer(t))
	const client = "10.0.2.1:1111"
	for i := 0; i < 120; i++ {
		path := fmt.Sprintf("/api/observations/%d", 10000+i)
		if rec := doFrom(t, handler, http.MethodDelete, path, client); rec.Code != http.StatusNotFound {
			t.Fatalf("delete %d: got %d, want 404", i+1, rec.Code)
		}
	}
	// 121st request on yet another id of the same pattern is limited.
	assertRateLimited(t, doFrom(t, handler, http.MethodDelete, "/api/observations/99999", client))

	// A different client keeps its own budget on the same pattern.
	if rec := doFrom(t, handler, http.MethodDelete, "/api/observations/99999", "10.0.2.2:1111"); rec.Code != http.StatusNotFound {
		t.Errorf("other client: got %d, want 404", rec.Code)
	}
	// The same client still has budget on a different route pattern.
	if rec := doFrom(t, handler, http.MethodDelete, "/api/prompts/99999", client); rec.Code != http.StatusNotFound {
		t.Errorf("other pattern: got %d, want 404", rec.Code)
	}
}
