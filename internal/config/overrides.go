package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const overridesFileName = "explorer-settings.json"

// Overrides holds user-set path overrides. Empty fields mean "no override".
type Overrides struct {
	EngramDbPath      string `json:"engramDbPath,omitempty"`
	ClaudeProjectsDir string `json:"claudeProjectsDir,omitempty"`
}

// OverridesPath returns the absolute path of the overrides file for a config home.
func OverridesPath(configHome string) string {
	return filepath.Join(configHome, overridesFileName)
}

// LoadOverrides reads the overrides file; a missing or unreadable file yields an
// empty Overrides (never an error — overrides are best-effort).
func LoadOverrides(configHome string) Overrides {
	var o Overrides
	data, err := os.ReadFile(OverridesPath(configHome))
	if err != nil {
		return Overrides{}
	}
	_ = json.Unmarshal(data, &o)
	return o
}

// SaveOverrides atomically writes the overrides file (temp file + rename).
func SaveOverrides(configHome string, o Overrides) error {
	if err := os.MkdirAll(configHome, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return err
	}
	// Unique temp name so concurrent writers don't collide on the same file
	// (notably on Windows, where an in-progress write blocks others).
	f, err := os.CreateTemp(configHome, overridesFileName+".*.tmp")
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
	return os.Rename(tmp, OverridesPath(configHome))
}
