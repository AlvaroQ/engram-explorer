package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// pinClock fixes the seeder clock and points HOME at an empty directory.
func pinClock(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	prev := nowFunc
	nowFunc = func() time.Time { return time.Date(2026, 3, 10, 9, 30, 0, 0, time.UTC) }
	t.Cleanup(func() { nowFunc = prev })
	return home
}

// snapshotTree maps every file under dir to its contents and mtime.
func snapshotTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		snap[rel] = fi.ModTime().UTC().Format(time.RFC3339) + "\n" + string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return snap
}

// TestRun_WritesSampleClaudeSessions: besides engram.db, the seeder writes
// sample Claude Code transcripts under <out>/claude/<project-folder>/<uuid>.jsonl
// that the dashboard's own transcript reader lists, with exact token usage
// (input, output, cache write, cache read) priced by a known model, and a
// tool_use / tool_result pair in the detail view.
func TestRun_WritesSampleClaudeSessions(t *testing.T) {
	pinClock(t)
	out := filepath.Join(t.TempDir(), "demo")
	if err := run([]string{"--out", out}, io.Discard); err != nil {
		t.Fatalf("run: %v", err)
	}

	reader := services.DiskProjectsReader{BaseDir: filepath.Join(out, "claude")}
	res, err := services.CCSessionsList(reader, services.CCSessionListParams{})
	if err != nil {
		t.Fatalf("CCSessionsList: %v", err)
	}
	if len(res.Items) < 3 {
		t.Fatalf("listed %d sample sessions, want at least 3", len(res.Items))
	}
	if !reflect.DeepEqual(res.Projects, []string{"demo-api", "demo-web", "infra"}) {
		t.Errorf("projects = %v, want [demo-api demo-web infra]", res.Projects)
	}

	toolPairs := 0
	for _, it := range res.Items {
		if it.FirstPrompt == "" || it.CWD == "" || it.GitBranch == "" || it.CCVersion == "" {
			t.Errorf("session %s: missing head metadata %+v", it.ID, it)
		}
		u := it.Usage
		if u.InputTokens == 0 || u.OutputTokens == 0 || u.CacheRead == 0 || u.CacheWrite5m+u.CacheWrite1h == 0 {
			t.Errorf("session %s: incomplete usage %+v", it.ID, u)
		}
		if u.CostUSD <= 0 || u.UnknownModel {
			t.Errorf("session %s: cost %v unknownModel %v, want a priced model", it.ID, u.CostUSD, u.UnknownModel)
		}

		d, err := services.CCSessionGetDetail(reader, it.ProjectFolder, it.ID)
		if err != nil || d == nil {
			t.Fatalf("detail %s: %v", it.ID, err)
		}
		uses := map[string]bool{}
		for _, turn := range d.Turns {
			for _, b := range turn.Blocks {
				switch b.Type {
				case "tool_use":
					uses[b.ID] = true
				case "tool_result":
					if uses[b.ToolUseID] {
						toolPairs++
					}
				}
			}
		}
		if !strings.Contains(it.StartedAt.Format(time.RFC3339), "2026-") {
			t.Errorf("session %s: started_at %v not anchored to the seed clock", it.ID, it.StartedAt)
		}
	}
	if toolPairs < len(res.Items) {
		t.Errorf("found %d tool_use/tool_result pairs across %d sessions, want one per session", toolPairs, len(res.Items))
	}
}

// TestRun_SampleSessionsAreIdempotent: re-running with --force rewrites the
// same Claude transcripts and Codex rollouts byte for byte (same files, same
// mtimes); without --force the seeder refuses and leaves them untouched.
func TestRun_SampleSessionsAreIdempotent(t *testing.T) {
	pinClock(t)
	out := filepath.Join(t.TempDir(), "demo")
	if err := run([]string{"--out", out}, io.Discard); err != nil {
		t.Fatalf("first run: %v", err)
	}
	for _, sub := range []string{"claude", "codex"} {
		if len(snapshotTree(t, filepath.Join(out, sub))) == 0 {
			t.Fatalf("no sample sessions written under %s", sub)
		}
	}
	snap := func() map[string]string {
		return map[string]string{
			"claude": fmt.Sprint(snapshotTree(t, filepath.Join(out, "claude"))),
			"codex":  fmt.Sprint(snapshotTree(t, filepath.Join(out, "codex"))),
		}
	}
	first := snap()

	if err := run([]string{"--out", out, "--force"}, io.Discard); err != nil {
		t.Fatalf("forced run: %v", err)
	}
	if again := snap(); !reflect.DeepEqual(first, again) {
		t.Errorf("forced re-run changed the sample sessions")
	}

	err := run([]string{"--out", out}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("run without --force = %v, want an 'already exists' error", err)
	}
	if again := snap(); !reflect.DeepEqual(first, again) {
		t.Errorf("refused run changed the sample sessions")
	}
}

// TestRun_RefusesEngramHome: the seeder never writes inside ~/.engram.
func TestRun_RefusesEngramHome(t *testing.T) {
	home := pinClock(t)
	err := run([]string{"--out", filepath.Join(home, ".engram", "demo")}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "SAFETY") {
		t.Fatalf("run into ~/.engram = %v, want a SAFETY error", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".engram")); !os.IsNotExist(err) {
		t.Errorf("~/.engram was created (stat err %v)", err)
	}
}
