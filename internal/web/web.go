// Package web embeds the compiled React frontend and wires it with the API
// handler into a single http.Handler.  The embedded FS is populated by
// `make embed` which copies apps/frontend/dist/* into internal/web/dist/.
// A minimal placeholder dist/index.html is committed so that
// `go build ./...` always compiles without running the frontend build first.
package web

import (
	"embed"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler returns an http.Handler that:
//  1. Routes all /api/* requests to the provided api handler.
//     Unknown /api/* paths are answered with a JSON 404 — never index.html.
//  2. For every other path: stat the embedded file, serve it with the correct
//     MIME type derived from the file extension.
//  3. If the file is not found AND the path is not under /api AND the request
//     Accept header includes "text/html" → serve index.html with 200 so that
//     SPA deep-links (e.g. /sessions, /projects/foo) work after a hard reload.
//  4. Any other miss → plain 404 text response.
func Handler(api http.Handler) http.Handler {
	// Wrap the embedded FS so paths look like "index.html" not "dist/index.html".
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// dist/ is always present (placeholder at minimum), so this is a hard
		// compile-time guarantee.  Panic is appropriate here.
		panic("web: could not sub embedded dist FS: " + err.Error())
	}

	fileServer := http.FileServerFS(sub)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// --- API delegation ---
		if strings.HasPrefix(path, "/api/") || path == "/api" {
			api.ServeHTTP(w, r)
			return
		}

		// --- Static file lookup ---
		// Strip leading slash to get the FS key.
		fsPath := strings.TrimPrefix(path, "/")
		if fsPath == "" {
			fsPath = "index.html"
		}

		f, err := sub.Open(fsPath)
		if err == nil {
			// File exists: serve it directly with the correct MIME type.
			defer f.Close()

			ext := filepath.Ext(fsPath)
			if ct := mime.TypeByExtension(ext); ct != "" {
				w.Header().Set("Content-Type", ct)
			}

			// Use the file server for full Range/ETag/Last-Modified support.
			fileServer.ServeHTTP(w, r)
			return
		}

		// --- SPA fallback ---
		// Only fall back to index.html when the client accepts HTML (i.e. a
		// browser navigation, not an asset fetch or XHR).
		if acceptsHTML(r) {
			idx, err2 := sub.Open("index.html")
			if err2 == nil {
				defer idx.Close()
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				_, _ = io.Copy(w, idx)
				return
			}
		}

		http.NotFound(w, r)
	})
}

// acceptsHTML reports whether the request Accept header includes "text/html".
func acceptsHTML(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept"), ",") {
		mt := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if mt == "text/html" || mt == "text/*" || mt == "*/*" {
			return true
		}
	}
	return false
}
