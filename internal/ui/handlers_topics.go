package ui

import (
	"net/http"
	"strings"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// handleTopicsPage serves GET /topics.
// Full page on direct GET; partial content when HX-Request: true.
func handleTopicsPage(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		theme := themeForRequest(r)
		q := strings.TrimSpace(r.URL.Query().Get("q"))

		topics, err := loadTopics(r, d, q)
		if err != nil {
			render(w, r, ErrorPartial("Failed to load topics: "+err.Error()))
			return
		}

		if IsHTMX(r) {
			render(w, r, TopicsPartial(topics, q, lang))
		} else {
			renderDeps(w, r, d, TopicsPage(topics, q, lang, theme, sidebarStateForRequest(r)))
		}
	}
}

// handleTopicsListPartial serves GET /topics/list.
// Always returns the bare TopicsTable partial (HTMX filter target).
func handleTopicsListPartial(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		q := strings.TrimSpace(r.URL.Query().Get("q"))

		topics, err := loadTopics(r, d, q)
		if err != nil {
			render(w, r, ErrorPartial("Failed to load topics: "+err.Error()))
			return
		}

		render(w, r, TopicsTable(topics, lang))
	}
}

// loadTopics fetches topics from the DB and applies an optional client-side
// filter on topic_key and project.  When RoDB is nil, returns an empty slice
// so the handler renders an empty state instead of panicking.
func loadTopics(r *http.Request, d Deps, q string) ([]services.TopicRow, error) {
	if d.RoDB == nil {
		return []services.TopicRow{}, nil
	}

	rows, err := services.TopicsList(d.RoDB, services.TopicsListParams{})
	if err != nil {
		return nil, err
	}

	if q == "" {
		return rows, nil
	}

	needle := strings.ToLower(q)
	filtered := rows[:0]
	for _, t := range rows {
		keyMatch := strings.Contains(strings.ToLower(t.TopicKey), needle)
		projMatch := t.Project != nil && strings.Contains(strings.ToLower(*t.Project), needle)
		if keyMatch || projMatch {
			filtered = append(filtered, t)
		}
	}
	return filtered, nil
}
