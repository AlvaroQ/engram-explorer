package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
)

// TestCodexModule_PathEditThroughSettings covers S7: the Codex home is edited
// through the generic Settings → Modules path form (reg.Validate): an invalid
// home is rejected inline and changes nothing; a valid one re-opens the
// provider on it and is persisted to the active profile.
func TestCodexModule_PathEditThroughSettings(t *testing.T) {
	f := newAgentFixture(t)
	post := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/settings/modules/codex-sessions/path",
			strings.NewReader(url.Values{"path": {path}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		f.h.ServeHTTP(w, req)
		return w
	}

	bad := filepath.Join(t.TempDir(), "no-codex")
	w := post(bad)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "codex sessions directory not found") {
		t.Fatalf("invalid home: status %d, body lacks the validation message (%.300s)", w.Code, w.Body.String())
	}
	before, _ := config.LoadProfileStore(f.home)
	if before != nil && before.Profiles["default"].ProviderCfg("codex-sessions").Path == bad {
		t.Fatal("an invalid Codex home was persisted")
	}
	f.wantOK(t, codexDetailURL)

	other := filepath.Join(t.TempDir(), "codex2")
	writeFixtureFile(t, filepath.Join(other, "sessions", "2025", "11", "01", "rollout-2025-11-01T10-00-00-x.jsonl"), codexFixtureRollout)
	if w := post(other); w.Code != http.StatusSeeOther {
		t.Fatalf("valid home: status %d, want 303 (body %.300s)", w.Code, w.Body.String())
	}
	saved, err := config.LoadProfileStore(f.home)
	if err != nil || saved.Profiles["default"].ProviderCfg("codex-sessions") != (config.ProviderConfig{Enabled: true, Path: other}) {
		t.Fatalf("persisted codex-sessions = %+v (err %v), want enabled at %s", saved, err, other)
	}
	if w := f.fetch(t, codexDetailURL); w.Code != http.StatusNotFound {
		t.Errorf("old home still served after the path change: status %d", w.Code)
	}
	f.wantOK(t, "/agent-sessions/codex/2025/11/01/rollout-2025-11-01T10-00-00-x.jsonl")
}
