package httpapi_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
)

// newEngramContainer creates a Container backed by the seeded engram schema DB.
func newEngramContainer(t *testing.T) *httpapi.Container {
	t.Helper()
	path := seedEngramDB(t)
	cfg := config.Config{
		Host:            "127.0.0.1",
		Port:            8787,
		EngramDbPath:    path,
		DaemonBaseURL:   "http://127.0.0.1:7437",
		DaemonTimeoutMs: 100,
		Env:             "development",
		ExposeDetails:   true,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	c, err := httpapi.NewContainer(cfg, logger)
	if err != nil {
		t.Fatalf("NewContainer: %v", err)
	}
	t.Cleanup(c.Close)
	return c
}

func doGet(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	handler.ServeHTTP(rec, req)
	return rec
}

// ---- /api/observations ----

func TestObservationsList_Status200(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/observations")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestObservationsList_ResponseShape(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/observations")

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body["items"]; !ok {
		t.Error("missing 'items'")
	}
	if _, ok := body["nextCursor"]; !ok {
		t.Error("missing 'nextCursor'")
	}
	if _, ok := body["total"]; !ok {
		t.Error("missing 'total'")
	}
}

func TestObservationsList_ContainsSeededObs(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/observations")

	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(body.Items))
	}
	if body.Items[0]["type"] != "note" {
		t.Errorf("type: got %v, want note", body.Items[0]["type"])
	}
}

func TestObservationsTypes_Status200(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/observations/types")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	// Node returns an object {items:[...]} (listTypes(): { items: string[] }), not a bare array.
	var body struct {
		Items []string `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0] != "note" {
		t.Errorf("types: got %v, want {items:[note]}", body.Items)
	}
}

func TestObservationsSearch_MissingQ(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/observations/search")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", rec.Code)
	}
}

func TestObservationsSearch_WithQ(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/observations/search?q=hello")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body["items"]; !ok {
		t.Error("missing 'items'")
	}
}

func TestObservationsGetByID_Found(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/observations/1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body["observation"]; !ok {
		t.Error("missing 'observation'")
	}
	if _, ok := body["revisions"]; !ok {
		t.Error("missing 'revisions'")
	}
}

func TestObservationsGetByID_NotFound(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/observations/9999")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", rec.Code)
	}
}

func TestObservationsGetByID_InvalidID(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/observations/types") // confirm types is NOT caught by /{id}
	if rec.Code != http.StatusOK {
		t.Errorf("/observations/types should return 200, got %d", rec.Code)
	}

	rec2 := doGet(t, handler, "/api/observations/abc")
	if rec2.Code != http.StatusBadRequest {
		t.Errorf("non-integer id should return 400, got %d", rec2.Code)
	}
}

// ---- /api/prompts ----

func TestPromptsList_Status200(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/prompts")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 1 {
		t.Errorf("expected 1 prompt, got %d", len(body.Items))
	}
}

func TestPromptsSearch_MissingQ(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/prompts/search")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", rec.Code)
	}
}

// ---- /api/projects ----

func TestProjectsList_Status200(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/projects")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	found := false
	for _, item := range body.Items {
		if item["project"] == "myproject" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'myproject' in projects list, got %v", body.Items)
	}
}

func TestProjectsGetOverview_Found(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/projects/myproject/overview")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["project"] != "myproject" {
		t.Errorf("project: got %v, want myproject", body["project"])
	}
	for _, key := range []string{"kpis", "activity_30d", "by_type", "by_tool", "top_topics", "recent_observations", "recent_sessions", "recent_prompts"} {
		if _, ok := body[key]; !ok {
			t.Errorf("missing key %q in overview", key)
		}
	}
}

func TestProjectsGetOverview_NotFound(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/projects/doesnotexist/overview")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", rec.Code)
	}
}

func TestTopicsList_Status200(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/topics")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 1 {
		t.Errorf("expected 1 topic, got %d", len(body.Items))
	}
}

// ---- /api/overview ----

func TestOverview_Status200(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/overview")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"kpis", "activity_30d", "by_type", "recent_observations", "sync_summary"} {
		if _, ok := body[key]; !ok {
			t.Errorf("overview missing key %q", key)
		}
	}
}

// ---- /api/orphans ----

func TestOrphansList_Status200(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/orphans")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"observations", "sessions", "prompts", "totals"} {
		if _, ok := body[key]; !ok {
			t.Errorf("orphans missing key %q", key)
		}
	}
}

// ---- /api/sessions ----

func TestSessionsList_Status200(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/sessions")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var body struct {
		Items      []map[string]any `json:"items"`
		NextCursor any              `json:"nextCursor"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 1 {
		t.Errorf("expected 1 session, got %d", len(body.Items))
	}
	if body.Items[0]["id"] != "sess-1" {
		t.Errorf("id: got %v, want sess-1", body.Items[0]["id"])
	}
}

func TestSessionsList_SortRecentActivity(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/sessions?sort=recent_activity")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestSessionsList_Enrich(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/sessions?enrich=true")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) == 0 {
		t.Fatal("expected at least 1 item")
	}
	if _, ok := body.Items[0]["tags"]; !ok {
		t.Error("expected 'tags' field when enrich=true")
	}
}

