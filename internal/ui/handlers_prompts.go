package ui

import (
	"net/http"
	"strings"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// handlePromptsPage serves GET /prompts.
// If ?q= is present, runs FTS search; otherwise returns the list.
// Full page on direct GET; partial content when HX-Request: true.
func handlePromptsPage(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		theme := themeForRequest(r)
		q := strings.TrimSpace(r.URL.Query().Get("q"))

		items, searchItems, err := loadPrompts(d, q)
		if err != nil {
			render(w, r, ErrorPartial("Failed to load prompts: "+err.Error()))
			return
		}

		if IsHTMX(r) {
			render(w, r, PromptsPartial(items, searchItems, q, lang))
		} else {
			renderDeps(w, r, d, PromptsPage(items, searchItems, q, lang, theme))
		}
	}
}

// handlePromptsListPartial serves GET /prompts/list.
// Always returns the bare list content (HTMX swap target for the search input).
func handlePromptsListPartial(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		q := strings.TrimSpace(r.URL.Query().Get("q"))

		items, searchItems, err := loadPrompts(d, q)
		if err != nil {
			render(w, r, ErrorPartial("Failed to load prompts: "+err.Error()))
			return
		}

		render(w, r, PromptsListPartialContent(items, searchItems, q, lang))
	}
}

// loadPrompts fetches either the list or search results depending on q.
// When RoDB is nil, returns empty slices so the handler can render an empty state.
func loadPrompts(d Deps, q string) ([]services.PromptRow, []services.PromptWithSnippet, error) {
	if d.RoDB == nil {
		return []services.PromptRow{}, []services.PromptWithSnippet{}, nil
	}

	if q != "" {
		results, err := services.PromptsSearch(d.RoDB, q, 50)
		if err != nil {
			return nil, nil, err
		}
		return []services.PromptRow{}, results, nil
	}

	items, err := services.PromptsList(d.RoDB, services.PromptsListParams{Limit: 100})
	if err != nil {
		return nil, nil, err
	}
	return items, []services.PromptWithSnippet{}, nil
}
