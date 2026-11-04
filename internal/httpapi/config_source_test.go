package httpapi_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	engramprovider "github.com/AlvaroQ/engram-explorer/internal/providers/engram"
)

// TestSettingsPathSwitch_WritesConfigJSONOnly covers S6: config.json is the only
// persisted configuration source. Switching the Engram database or the Claude
// projects directory from Settings updates the active profile in config.json
// and never creates or modifies explorer-settings.json.
func TestSettingsPathSwitch_WritesConfigJSONOnly(t *testing.T) {
	home := t.TempDir()
	pathA := makeEngramDB(t, "a")
	pathB := makeEngramDB(t, "b")
	claudeDir := makeClaudeDir(t)

	cfg := config.Config{ConfigHome: home, EngramDbPath: pathA, DaemonTimeoutMs: 50}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := providers.NewRegistry(logger)
	reg.Register(engramprovider.NewProvider(cfg))
	data := config.ProfileStore{
		Version:       1,
		ActiveProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {Providers: map[string]config.ProviderConfig{"engram": {Enabled: true, Path: pathA}}},
		},
	}
	reg.Boot(context.Background(), httpapi.AdaptProfile(data.Profiles["default"]))
	c := httpapi.NewContainerWithRegistry(reg, cfg, logger)
	c.CloseGrace = 0
	c.ProfileStore = config.NewStore(home, data)
	t.Cleanup(c.Close)
	f := profileFixture{c: c, h: httpapi.NewServeMux(c), home: home}

	if code := f.post(t, "/settings/engram-db", url.Values{"path": {pathB}}); code != http.StatusSeeOther {
		t.Fatalf("engram-db: status %d, want 303", code)
	}
	if code := f.post(t, "/settings/claude-dir", url.Values{"path": {claudeDir}}); code != http.StatusSeeOther {
		t.Fatalf("claude-dir: status %d, want 303", code)
	}

	saved, err := config.LoadProfileStore(home)
	if err != nil {
		t.Fatalf("LoadProfileStore: %v", err)
	}
	if got := saved.Profiles["default"].ProviderCfg("engram").Path; got != pathB {
		t.Errorf("config.json engram path: got %q, want %q", got, pathB)
	}
	if got := saved.Profiles["default"].ProviderCfg("cc-sessions").Path; got != claudeDir {
		t.Errorf("config.json cc-sessions path: got %q, want %q", got, claudeDir)
	}
	if _, err := os.Stat(filepath.Join(home, "explorer-settings.json")); !os.IsNotExist(err) {
		t.Errorf("explorer-settings.json must not be written (stat err %v)", err)
	}
}
