// Package web embeds the compiled React island bundles and wires them with
// the API handler into a single http.Handler.  The embedded FS is populated
// by `make embed` which copies apps/frontend/dist/* into internal/web/dist/.
// A minimal placeholder dist/islands/.keep is committed so that
// `go build ./...` always compiles without running the frontend build first.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler returns an http.Handler that:
//  1. Serves /islands/* and /assets/* from the embedded dist FS (island JS bundles).
//  2. Delegates everything else to the provided api handler (templ UI at root,
//     /api/*, /doctor/*, /static/* are all registered on the API ServeMux).
func Handler(api http.Handler) http.Handler {
	// Wrap the embedded FS so paths look like "islands/foo.js" not "dist/islands/foo.js".
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// dist/ is always present (placeholder at minimum), so this is a hard
		// compile-time guarantee.  Panic is appropriate here.
		panic("web: could not sub embedded dist FS: " + err.Error())
	}

	fileServer := http.FileServerFS(sub)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// --- Island/asset bundles ---
		// Only paths with these prefixes are served from the embedded dist FS.
		// Everything else (templ pages, /api/*, /doctor/*, /static/*) goes to
		// the API ServeMux.
		if strings.HasPrefix(path, "/islands/") || strings.HasPrefix(path, "/assets/") {
			fileServer.ServeHTTP(w, r)
			return
		}

		// --- Delegate all other requests to the API ServeMux ---
		// The UI module registers GET /{$} for the overview, /topics, /sessions,
		// etc. at the root level. /api/*, /doctor/*, /static/* are also on the mux.
		api.ServeHTTP(w, r)
	})
}
