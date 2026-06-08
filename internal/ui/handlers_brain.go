package ui

import (
	"net/http"
)

// handleBrainPage serves GET /brain.
// Renders the Layout shell with the "brain" island inside a full-bleed container.
func handleBrainPage(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		theme := themeForRequest(r)
		render(w, r, BrainPage(lang, theme))
	}
}
