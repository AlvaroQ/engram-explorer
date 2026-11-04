package httpapi_test

import (
	"context"
	"database/sql"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/providers/providertest"
	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
)

// TestServerWiring_NoProviderIDSwitch covers S5: the HTTP wiring discovers
// providers through capability interfaces, never by switching on a provider
// ID (no `switch x.ID` / `switch x.ID()` and no case on a provider ID string).
func TestServerWiring_NoProviderIDSwitch(t *testing.T) {
	ids := map[string]bool{"engram": true, "cc-sessions": true}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sw, ok := n.(*ast.SwitchStmt)
			if !ok {
				return true
			}
			tag := sw.Tag
			if call, ok := tag.(*ast.CallExpr); ok {
				tag = call.Fun
			}
			if sel, ok := tag.(*ast.SelectorExpr); ok && sel.Sel.Name == "ID" {
				t.Errorf("%s: switch on a provider ID", fset.Position(sw.Pos()))
			}
			for _, stmt := range sw.Body.List {
				for _, expr := range stmt.(*ast.CaseClause).List {
					if lit, ok := expr.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						if v, _ := strconv.Unquote(lit.Value); ids[v] {
							t.Errorf("%s: case on provider ID %q", fset.Position(lit.Pos()), v)
						}
					}
				}
			}
			return true
		})
	}
}

// storeInstance decorates a fake instance with the SQLite store capability
// the host serves the memory JSON API from (ContainerDeps), optionally plus
// providers.Writable.
type storeInstance struct {
	*providertest.FakeInstance
	db *sql.DB
}

func (s storeInstance) ContainerDeps() (sqlite.Querier, sqlite.Querier, *config.RuntimePaths) {
	return s.db, s.db, nil
}

type writableStore struct{ storeInstance }

func (writableStore) Writable() bool { return true }

// searchStatsStore adds providers.Searcher and providers.StatsSource with
// canned results.
type searchStatsStore struct{ storeInstance }

func (searchStatsStore) Search(_ context.Context, q string, limit int) (any, error) {
	return []map[string]any{{"q": q, "limit": limit}}, nil
}

func (searchStatsStore) Stats(context.Context) (any, error) {
	return map[string]bool{"canned": true}, nil
}

// newCapabilityHandler boots a registry with one fake provider named "notes"
// (not a real provider ID) whose instances are decorated by wrap, and
// returns the served handler plus the registry.
func newCapabilityHandler(t *testing.T, opts providertest.FakeProviderOptions) (http.Handler, *providers.Registry) {
	t.Helper()
	opts.ID, opts.DetectAvailable = "notes", true
	reg := providertest.NewRegistry(t, providertest.NewFakeProfile(map[string]providers.ProviderConfig{
		"notes": {Enabled: true, Path: "/notes"},
	}), providertest.NewFakeProvider(opts))
	return httpapi.NewServeMux(newRegistryContainer(t, reg)), reg
}

