package main

import (
	"os"
	"strings"
	"testing"
)

// TestRun_RemovedDemoOptionsFailFast covers the removal of demo mode: the old
// --demo / --demo-db flags and ENGRAM_DEMO / ENGRAM_DEMO_DB variables make the
// binary fail before it loads configuration or serves anything, with a message
// naming the replacement. Silently ignoring them would serve the user's real
// data to someone who asked for the demo.
func TestRun_RemovedDemoOptionsFailFast(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		envKey string
		envVal string
		want   string
	}{
		{name: "--demo flag", args: []string{"--demo"}, want: "--demo"},
		{name: "--demo-db flag", args: []string{"--demo-db=/tmp/demo.db"}, want: "--demo-db"},
		{name: "bare --demo-db flag", args: []string{"--demo-db", "/tmp/demo.db"}, want: "--demo-db"},
		{name: "ENGRAM_DEMO env", envKey: "ENGRAM_DEMO", envVal: "1", want: "ENGRAM_DEMO"},
		{name: "ENGRAM_DEMO=0 env is still rejected", envKey: "ENGRAM_DEMO", envVal: "0", want: "ENGRAM_DEMO"},
		{name: "ENGRAM_DEMO_DB env", envKey: "ENGRAM_DEMO_DB", envVal: "/tmp/demo.db", want: "ENGRAM_DEMO_DB"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			dataDir := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("ENGRAM_DATA_DIR", dataDir)
			t.Setenv("ENGRAM_DEMO", "")
			t.Setenv("ENGRAM_DEMO_DB", "")
			// Safety net: if the check regressed, run would continue and fail
			// to listen on an invalid port instead of blocking the test.
			t.Setenv("DASHBOARD_PORT", "-1")
			if tc.envKey != "" {
				t.Setenv(tc.envKey, tc.envVal)
			}

			err := run(tc.args)
			if err == nil {
				t.Fatal("run returned nil, want an error naming the replacement")
			}
			msg := err.Error()
			for _, s := range []string{tc.want, "demo mode was removed", "make demo",
				"ENGRAM_DATA_DIR", "CLAUDE_PROJECTS_DIR", "ENGRAM_DASH_READONLY"} {
				if !strings.Contains(msg, s) {
					t.Errorf("error %q does not mention %q", msg, s)
				}
			}
			for _, dir := range []string{home, dataDir} {
				entries, err := os.ReadDir(dir)
				if err != nil {
					t.Fatal(err)
				}
				if len(entries) != 0 {
					t.Errorf("run created files before failing in %s: %v", dir, entries)
				}
			}
		})
	}
}

// TestRun_NoDemoOptionsPassesTheCheck: with no removed option present, run
// gets past the check (it then fails on the invalid port, which proves it
// went on to load the configuration).
func TestRun_NoDemoOptionsPassesTheCheck(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("ENGRAM_DATA_DIR", t.TempDir())
	t.Setenv("ENGRAM_DEMO", "")
	t.Setenv("ENGRAM_DEMO_DB", "")
	t.Setenv("DASHBOARD_PORT", "-1")

	err := run(nil)
	if err == nil || strings.Contains(err.Error(), "demo mode was removed") {
		t.Fatalf("run(nil) = %v, want a listen error unrelated to demo mode", err)
	}
}
