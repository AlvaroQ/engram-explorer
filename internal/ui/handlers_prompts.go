package ui

import (
	"net/http"
	"strings"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// handlePromptsListPartial serves GET /prompts/list.
// Always returns the bare list content (HTMX swap target for the search input).
func handlePromptsListPartial(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		q := strings.TrimSpace(r.URL.Query().Get("q"))

		items, searchItems, err := loadPrompts(d, q)
		if err != nil {
			RenderError(w, r, "errors.loadPrompts", err)
			return
		}

		Render(w, r, PromptsListPartialContent(items, searchItems, q, lang))
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