func TestSessionsGetDetail_Found(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/sessions/sess-1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"session", "stats", "events"} {
		if _, ok := body[key]; !ok {
			t.Errorf("missing key %q", key)
		}
	}
}

func TestSessionsGetDetail_NotFound(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/sessions/does-not-exist")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", rec.Code)
	}
}

func TestSessionsGetEvents_Found(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/sessions/sess-1/events")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var body struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Should contain 1 obs + 1 prompt event.
	if len(body.Events) != 2 {
		t.Errorf("expected 2 events (1 obs + 1 prompt), got %d", len(body.Events))
	}
}

// ---- /api/sync/* ----

func TestSyncProjectsList_Status200(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/sync/projects")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body["projects"]; !ok {
		t.Error("missing 'projects'")
	}
}

func TestSyncProjectDetail_NotFound(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/sync/does-not-exist")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", rec.Code)
	}
}

func TestSyncProjectDetail_Found(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	// "myproject" exists in the seeded DB (has observations/sessions).
	rec := doGet(t, handler, "/api/sync/myproject")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"summary", "pending", "upgrade_state"} {
		if _, ok := body[key]; !ok {
			t.Errorf("missing key %q in sync project detail", key)
		}
	}
}

func TestSyncIssues_Status200(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/sync/issues")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body["issues"]; !ok {
		t.Error("missing 'issues'")
	}
	if _, ok := body["generated_at"]; !ok {
		t.Error("missing 'generated_at'")
	}
}

// ---- /api/graph ----

func TestGraph_Status200(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/graph")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"nodes", "edges", "meta"} {
		if _, ok := body[key]; !ok {
			t.Errorf("missing key %q", key)
		}
	}
}

func TestGraph_WithProject(t *testing.T) {
	handler := httpapi.NewServeMux(newEngramContainer(t))
	rec := doGet(t, handler, "/api/graph?project=myproject")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
}

// ---- /api/daemon/* ----

func TestDaemonHealth_503WhenOffline(t *testing.T) {
	// Use an unreachable port (1 is reserved and always refused).
	path := seedEngramDB(t)
	cfg := config.Config{
		Host:            "127.0.0.1",
		Port:            8787,
		EngramDbPath:    path,
		DaemonBaseURL:   "http://127.0.0.1:1", // always refused
		DaemonTimeoutMs: 200,
		Env:             "development",
		ExposeDetails:   true,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	c, err := httpapi.NewContainer(cfg, logger)
	if err != nil {
		t.Fatalf("NewContainer: %v", err)
	}
	t.Cleanup(c.Close)
	handler := httpapi.NewServeMux(c)

	rec := doGet(t, handler, "/api/daemon/health")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d, want 503 (body: %s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("missing 'error' object in: %v", body)
	}
	if errObj["code"] != "DAEMON_UNAVAILABLE" {
		t.Errorf("code: got %v, want DAEMON_UNAVAILABLE", errObj["code"])
	}
}
