package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
)

// jsonBodyLimit mirrors the 1 MiB cap the write handlers apply to JSON bodies.
const jsonBodyLimit = 1 << 20

// oversizedJSON returns a syntactically valid JSON object whose single string
// field pushes the body past the 1 MiB limit.
func oversizedJSON(field string) string {
	return `{"` + field + `":"` + strings.Repeat("a", jsonBodyLimit+16) + `"}`
}

func doRawJSON(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	return rec
}

// assertErrorCode checks the status code and the JSON error envelope code/message.
func assertErrorCode(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode, wantMessage string) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, wantStatus, rec.Body.String())
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
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if env.Error.Code != wantCode {
		t.Errorf("error.code: got %q, want %q", env.Error.Code, wantCode)
	}
	if wantMessage != "" && env.Error.Message != wantMessage {
		t.Errorf("error.message: got %q, want %q", env.Error.Message, wantMessage)
	}
}

func queryString(t *testing.T, c *httpapi.Container, query string, args ...any) string {
	t.Helper()
	var s string
	if err := c.RWDB.QueryRow(query, args...).Scan(&s); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return s
}

func TestBodyLimit_ObservationPatch_Oversized413(t *testing.T) {
	c := newWriteContainer(t)
	handler := httpapi.NewServeMux(c)

	rec := doRawJSON(t, handler, http.MethodPatch, "/api/observations/1", oversizedJSON("title"))
	assertErrorCode(t, rec, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "")

	if got := queryString(t, c, `SELECT title FROM observations WHERE id = 1`); got != "Test Obs" {
		t.Errorf("title changed after rejected body: got %q, want %q", got, "Test Obs")
	}
}

func TestBodyLimit_AssignProject_Oversized413(t *testing.T) {
	c := newWriteContainer(t)
	handler := httpapi.NewServeMux(c)

	rec := doRawJSON(t, handler, http.MethodPatch, "/api/observations/1/project", oversizedJSON("project"))
	assertErrorCode(t, rec, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "")

	if got := queryString(t, c, `SELECT project FROM observations WHERE id = 1`); got != "myproject" {
		t.Errorf("project changed after rejected body: got %q, want %q", got, "myproject")
	}
}

func TestBodyLimit_ProjectRename_Oversized413(t *testing.T) {
	c := newWriteContainer(t)
	handler := httpapi.NewServeMux(c)

	rec := doRawJSON(t, handler, http.MethodPost, "/api/projects/myproject/rename", oversizedJSON("target"))
	assertErrorCode(t, rec, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "")

	if got := queryString(t, c, `SELECT project FROM sessions WHERE id = 'sess-1'`); got != "myproject" {
		t.Errorf("session project changed after rejected body: got %q, want %q", got, "myproject")
	}
	if got := queryString(t, c, `SELECT project FROM observations WHERE id = 1`); got != "myproject" {
		t.Errorf("observation project changed after rejected body: got %q, want %q", got, "myproject")
	}
}

// A body just under the limit is still accepted.
func TestBodyLimit_ObservationPatch_UnderLimitAccepted(t *testing.T) {
	c := newWriteContainer(t)
	handler := httpapi.NewServeMux(c)

	title := strings.Repeat("b", jsonBodyLimit-64)
	rec := doRawJSON(t, handler, http.MethodPatch, "/api/observations/1", `{"title":"`+title+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %.200s)", rec.Code, rec.Body.String())
	}
	if got := queryString(t, c, `SELECT title FROM observations WHERE id = 1`); got != title {
		t.Errorf("title not stored: got len %d, want len %d", len(got), len(title))
	}
}

// Malformed JSON keeps the previous 400 BAD_INPUT "invalid JSON body" response.
func TestBodyLimit_MalformedJSON_Still400(t *testing.T) {
	cases := []struct {
		method, path string
	}{
		{http.MethodPatch, "/api/observations/1"},
		{http.MethodPatch, "/api/observations/1/project"},
		{http.MethodPost, "/api/projects/myproject/rename"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			c := newWriteContainer(t)
			handler := httpapi.NewServeMux(c)
			rec := doRawJSON(t, handler, tc.method, tc.path, `{"title":`)
			assertErrorCode(t, rec, http.StatusBadRequest, "BAD_INPUT", "invalid JSON body")
			if got := queryString(t, c, `SELECT title FROM observations WHERE id = 1`); got != "Test Obs" {
				t.Errorf("title changed: got %q", got)
			}
		})
	}
}

// Data after the JSON value is rejected (400, nothing stored); trailing
// whitespace is still accepted.
func TestBodyLimit_TrailingDataRejected(t *testing.T) {
	for _, body := range []string{
		`{"title":"x"}{"title":"y"}`,
		`{"title":"x"} garbage`,
		`{"title":"x"}]`,
	} {
		t.Run(body, func(t *testing.T) {
			c := newWriteContainer(t)
			handler := httpapi.NewServeMux(c)
			rec := doRawJSON(t, handler, http.MethodPatch, "/api/observations/1", body)
			assertErrorCode(t, rec, http.StatusBadRequest, "BAD_INPUT", "invalid JSON body")
			if got := queryString(t, c, `SELECT title FROM observations WHERE id = 1`); got != "Test Obs" {
				t.Errorf("title changed after rejected body: got %q", got)
			}
		})
	}

	c := newWriteContainer(t)
	handler := httpapi.NewServeMux(c)
	rec := doRawJSON(t, handler, http.MethodPatch, "/api/observations/1", "{\"title\":\"ok\"} \n\t")
	if rec.Code != http.StatusOK {
		t.Fatalf("trailing whitespace: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if got := queryString(t, c, `SELECT title FROM observations WHERE id = 1`); got != "ok" {
		t.Errorf("title: got %q, want ok", got)
	}
}
