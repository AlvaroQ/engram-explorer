package codex_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions/codex"
)

func TestSourceIdentity(t *testing.T) {
	src := codex.New("codex-main", "/nowhere")
	if src.ID() != "codex-main" || src.Agent() != codex.Agent || codex.Agent != "codex" {
		t.Fatalf("ID=%q Agent=%q", src.ID(), src.Agent())
	}
}

func TestDefaultHome(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	tests := []struct {
		name string
		env  map[string]string
		home string
		want string
	}{
		{"CODEX_HOME wins", map[string]string{"CODEX_HOME": "/opt/codex"}, "/Users/me", "/opt/codex"},
		{"blank CODEX_HOME ignored", map[string]string{"CODEX_HOME": "  "}, "/Users/me", filepath.Join("/Users/me", ".codex")},
		{"user home", nil, "/Users/me", filepath.Join("/Users/me", ".codex")},
		{"nothing", nil, "", ""},
	}
	for _, tt := range tests {
		if got := codex.DefaultHome(env(tt.env), tt.home); got != tt.want {
			t.Errorf("%s: DefaultHome = %q, want %q", tt.name, got, tt.want)
		}
	}
	if got := codex.DefaultHome(nil, "/Users/me"); got != filepath.Join("/Users/me", ".codex") {
		t.Errorf("nil getenv: DefaultHome = %q", got)
	}
}

func TestFingerprintIsStatOnlyAndTracksRollouts(t *testing.T) {
	ctx := context.Background()
	if fp, err := codex.New("c", filepath.Join(t.TempDir(), "absent")).Fingerprint(ctx); err != nil || fp == "" {
		t.Fatalf("missing home fingerprint = %q, %v", fp, err)
	}
	home := t.TempDir()
	dir := filepath.Join(home, "sessions", "2025", "11", "02")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := codex.New("c", home)
	empty, err := src.Fingerprint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "notes.txt"), "ignored")
	writeFile(t, filepath.Join(dir, "other.jsonl"), "ignored")
	if fp, _ := src.Fingerprint(ctx); fp != empty {
		t.Fatal("non-rollout files must not change the fingerprint")
	}
	writeFile(t, filepath.Join(dir, "rollout-one.jsonl"), "{}\n")
	one, _ := src.Fingerprint(ctx)
	if one == empty {
		t.Fatal("a new rollout must change the fingerprint")
	}
	if fp, _ := src.Fingerprint(ctx); fp != one {
		t.Fatal("fingerprint must be stable")
	}
	writeFile(t, filepath.Join(dir, "rollout-one.jsonl"), "{}\n{}\n")
	if fp, _ := src.Fingerprint(ctx); fp == one {
		t.Fatal("a grown rollout must change the fingerprint")
	}
}
