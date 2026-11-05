package doctor_test

import (
	"database/sql"
	"net/http"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/doctor"
)

const dataQualityPath = "/doctor/maintenance/data-quality"

// seedDataQuality seeds one empty title (id 1), a whitespace title (id 2) and
// an exact-duplicate group of three (ids 10-12), plus a soft-deleted copy.
func seedDataQuality(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO observations (id, type, title, project, normalized_hash, deleted_at) VALUES
		(1, 'note', '', 'repo', 'u1', NULL), (2, 'note', '  ', 'repo', 'u2', NULL),
		(10, 'note', 'Same note', 'repo', 'h1', NULL), (11, 'note', 'Same note', 'repo', 'h1', NULL),
		(12, 'note', 'Same note', 'other', 'h1', NULL), (13, 'note', 'Same note', 'repo', 'h1', '2026-01-01')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestMaintenancePage_DataQualitySection(t *testing.T) {
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{})
	body := doctorRequest(t, mux, http.MethodGet, "/settings/maintenance", "en").Body.String()
	for _, want := range []string{`id="data-quality"`, `hx-get="` + dataQualityPath + `"`, "Data quality", `id="obs-detail"`} {
		if !strings.Contains(body, want) {
			t.Errorf("maintenance page missing %q", want)
		}
	}
}

// TestDataQualityPartial_Findings (en and es): counts, the memories with their
// detail links into the section's drawer, and no write action at all.
func TestDataQualityPartial_Findings(t *testing.T) {
	db := openTestDB(t)
	seedDataQuality(t, db)
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db, RWDB: db})

	cases := map[string][]string{
		"en": {"2 memories have an empty title.", "1 group of exact duplicates (3 memories).", "3 copies", "(no title)", "Same note"},
		"es": {"2 memorias tienen el título vacío.", "1 grupo de duplicados exactos (3 memorias).", "3 copias", "(sin título)", "Same note"},
	}
	for lang, wants := range cases {
		body := doctorRequest(t, mux, http.MethodGet, dataQualityPath, lang).Body.String()
		for _, want := range append(wants, `hx-get="/observations/1"`, `hx-get="/observations/2"`, `hx-get="/observations/10"`,
			`hx-get="/observations/12"`, `hx-target="#obs-detail"`) {
			if !strings.Contains(body, want) {
				t.Errorf("lang=%s: body missing %q", lang, want)
			}
		}
		if strings.Contains(body, `/observations/13"`) {
			t.Errorf("lang=%s: a soft-deleted copy must not be listed", lang)
		}
		for _, verb := range []string{"hx-post", "hx-delete", "hx-put", "hx-patch"} {
			if strings.Contains(body, verb) {
				t.Errorf("lang=%s: data quality must not offer write actions (%s)", lang, verb)
			}
		}
	}
}

func TestDataQualityPartial_None(t *testing.T) {
	db := openTestDB(t)
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db})
	body := doctorRequest(t, mux, http.MethodGet, dataQualityPath, "en").Body.String()
	for _, want := range []string{"No memories with an empty title.", "No exact duplicates."} {
		if !strings.Contains(body, want) {
			t.Errorf("empty DB: body missing %q; got %s", want, body)
		}
	}
}

// TestDataQualityPartial_ReadOnly: the checks only read, so read-only mode
// shows the same findings.
func TestDataQualityPartial_ReadOnly(t *testing.T) {
	db := openTestDB(t)
	seedDataQuality(t, db)
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db})
	w := doctorRequest(t, mux, http.MethodGet, dataQualityPath, "en")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `hx-get="/observations/10"`) {
		t.Errorf("read-only: status %d, want the findings; got %s", w.Code, w.Body.String())
	}
}
