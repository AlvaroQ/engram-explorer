package services_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// Regression: each kind of other-project row leaves no trace in the raw
// bytes of a project snapshot (one marker per kind, so a failure names the
// leaking table), the FTS indexes pass integrity-check, and cleanup removes
// the whole export directory.
func TestExportProjectSnapshot_NoForeignBytes(t *testing.T) {
	src, _ := openEngramDB(t, "engram.db")
	for _, s := range []string{
		`INSERT INTO sessions (id, project, directory) VALUES ('s-k', 'keep', '/k'), ('s-o', 'other', '/o')`,
		`INSERT INTO observations (id, session_id, type, title, content, project, sync_id) VALUES
			(1, 's-k', 'note', 'kept title', 'kept body', 'keep', 'o-k'),
			(2, 's-o', 'note', 'OTHEROBSTITLE', 'OTHEROBSCONTENT', 'other', 'o-o'),
			(3, 's-o', 'note', 'NULLPROJOBS', 'x', NULL, 'o-n')`,
		`INSERT INTO observation_versions (observation_id, version, title, content) VALUES
			(1, 1, 'kept v1', 'kept old'), (2, 1, 'OTHERVERSION', 'x')`,
		`INSERT INTO memory_relations (sync_id, source_id, target_id, reason) VALUES
			('r-x', 'o-k', 'o-o', 'OTHERRELATION'), ('r-n', 'o-k', 'o-n', 'NULLPROJRELATION')`,
		`INSERT INTO user_prompts (session_id, content, project, sync_id) VALUES
			('s-k', 'kept prompt', 'keep', 'p-k'),
			('s-o', 'OTHERPROMPT', 'other', 'p-o'),
			('s-o', 'NULLPROJPROMPT', NULL, 'p-n')`,
		`INSERT INTO sync_apply_deferred (sync_id, entity, payload) VALUES ('d-1', 'observation', 'OTHERDEFERRED')`,
	} {
		if _, err := src.Exec(s); err != nil {
			t.Fatalf("seed %.60q: %v", s, err)
		}
	}

	path, cleanup, err := services.ExportProjectSnapshot(context.Background(), src, "keep")
	if err != nil {
		t.Fatalf("ExportProjectSnapshot: %v", err)
	}
	defer cleanup()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"OTHEROBSTITLE", "OTHEROBSCONTENT", "NULLPROJOBS", "OTHERVERSION",
		"OTHERRELATION", "NULLPROJRELATION", "OTHERPROMPT", "NULLPROJPROMPT", "OTHERDEFERRED"} {
		if bytes.Contains(raw, []byte(m)) {
			t.Errorf("snapshot bytes contain %s", m)
		}
	}
	for _, m := range []string{"kept title", "kept v1", "kept prompt"} {
		if !bytes.Contains(raw, []byte(m)) {
			t.Errorf("snapshot bytes lack the kept row %q", m)
		}
	}

	snap := openSnapshot(t, path)
	for _, fts := range []string{"observations_fts", "prompts_fts"} {
		if _, err := snap.Exec(`INSERT INTO ` + fts + `(` + fts + `) VALUES ('integrity-check')`); err != nil {
			t.Errorf("%s integrity-check: %v", fts, err)
		}
	}
	_ = snap.Close()

	dir := filepath.Dir(path)
	cleanup()
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("cleanup left the export directory %s: %v", dir, err)
	}
}
