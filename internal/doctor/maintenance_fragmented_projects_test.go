package doctor_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/doctor"
)

// seedFragmented seeds "foo" (2 memories) and "/Users/x/repo/foo" (1 memory),
// plus the unrelated "api" / "API-docs" pair that must never group.
func seedFragmented(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, s := range []string{
		`INSERT INTO sessions (id, project, directory, started_at) VALUES
		 ('f1', 'foo', '/tmp', '2026-01-01 10:00:00'), ('f2', '/Users/x/repo/foo', '/tmp', '2026-01-01 10:00:00'),
		 ('f3', 'api', '/tmp', '2026-01-01 10:00:00'), ('f4', 'API-docs', '/tmp', '2026-01-01 10:00:00')`,
		`INSERT INTO observations (session_id, type, title, project) VALUES
		 ('f1', 'note', 'a', 'foo'), ('f1', 'note', 'b', 'foo'), ('f2', 'note', 'c', '/Users/x/repo/foo')`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

func projectObsCount(t *testing.T, db *sql.DB, project string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM observations WHERE project = ?`, project).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func postCombine(t *testing.T, mux http.Handler, source, target, lang string) string {
	t.Helper()
	form := url.Values{"source": {source}, "target": {target}}
	req := httptest.NewRequest(http.MethodPost, "/doctor/maintenance/fragmented-projects/combine", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: "lang", Value: lang})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST combine: status %d", w.Code)
	}
	return w.Body.String()
}

const fragmentedPath = "/doctor/maintenance/fragmented-projects"

func TestMaintenancePage_FragmentedProjectsSection(t *testing.T) {
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{})
	body := doctorRequest(t, mux, http.MethodGet, "/settings/maintenance", "en").Body.String()
	for _, want := range []string{`id="fragmented-projects"`, `hx-get="` + fragmentedPath + `"`, "Possibly fragmented projects"} {
		if !strings.Contains(body, want) {
			t.Errorf("maintenance page missing %q", want)
		}
	}
}

// TestFragmentedPartial_SuggestsCombine (en and es): the group, its counts,
// the backup link and a confirmed combine form into the suggested target.
func TestFragmentedPartial_SuggestsCombine(t *testing.T) {
	db := openTestDB(t)
	seedFragmented(t, db)
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db, RWDB: db})

	cases := map[string][]string{
		"en": {"Suggested target", "Combine into foo", `hx-confirm="Combine &#34;/Users/x/repo/foo&#34; into &#34;foo&#34;?`, "Download a backup first"},
		"es": {"Destino sugerido", "Combinar en foo", `hx-confirm="¿Combinar «/Users/x/repo/foo» en «foo»?`, "Descarga antes una copia de seguridad"},
	}
	for lang, wants := range cases {
		body := doctorRequest(t, mux, http.MethodGet, fragmentedPath, lang).Body.String()
		for _, want := range append(wants, `href="/api/db/export"`, `name="source" value="/Users/x/repo/foo"`, `name="target" value="foo"`, `hx-post="`+fragmentedPath+`/combine"`) {
			if !strings.Contains(body, want) {
				t.Errorf("lang=%s: body missing %q", lang, want)
			}
		}
		if strings.Contains(body, "API-docs") {
			t.Errorf("lang=%s: api / API-docs must not be suggested", lang)
		}
	}
}

func TestFragmentedPartial_None(t *testing.T) {
	db := openTestDB(t)
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db, RWDB: db})
	body := doctorRequest(t, mux, http.MethodGet, fragmentedPath, "en").Body.String()
	if !strings.Contains(body, "No possibly fragmented projects found.") || strings.Contains(body, "hx-post") {
		t.Errorf("want empty state without combine form, got %s", body)
	}
}

// TestFragmentedPartial_EnrolledNotCombinable: an enrolled target is shown,
// but no combine form is offered, and the reason is explained.
func TestFragmentedPartial_EnrolledNotCombinable(t *testing.T) {
	db := openTestDB(t)
	seedFragmented(t, db)
	seedEnrolledProject(t, db, "foo")
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db, RWDB: db})
	body := doctorRequest(t, mux, http.MethodGet, fragmentedPath, "en").Body.String()
	if !strings.Contains(body, "/Users/x/repo/foo") || strings.Contains(body, "hx-post") {
		t.Errorf("enrolled group must be listed without a combine form, got %s", body)
	}
	if !strings.Contains(body, "Cloud-enrolled projects cannot be combined here") {
		t.Error("enrolled group must explain why it cannot be combined")
	}
}

// TestFragmentedPartial_ReadOnly hides the form and blocks the POST.
func TestFragmentedPartial_ReadOnly(t *testing.T) {
	db := openTestDB(t)
	seedFragmented(t, db)
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db})
	body := doctorRequest(t, mux, http.MethodGet, fragmentedPath, "en").Body.String()
	if strings.Contains(body, "hx-post") || !strings.Contains(body, "Read-only mode: cleanup actions are disabled.") {
		t.Errorf("read-only: want no form and the read-only hint, got %s", body)
	}
	body = postCombine(t, mux, "/Users/x/repo/foo", "foo", "en")
	if !strings.Contains(body, "inline-error") || projectObsCount(t, db, "/Users/x/repo/foo") != 1 {
		t.Errorf("read-only POST must be blocked without changes, got %s", body)
	}
}

// TestFragmentedCombine_Success merges the source into the target and
// re-renders the section without the group.
func TestFragmentedCombine_Success(t *testing.T) {
	db := openTestDB(t)
	seedFragmented(t, db)
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db, RWDB: db})
	body := postCombine(t, mux, "/Users/x/repo/foo", "foo", "es")
	if !strings.Contains(body, "Se combinó «/Users/x/repo/foo» en «foo».") {
		t.Errorf("want the combined notice, got %s", body)
	}
	if strings.Contains(body, `name="source"`) {
		t.Error("the combined group must disappear from the refreshed section")
	}
	if projectObsCount(t, db, "foo") != 3 || projectObsCount(t, db, "/Users/x/repo/foo") != 0 {
		t.Error("memories must move to the target")
	}
}

// TestFragmentedCombine_Refusals: an unrelated pair and an enrolled target are
// refused with an inline message and no data change.
func TestFragmentedCombine_Refusals(t *testing.T) {
	db := openTestDB(t)
	seedFragmented(t, db)
	seedEnrolledProject(t, db, "foo")
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db, RWDB: db})

	body := postCombine(t, mux, "api", "API-docs", "en")
	if !strings.Contains(body, "no longer suggested as the same project") {
		t.Errorf("unrelated pair: want refusal, got %s", body)
	}
	body = postCombine(t, mux, "api", "API-docs", "es")
	if !strings.Contains(body, "ya no se sugieren como el mismo proyecto") {
		t.Errorf("unrelated pair (es): want localized refusal, got %s", body)
	}
	body = postCombine(t, mux, "/Users/x/repo/foo", "foo", "en")
	if !strings.Contains(body, "is enrolled for cloud sync") {
		t.Errorf("enrolled target: want refusal, got %s", body)
	}
	if projectObsCount(t, db, "/Users/x/repo/foo") != 1 || projectObsCount(t, db, "foo") != 2 {
		t.Error("refused combines must not change data")
	}
}
