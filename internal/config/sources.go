package config

// This file wires the configuration sources together; the precedence is
// documented in the package comment (config.go): env > config.json > default.

// OpenStore loads config.json from resolved.ConfigHome (seeding it once from
// the legacy explorer-settings.json when it does not exist) and returns a
// persisted Store that applies resolved.EnvOverrides in memory only.
//
// Installs that predate Claude Code accounts get the default account here,
// once, so request handlers never write on a read path.
func OpenStore(resolved Config) (*Store, error) {
	data, err := EnsureConfig(resolved.ConfigHome, resolved)
	if err != nil {
		return nil, err
	}
	s := NewStore(resolved.ConfigHome, *data)
	s.env = resolved.EnvOverrides
	if len(data.CCAccounts) == 0 {
		dir := data.Profiles[data.ActiveProfile].ProviderCfg("cc-sessions").Path
		if err := s.SeedDefaultCCAccount(firstNonEmpty(dir, resolved.Defaults.ClaudeProjectsDir, resolved.ClaudeProjectsDir)); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// NewMemoryStore returns an ephemeral Store holding a single "default" profile
// built from cfg (ProfileFromConfig). Nothing it does is ever written to disk:
// demo mode uses it so the demo database is served and config.json is never
// read or modified, and boot falls back to it when config.json is unreadable.
func NewMemoryStore(cfg Config) *Store {
	data := ProfileStore{
		Version:       1,
		ActiveProfile: "default",
		Profiles:      map[string]Profile{"default": ProfileFromConfig(cfg)},
	}
	SeedDefaultAccount(&data, cfg.ClaudeProjectsDir)
	return NewEphemeralStore(data)
}

// Effective returns a copy of p with the environment overrides applied: a
// set ENGRAM_DATA_DIR / CLAUDE_PROJECTS_DIR replaces the provider path (and
// enables a provider the profile does not configure at all; an explicit
// "enabled": false is respected), and ENGRAM_DAEMON_URL replaces the daemon URL.
func (s *Store) Effective(p Profile) Profile {
	out := cloneProfile(p)
	override := func(id, path, daemonURL string) {
		pc, exists := out.Providers[id]
		if path == "" && (daemonURL == "" || !exists) {
			return
		}
		if path != "" {
			pc.Path = path
			pc.Enabled = pc.Enabled || !exists
		}
		if daemonURL != "" {
			pc.DaemonURL = daemonURL
		}
		out.SetProviderCfg(id, pc)
	}
	override("engram", s.env.EngramDbPath, s.env.DaemonBaseURL)
	override("cc-sessions", s.env.ClaudeProjectsDir, "")
	return out
}

// EffectiveCCAccounts returns the tracked Claude Code accounts with
// CLAUDE_PROJECTS_DIR (when set) replacing the path of the "default" account.
func (s *Store) EffectiveCCAccounts() []CCAccount {
	accounts := s.CCAccounts()
	if s.env.ClaudeProjectsDir != "" {
		for i := range accounts {
			if accounts[i].ID == "default" {
				accounts[i].Path = s.env.ClaudeProjectsDir
			}
		}
	}
	return accounts
}
