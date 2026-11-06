package doctor_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/doctor"
)

// TestDoctorUsesSharedUIHelpers fails if the doctor package defines its own
// copy of the HTML shell helpers the ui package already provides (one render
// path for both modules): HTMX detection, rendering, error partials and the
// read-only write guard.
func TestDoctorUsesSharedUIHelpers(t *testing.T) {
	duplicates := map[string]bool{
		"IsHTMX": true, "render": true, "renderError": true, "requireRW": true, "ErrorPartial": true,
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && duplicates[fn.Name.Name] {
				t.Errorf("%s defines %s; use the ui package version instead", f, fn.Name.Name)
			}
		}
	}
}

// TestDoctorRendering_ShellBehaviourPreserved pins the doctor rendering
// behaviour shared with the ui package: HTMX requests get the bare partial,
// direct navigation gets the full ui.Layout page, both as text/html, and the
// write guard answers with the read-only error partial without
// touching the database.
func TestDoctorRendering_ShellBehaviourPreserved(t *testing.T) {
	db := openSyncTestDB(t)
	ensureSyncStateTables(t, db)
	const proj = "shell-proj"
	seedObservationForProject(t, db, proj)
	seedEnrolledProject(t, db, proj)
	seedSyncState(t, db, "cloud:"+proj, "active", nil)

	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db, RWDB: db})
	for _, htmx := range []bool{true, false} {
		req := httptest.NewRequest(http.MethodGet, "/doctor/sync/"+proj, nil)
		if htmx {
			req.Header.Set("HX-Request", "true")
		}
		w := serve(mux, req)
		if w.Code != http.StatusOK {
			t.Fatalf("htmx=%v: status = %d, want 200", htmx, w.Code)
		}
		if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
			t.Errorf("htmx=%v: Content-Type = %q", htmx, ct)
		}
		body := w.Body.String()
		fullPage := strings.Contains(strings.ToLower(body), "<!doctype html>") && strings.Contains(body, `id="main-content"`)
		if fullPage == htmx {
			t.Errorf("htmx=%v: full page = %v, want %v", htmx, fullPage, !htmx)
		}
		if !strings.Contains(body, `id="sync-detail-shell-proj"`) {
			t.Errorf("htmx=%v: detail fragment missing", htmx)
		}
	}

	cases := []struct {
		name string
		deps doctor.Deps
		want string
	}{
		{"read-only", doctor.Deps{RoDB: db}, "Write operations are not available in read-only mode."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := seedOrphanObservation(t, db)
			mux := http.NewServeMux()
			doctor.Mount(mux, tc.deps)
			req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/doctor/orphans/observations/%d/project", id),
				strings.NewReader(url.Values{"project": {"alpha"}}.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("HX-Request", "true")
			w := serve(mux, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Code)
			}
			want := `<div class="inline-error" role="alert"><strong>Error:</strong> <span class="inline-error-message">` + tc.want + `</span></div>`
			if body := w.Body.String(); body != want {
				t.Errorf("body = %q, want %q", body, want)
			}
			var project *string
			if err := db.QueryRow(`SELECT project FROM observations WHERE id = ?`, id).Scan(&project); err != nil {
				t.Fatalf("query: %v", err)
			}
			if project != nil {
				t.Errorf("blocked write changed the observation project to %q", *project)
			}
		})
	}
}
