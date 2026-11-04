package ui_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// TestPromptsSearch_EscapesStoredHTML guards against stored XSS through the
// FTS snippet: prompt content is attacker-influenced (any agent session can
// write it), so the rendered search result must escape it and only the match
// delimiters may become <mark> elements.
func TestPromptsSearch_EscapesStoredHTML(t *testing.T) {
	db := openTestDB(t)
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS user_prompts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT, content TEXT, project TEXT,
			created_at TEXT, sync_id TEXT)`,
		`CREATE VIRTUAL TABLE prompts_fts USING fts5(content)`,
		`INSERT INTO user_prompts (id, content, project) VALUES
			(1, '<script>alert(1)</script> needle <img src=x onerror=alert(2)> </mark><b>bold</b>', 'proj')`,
		`INSERT INTO prompts_fts (rowid, content) SELECT id, content FROM user_prompts`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("setup %q: %v", s, err)
		}
	}

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/prompts/list?q=needle", nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, raw := range []string{"<script>alert(1)", "<img src=x", "<b>bold</b>", "</mark><b>"} {
		if strings.Contains(body, raw) {
			t.Errorf("rendered body contains unescaped %q; body: %s", raw, body)
		}
	}
	for _, want := range []string{"&lt;script&gt;alert(1)&lt;/script&gt;", "<mark>needle</mark>", "&lt;b&gt;bold&lt;/b&gt;"} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered body missing %q; body: %s", want, body)
		}
	}
	if n := strings.Count(body, "<mark>"); n != 1 {
		t.Errorf("expected exactly one <mark> element (the match), got %d; body: %s", n, body)
	}
}
