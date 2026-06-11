package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseFlags_Demo verifies that --demo and ENGRAM_DEMO env var both activate
// demo mode, and that --demo-db= / ENGRAM_DEMO_DB populate the override path.
func TestParseFlags_Demo(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		envKey       string
		envVal       string
		wantDemo     bool
		wantOverride string
	}{
		{
			name:     "no flags",
			args:     nil,
			wantDemo: false,
		},
		{
			name:     "--demo flag",
			args:     []string{"--demo"},
			wantDemo: true,
		},
		{
			name:     "ENGRAM_DEMO=1 env",
			envKey:   "ENGRAM_DEMO",
			envVal:   "1",
			wantDemo: true,
		},
		{
			name:     "ENGRAM_DEMO=true env",
			envKey:   "ENGRAM_DEMO",
			envVal:   "true",
			wantDemo: true,
		},
		{
			name:     "ENGRAM_DEMO=0 env is false",
			envKey:   "ENGRAM_DEMO",
			envVal:   "0",
			wantDemo: false,
		},
		{
			name:         "--demo-db path flag",
			args:         []string{"--demo", "--demo-db=/tmp/demo.db"},
			wantDemo:     true,
			wantOverride: "/tmp/demo.db",
		},
		{
			name:         "ENGRAM_DEMO_DB env",
			envKey:       "ENGRAM_DEMO_DB",
			envVal:       "/some/path/demo.db",
			wantOverride: "/some/path/demo.db",
		},
		{
			name:     "--version does not set demo",
			args:     []string{"--version"},
			wantDemo: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Isolate env changes to each sub-test.
			if tt.envKey != "" {
				t.Setenv(tt.envKey, tt.envVal)
			}
			// Unset any other relevant vars to avoid test interference.
			if tt.envKey != "ENGRAM_DEMO" {
				t.Setenv("ENGRAM_DEMO", "")
			}
			if tt.envKey != "ENGRAM_DEMO_DB" {
				t.Setenv("ENGRAM_DEMO_DB", "")
			}

			got := parseFlags(tt.args)
			if got.demo != tt.wantDemo {
				t.Errorf("demo: got %v, want %v", got.demo, tt.wantDemo)
			}
			if got.demoDBOverride != tt.wantOverride {
				t.Errorf("demoDBOverride: got %q, want %q", got.demoDBOverride, tt.wantOverride)
			}
		})
	}
}

// TestResolveDemoDBPath_Found verifies that resolveDemoDBPath returns the path
// when the explicit override points at an existing file.
func TestResolveDemoDBPath_Found(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "engram.db")
	if err := os.WriteFile(dbPath, []byte("SQLite"), 0o644); err != nil {
		t.Fatalf("create test file: %v", err)
	}

	got, err := resolveDemoDBPath(dbPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != dbPath {
		t.Errorf("path: got %q, want %q", got, dbPath)
	}
}

// TestResolveDemoDBPath_ExplicitMissing verifies the actionable error when an
// explicit --demo-db path does not exist.
func TestResolveDemoDBPath_ExplicitMissing(t *testing.T) {
	_, err := resolveDemoDBPath("/nonexistent/path/demo.db")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	// Must mention the missing path and the flag source so the message is actionable.
	if !strings.Contains(msg, "/nonexistent/path/demo.db") {
		t.Errorf("error must mention the path, got: %q", msg)
	}
	if !strings.Contains(msg, "--demo-db") {
		t.Errorf("error must mention the flag source, got: %q", msg)
	}
}

// TestResolveDemoDBPath_NoCandidates verifies the actionable error when no
// demo DB can be found via auto-discovery (no override, no demo/engram.db file
// next to the executable or cwd).
func TestResolveDemoDBPath_NoCandidates(t *testing.T) {
	// Point cwd to an empty temp dir so demo/engram.db won't be found.
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	_, err = resolveDemoDBPath("")
	if err == nil {
		t.Fatal("expected error when demo DB is absent, got nil")
	}
	msg := err.Error()
	// Must include the seed-demo hint so the user knows how to recover.
	if !strings.Contains(msg, "seed-demo") {
		t.Errorf("error must include the seed-demo hint, got: %q", msg)
	}
}
