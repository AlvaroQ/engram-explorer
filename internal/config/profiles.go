package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

const configFileName = "config.json"

// ProviderConfig holds the per-profile settings for a single data-source
// provider. Unknown providers receive a zero value (Enabled=false).
type ProviderConfig struct {
	// Enabled is the user-set activation flag for this provider in this profile.
	Enabled bool `json:"enabled"`

	// Path is the filesystem path to the data source.
	// For Engram: path to engram.db.
	// For Claude Code sessions: path to the projects directory.
	Path string `json:"path,omitempty"`

	// DaemonURL is the Engram daemon HTTP base URL, stored per-profile so
	// different profiles can point at different daemon instances or ports.
	// Used only by the Engram provider; ignored by others.
	DaemonURL string `json:"daemonUrl,omitempty"`
}

// ProfilePreferences holds per-profile UI preference flags. It is an
// extensible struct: new fields use omitempty so old config.json files that
// lack the "preferences" key unmarshal to the zero value (all booleans false).
type ProfilePreferences struct {
	// AdvancedView, when true, shows technical columns (Rev, Dup, Sync id) in
	// the observations table. Defaults to false so casual users see a clean table.
	AdvancedView bool `json:"advancedView,omitempty"`
}

// Profile holds the provider settings for one named profile. It satisfies the
// providers.Profile interface via the ProviderCfg method.
type Profile struct {
	// Providers maps a provider ID (e.g. "engram", "cc-sessions") to its config.
	Providers map[string]ProviderConfig `json:"providers,omitempty"`

	// Preferences holds per-profile UI flags. The zero value is valid (all off).
	Preferences ProfilePreferences `json:"preferences,omitempty"`
}

// IsAdvancedView is a convenience helper that reads the AdvancedView preference
// from the profile, returning false when Preferences is zero-valued.
func (p Profile) IsAdvancedView() bool {
	return p.Preferences.AdvancedView
}

// ProviderCfg returns the ProviderConfig for the named provider. Unknown
// provider IDs return a zero ProviderConfig (Enabled=false, empty paths).
// This method makes Profile satisfy the providers.Profile interface.
func (p Profile) ProviderCfg(providerID string) ProviderConfig {
	if p.Providers == nil {
		return ProviderConfig{}
	}
	return p.Providers[providerID]
}

// ProfileStore is the top-level structure written to config.json. It holds all
// named profiles and the name of the currently active one.
type ProfileStore struct {
	// Version is the schema version, currently always 1.
	Version int `json:"version"`

	// ActiveProfile is the name of the profile used on the next boot / switch.
	ActiveProfile string `json:"activeProfile"`

	// Profiles maps profile names to their per-provider configurations.
	Profiles map[string]Profile `json:"profiles,omitempty"`

	// CCAccounts is the list of Claude Code installations tracked by the
	// explorer. Each entry points to a separate ~/.claude/projects directory.
	// Populated on first boot via SeedDefaultAccount.
	CCAccounts []CCAccount `json:"ccAccounts,omitempty"`
}

// cloneProfileStore returns a deep copy of st (maps and slices included).
func cloneProfileStore(st ProfileStore) ProfileStore {
	out := st
	if st.Profiles != nil {
		out.Profiles = make(map[string]Profile, len(st.Profiles))
		for name, p := range st.Profiles {
			out.Profiles[name] = cloneProfile(p)
		}
	}
	out.CCAccounts = append([]CCAccount(nil), st.CCAccounts...)
	return out
}

// cloneProfile returns a deep copy of p.
func cloneProfile(p Profile) Profile {
	out := p
	if p.Providers != nil {
		out.Providers = make(map[string]ProviderConfig, len(p.Providers))
		for id, c := range p.Providers {
			out.Providers[id] = c
		}
	}
	return out
}

