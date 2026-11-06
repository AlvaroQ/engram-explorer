package doctor_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/doctor"
)

// TestDoctorPartials_FollowLangCookie (S6): the doctor partials take no lang
// parameter; they read the request language from the render context, so the
// es cookie localizes the orphans list, its detail dialog and the sync panel.
func TestDoctorPartials_FollowLangCookie(t *testing.T) {
	db := openTestDB(t)
	id := seedOrphanObservation(t, db)
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db})
	cases := []struct{ path, es, en string }{
		{"/doctor/orphans/list", "<th>Acciones</th>", "<th>Actions</th>"},
		{fmt.Sprintf("/doctor/orphans/observations/%d/detail", id), "Directorio de trabajo de la sesión", "Session working directory"},
		{"/settings/maintenance", "Problemas de sync", "Sync Issues"},
	}
	for _, tc := range cases {
		for lang, want := range map[string]string{"es": tc.es, "en": tc.en} {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("HX-Request", "true")
			req.AddCookie(&http.Cookie{Name: "lang", Value: lang})
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), want) {
				t.Errorf("GET %s (lang=%s) = %d, want %q in body", tc.path, lang, w.Code, want)
			}
		}
	}
}
