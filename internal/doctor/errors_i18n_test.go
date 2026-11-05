package doctor_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/doctor"
)

// TestDoctorErrors_Spanish: the inline errors of the orphan handlers follow
// the lang cookie.
func TestDoctorErrors_Spanish(t *testing.T) {
	db := openTestDB(t)
	id := seedOrphanObservation(t, db)
	rw := http.NewServeMux()
	doctor.Mount(rw, doctor.Deps{RoDB: db, RWDB: db})
	noDB := http.NewServeMux()
	doctor.Mount(noDB, doctor.Deps{})

	req := func(method, target string, form url.Values) *http.Request {
		var r *http.Request
		if form != nil {
			r = httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		} else {
			r = httptest.NewRequest(method, target, nil)
		}
		r.Header.Set("HX-Request", "true")
		r.AddCookie(&http.Cookie{Name: "lang", Value: "es"})
		return r
	}
	cases := []struct {
		name string
		mux  *http.ServeMux
		req  *http.Request
		want string
	}{
		{"missing observation", rw, req(http.MethodGet, "/doctor/orphans/observations/99999/detail", nil), "No se encontró la observación 99999."},
		{"bad detail id", rw, req(http.MethodGet, "/doctor/orphans/observations/abc/detail", nil), "Id de observación no válido: abc"},
		{"no database", noDB, req(http.MethodGet, fmt.Sprintf("/doctor/orphans/observations/%d/detail", id), nil), "La base de datos no está disponible."},
		{"project required", rw, req(http.MethodPost, fmt.Sprintf("/doctor/orphans/observations/%d/project", id), url.Values{"project": {""}}), "Elige un proyecto."},
		{"bad entity id", rw, req(http.MethodPost, "/doctor/orphans/observations/abc/project", url.Values{"project": {"x"}}), "Id no válido: abc"},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		tc.mux.ServeHTTP(w, tc.req)
		if !strings.Contains(w.Body.String(), `<span class="inline-error-message">`+tc.want+`</span>`) {
			t.Errorf("%s: body %q lacks %q", tc.name, w.Body.String(), tc.want)
		}
	}
}