// SetProviderCfg sets the config of one provider, allocating the map if needed.
func (p *Profile) SetProviderCfg(providerID string, c ProviderConfig) {
	if p.Providers == nil {
		p.Providers = make(map[string]ProviderConfig)
	}
	p.Providers[providerID] = c
}

// configFilePath returns the absolute path of config.json in configHome.
func configFilePath(configHome string) string {
	return filepath.Join(configHome, configFileName)
}

// LoadProfileStore reads config.json from configHome. It returns an error only
// when the file exists but cannot be decoded; a missing file returns
// ErrNotExist so callers can seed a default.
func LoadProfileStore(configHome string) (*ProfileStore, error) {
	data, err := os.ReadFile(configFilePath(configHome))
	if err != nil {
		return nil, err
	}
	var s ProfileStore
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// SaveProfileStore atomically writes store to config.json in configHome using
// a temp-file + rename pattern. This is the same approach as SaveOverrides
// (overrides.go) and is Windows-safe: a concurrent writer gets a unique temp
// name so the files never collide.
func SaveProfileStore(configHome string, store *ProfileStore) error {
	if err := os.MkdirAll(configHome, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}

	// Unique temp name so concurrent writers don't collide (critical on Windows
	// where an in-progress write blocks others on the same file name).
	f, err := os.CreateTemp(configHome, configFileName+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, configFilePath(configHome))
}

// EnsureConfig loads config.json from configHome, or seeds and saves a new
// "default" profile when no config.json exists.
//
// The seed is the one-time migration of the legacy explorer-settings.json:
// each path comes from that file when it sets one, otherwise from the built-in
// default (resolved.Defaults, or the resolved value when Defaults is empty).
// Environment overrides are never written into the seed. Once config.json
// exists the legacy file is ignored; EnsureConfig never modifies it.
func EnsureConfig(configHome string, resolved Config) (*ProfileStore, error) {
	store, err := LoadProfileStore(configHome)
	if err == nil {
		// File exists and loaded cleanly — return it as-is.
		return store, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		// File exists but is corrupt / unreadable.
		return nil, err
	}

	// No config.json yet — migrate the legacy file over the built-in defaults.
	// DaemonURL is not seeded so the default keeps following ENGRAM_PORT.
	legacy := LoadOverrides(configHome)
	base := Config{
		EngramDbPath:      firstNonEmpty(legacy.EngramDbPath, resolved.Defaults.EngramDbPath, resolved.EngramDbPath),
		ClaudeProjectsDir: firstNonEmpty(legacy.ClaudeProjectsDir, resolved.Defaults.ClaudeProjectsDir, resolved.ClaudeProjectsDir),
	}
	seed := &ProfileStore{
		Version:       1,
		ActiveProfile: "default",
		Profiles: map[string]Profile{
			"default": profileFromResolved(base),
		},
	}
	SeedDefaultAccount(seed, base.ClaudeProjectsDir)
	if err := SaveProfileStore(configHome, seed); err != nil {
		return nil, err
	}
	return seed, nil
}

// ProfileFromConfig is the exported form of profileFromResolved. It builds a
// Profile from an already-resolved Config. Used for in-memory stores (demo
// mode and the fallback when config.json cannot be loaded) and by tests.
func ProfileFromConfig(cfg Config) Profile {
	return profileFromResolved(cfg)
}

// profileFromResolved builds a Profile from an already-resolved Config.
func profileFromResolved(cfg Config) Profile {
	providers := make(map[string]ProviderConfig)

	if cfg.EngramDbPath != "" {
		providers["engram"] = ProviderConfig{
			Enabled:   true,
			Path:      cfg.EngramDbPath,
			DaemonURL: cfg.DaemonBaseURL,
		}
	}
	if cfg.ClaudeProjectsDir != "" {
		providers["cc-sessions"] = ProviderConfig{
			Enabled: true,
			Path:    cfg.ClaudeProjectsDir,
		}
	}

	return Profile{Providers: providers}
}
