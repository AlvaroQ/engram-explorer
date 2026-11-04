package httpapi_test

import (
	"net/http"
	"net/url"
	"strconv"
	"testing"
)

// TestModuleLifecycle_EngramDisabledLater covers S4: a module enabled at boot
// and disabled afterwards answers 503 MODULE_DISABLED (no 500 from a closed
// pool), and comes back after re-enabling.
func TestModuleLifecycle_EngramDisabledLater(t *testing.T) {
	f := newLifecycleFixture(t, true, false)

	f.wantOK(t, engramRoutes...)
	f.toggle(t, "engram", false)
	f.wantDisabled(t, engramRoutes...)
	f.toggle(t, "engram", true)
	f.wantOK(t, engramRoutes...)
}

// TestModuleLifecycle_ReloadAfterReenable covers S4 + S13: after a disable and
// re-enable the Settings DB switch targets the live instance, so the data and
// /api/health follow the new database.
func TestModuleLifecycle_ReloadAfterReenable(t *testing.T) {
	f := newLifecycleFixture(t, true, false)
	f.toggle(t, "engram", false)
	f.toggle(t, "engram", true)

	pathB := makeEngramDB(t, "b")
	if code := f.post(t, "/settings/engram-db", url.Values{"path": {pathB}}); code != http.StatusSeeOther {
		t.Fatalf("engram-db: status %d, want 303", code)
	}
	var v string
	if err := f.c.RoDB.QueryRow("SELECT v FROM marker").Scan(&v); err != nil || v != "b" {
		t.Errorf("live DB after switch: marker %q (err %v), want b", v, err)
	}
	body := healthBody(t, f.h)
	if db, _ := body["db"].(map[string]any); db["path"] != pathB {
		t.Errorf("db.path: got %v, want %q", db["path"], pathB)
	}
}

// TestModuleLifecycle_ConcurrentToggle covers S4 under -race: requests running
// while the module is toggled always find their route (never 404) and the
// handler never panics.
func TestModuleLifecycle_ConcurrentToggle(t *testing.T) {
	f := newLifecycleFixture(t, true, false)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 10; i++ {
			val := url.Values{"enabled": {strconv.FormatBool(i%2 == 1)}}
			if code := f.post(t, "/settings/modules/engram/toggle", val); code != http.StatusSeeOther {
				t.Errorf("toggle %d: status %d, want 303", i, code)
			}
		}
	}()
	for {
		select {
		case <-done:
			f.wantOK(t, engramRoutes...)
			return
		default:
			for _, route := range []string{"/api/observations", "/observations", "/doctor/orphans/list"} {
				if w := f.fetch(t, route); w.Code == http.StatusNotFound {
					t.Fatalf("GET %s during toggles: 404", route)
				}
			}
		}
	}
}