func openSeeded(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+seedEngramDB(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func serve(h http.Handler, method, route, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, route, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	h.ServeHTTP(rec, req)
	return rec
}

// TestCapabilities_ProviderRoutesServedWhileEnabled covers S4 + S5: the
// routes a provider registers in Provider.Routes are served while it is
// Enabled, answer 503 MODULE_DISABLED once disabled, and come back on enable,
// without remounting.
func TestCapabilities_ProviderRoutesServedWhileEnabled(t *testing.T) {
	h, reg := newCapabilityHandler(t, providertest.FakeProviderOptions{
		Routes: func(r providers.Router) {
			r.HandleFunc("GET /api/notes/ping", func(w http.ResponseWriter, _ *http.Request) {
				if r.Live() == nil {
					http.Error(w, "no live instance", http.StatusInternalServerError)
					return
				}
				_, _ = w.Write([]byte("pong"))
			})
		},
	})

	if rec := serve(h, http.MethodGet, "/api/notes/ping", ""); rec.Code != http.StatusOK || rec.Body.String() != "pong" {
		t.Fatalf("enabled: status %d body %q, want 200 pong", rec.Code, rec.Body.String())
	}
	if err := reg.Disable(context.Background(), "notes"); err != nil {
		t.Fatal(err)
	}
	if rec := serve(h, http.MethodGet, "/api/notes/ping", ""); rec.Code != http.StatusServiceUnavailable || errorCode(rec) != "MODULE_DISABLED" {
		t.Fatalf("disabled: status %d body %q, want 503 MODULE_DISABLED", rec.Code, rec.Body.String())
	}
	if err := reg.Enable(context.Background(), "notes", providers.ProviderConfig{Enabled: true, Path: "/notes"}); err != nil {
		t.Fatal(err)
	}
	if rec := serve(h, http.MethodGet, "/api/notes/ping", ""); rec.Code != http.StatusOK {
		t.Fatalf("re-enabled: status %d, want 200", rec.Code)
	}
}

// TestCapabilities_WritableGatesWriteRoutes covers S5: a provider that serves
// the SQLite store capability gets the memory API whatever its ID; without
// providers.Writable its write routes answer 503 RW_UNAVAILABLE.
func TestCapabilities_WritableGatesWriteRoutes(t *testing.T) {
	db := openSeeded(t)
	const patch = `{"title":"renamed"}`

	readOnly, _ := newCapabilityHandler(t, providertest.FakeProviderOptions{
		Wrap: func(i *providertest.FakeInstance) providers.Instance { return storeInstance{i, db} },
	})
	if rec := serve(readOnly, http.MethodGet, "/api/observations", ""); rec.Code != http.StatusOK {
		t.Fatalf("GET /api/observations: status %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	rec := serve(readOnly, http.MethodPatch, "/api/observations/1", patch)
	if rec.Code != http.StatusServiceUnavailable || errorCode(rec) != "RW_UNAVAILABLE" {
		t.Fatalf("PATCH without Writable: status %d body %q, want 503 RW_UNAVAILABLE", rec.Code, rec.Body.String())
	}
	var title string
	if err := db.QueryRow("SELECT title FROM observations WHERE id = 1").Scan(&title); err != nil || title != "Test Obs" {
		t.Fatalf("rejected write changed the row: title %q (err %v)", title, err)
	}

	writable, _ := newCapabilityHandler(t, providertest.FakeProviderOptions{
		Wrap: func(i *providertest.FakeInstance) providers.Instance { return writableStore{storeInstance{i, db}} },
	})
	if rec := serve(writable, http.MethodPatch, "/api/observations/1", patch); rec.Code != http.StatusOK {
		t.Fatalf("PATCH with Writable: status %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
}

// TestCapabilities_SearcherAndStatsSource covers S5: /api/observations/search
// is served by the live providers.Searcher and /api/overview by the live
// providers.StatsSource; a store without them answers 503 MODULE_DISABLED.
func TestCapabilities_SearcherAndStatsSource(t *testing.T) {
	db := openSeeded(t)

	h, _ := newCapabilityHandler(t, providertest.FakeProviderOptions{
		Wrap: func(i *providertest.FakeInstance) providers.Instance { return searchStatsStore{storeInstance{i, db}} },
	})
	rec := serve(h, http.MethodGet, "/api/observations/search?q=hello&limit=7", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"items":[{"limit":7,"q":"hello"}]`) {
		t.Errorf("search: status %d body %q, want the Searcher result as items", rec.Code, rec.Body.String())
	}
	rec = serve(h, http.MethodGet, "/api/overview", "")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"canned":true}` {
		t.Errorf("overview: status %d body %q, want the StatsSource result", rec.Code, rec.Body.String())
	}

	plain, _ := newCapabilityHandler(t, providertest.FakeProviderOptions{
		Wrap: func(i *providertest.FakeInstance) providers.Instance { return storeInstance{i, db} },
	})
	for _, route := range []string{"/api/observations/search?q=hello", "/api/overview"} {
		if rec := serve(plain, http.MethodGet, route, ""); rec.Code != http.StatusServiceUnavailable || errorCode(rec) != "MODULE_DISABLED" {
			t.Errorf("GET %s without the capability: status %d body %q, want 503 MODULE_DISABLED", route, rec.Code, rec.Body.String())
		}
	}
}
