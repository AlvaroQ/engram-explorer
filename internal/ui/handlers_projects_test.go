package ui_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/services"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// projectsSchema is the minimal DDL for the projects page tests.
const projectsSchema = `
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY, project TEXT, directory TEXT,
    started_at TEXT, ended_at TEXT, summary TEXT
);
CREATE TABLE IF NOT EXISTS user_prompts (
    id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT,
    project TEXT, content TEXT, created_at TEXT
);
CREATE TABLE IF NOT EXISTS sync_state (
    target_key TEXT PRIMARY KEY, lifecycle TEXT,
    last_enqueued_seq INTEGER, last_acked_seq INTEGER,
    last_pulled_seq INTEGER, consecutive_failures INTEGER,
    backoff_until TEXT, lease_owner TEXT, lease_until TEXT,
    last_error TEXT, reason_code TEXT, reason_message TEXT,
    updated_at TEXT
);
CREATE TABLE IF NOT EXISTS sync_mutations (
    seq INTEGER PRIMARY KEY AUTOINCREMENT, target_key TEXT,
    entity TEXT, entity_key TEXT, op TEXT, source TEXT,
    occurred_at TEXT, acked_at TEXT, project TEXT
);
CREATE TABLE IF NOT EXISTS sync_enrolled_projects (
    project TEXT PRIMARY KEY, enrolled_at TEXT
);
`

// fakeCloud is a minimal projectsCloud implementation for tests.
type fakeCloud struct {
	enrollCalls   []string
	unenrollCalls []string
	caps          services.CloudCapabilities
}

func (f *fakeCloud) Enroll(_ context.Context, project string) (services.CloudResult, error) {
	f.enrollCalls = append(f.enrollCalls, project)
	return services.CloudResult{OK: true, Project: project}, nil
}

func (f *fakeCloud) Unenroll(_ context.Context, project string) (services.CloudResult, error) {
	f.unenrollCalls = append(f.unenrollCalls, project)
	return services.CloudResult{OK: true, Project: project}, nil
}

func (f *fakeCloud) Capabilities(_ context.Context) (services.CloudCapabilities, error) {
	return f.caps, nil
}

// openProjectsDB opens a test DB with the full projects+sync schema.
func openProjectsDB(t *testing.T) *sql.DB {
	t.Helper()
	db := openTestDB(t) // opens with base observations schema
	if _, err := db.ExecContext(context.Background(), projectsSchema); err != nil {
		t.Fatalf("create projects schema: %v", err)
	}
	return db
}

// TestProjectsFullPage verifies GET /projects returns a full HTML page.
func TestProjectsFullPage(t *testing.T) {
	db := openProjectsDB(t)
	_, err := db.Exec(
		`INSERT INTO observations (type, title, project, created_at, revision_count) VALUES ('manual','test','myproject','2025-01-01',1)`,
	)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	mux := http.NewServeMux()
	cloud := &fakeCloud{caps: services.CloudCapabilities{Enroll: true, Unenroll: true}}
	ui.MountWithCloud(mux, ui.Deps{RoDB: db}, cloud)

	req := httptest.NewRequest(http.MethodGet, "/projects", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "<html") {
		t.Error("full page must contain <html>")
	}
	if !strings.Contains(body, "myproject") {
		t.Errorf("page must contain the seeded project; got:\n%s", body)
	}
}

// TestProjectsHTMXPartial verifies HX-Request returns a fragment without <html>.
func TestProjectsHTMXPartial(t *testing.T) {
	db := openProjectsDB(t)

	mux := http.NewServeMux()
	cloud := &fakeCloud{caps: services.CloudCapabilities{Enroll: true, Unenroll: true}}
	ui.MountWithCloud(mux, ui.Deps{RoDB: db}, cloud)

	req := httptest.NewRequest(http.MethodGet, "/projects", nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "<html") {
		t.Error("HTMX partial must NOT contain <html>")
	}
}

// TestProjectsListPartialEndpoint verifies GET /projects/list returns a fragment.
func TestProjectsListPartialEndpoint(t *testing.T) {
	db := openProjectsDB(t)

	mux := http.NewServeMux()
	cloud := &fakeCloud{caps: services.CloudCapabilities{Enroll: true, Unenroll: true}}
	ui.MountWithCloud(mux, ui.Deps{RoDB: db}, cloud)

	req := httptest.NewRequest(http.MethodGet, "/projects/list", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "<html") {
		t.Error("list partial must NOT contain <html>")
	}
}

// TestProjectsRoDBNil verifies nil RoDB does not panic and returns 200.
func TestProjectsRoDBNil(t *testing.T) {
	mux := http.NewServeMux()
	cloud := &fakeCloud{caps: services.CloudCapabilities{Enroll: true, Unenroll: true}}
	ui.MountWithCloud(mux, ui.Deps{RoDB: nil}, cloud)

	req := httptest.NewRequest(http.MethodGet, "/projects", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handler panicked: %v", r)
		}
	}()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with empty state, got %d; body: %s", w.Code, w.Body.String())
	}
}

// TestProjectsTextFilter verifies ?q= filters results server-side.
func TestProjectsTextFilter(t *testing.T) {
	db := openProjectsDB(t)
	for _, project := range []string{"alpha-project", "beta-project"} {
		_, err := db.Exec(
			`INSERT INTO observations (type, project, created_at, revision_count) VALUES ('manual',?,'2025-01-01',1)`,
			project,
		)
		if err != nil {
			t.Fatalf("seed %s: %v", project, err)
		}
	}

	mux := http.NewServeMux()
	cloud := &fakeCloud{caps: services.CloudCapabilities{Enroll: true, Unenroll: true}}
	ui.MountWithCloud(mux, ui.Deps{RoDB: db}, cloud)

	req := httptest.NewRequest(http.MethodGet, "/projects?q=alpha", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "alpha-project") {
		t.Error("filtered results must include alpha-project")
	}
	if strings.Contains(body, "beta-project") {
		t.Error("filtered results must not include beta-project")
	}
}
