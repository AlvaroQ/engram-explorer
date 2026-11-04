// Package codex reads local Codex CLI sessions (rollout JSONL files under
// <CODEX_HOME>/sessions/YYYY/MM/DD/rollout-*.jsonl) into the vendor-neutral
// agentsessions model. It understands the envelope format
// ({timestamp,type,payload}) and the 2025 pre-envelope legacy format.
package codex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
)

// Agent is the agent kind reported by every Codex source.
const Agent = "codex"

// Source is an agentsessions.Source rooted at a Codex home directory. It
// caches the parsed list by fingerprint and is safe for concurrent use.
type Source struct {
	id, home string

	mu       sync.Mutex
	fp       string
	sessions []agentsessions.Session
}

// New returns a source with the given ID reading <home>/sessions.
func New(id, home string) *Source { return &Source{id: id, home: home} }

// DefaultHome resolves the Codex home: $CODEX_HOME when set, otherwise
// <homeDir>/.codex; "" when neither is known. It is pure (inputs injected).
func DefaultHome(getenv func(string) string, homeDir string) string {
	if getenv != nil {
		if v := strings.TrimSpace(getenv("CODEX_HOME")); v != "" {
			return v
		}
	}
	if homeDir == "" {
		return ""
	}
	return filepath.Join(homeDir, ".codex")
}

// ID implements agentsessions.Source.
func (s *Source) ID() string { return s.id }

// Agent implements agentsessions.Source.
func (s *Source) Agent() string { return Agent }

type rolloutFile struct {
	key, path string
	size      int64
	mtime     time.Time
}

// walk lists regular rollout-*.jsonl files under the sessions dir, sorted by
// key. Symlinks are not followed. A missing directory is an empty source.
func (s *Source) walk(ctx context.Context) ([]rolloutFile, error) {
	root, err := filepath.EvalSymlinks(filepath.Join(s.home, "sessions"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("codex sessions dir: %w", err)
	}
	var files []rolloutFile
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if err != nil {
			if p == root {
				return err
			}
			return nil // unreadable entry: skip, never fatal
		}
		name := d.Name()
		if !d.Type().IsRegular() || !strings.HasPrefix(name, "rollout-") || !strings.HasSuffix(name, ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		files = append(files, rolloutFile{key: filepath.ToSlash(rel), path: p, size: info.Size(), mtime: info.ModTime()})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk codex sessions: %w", err)
	}
	return files, nil
}

func fingerprint(files []rolloutFile) string {
	h := sha256.New()
	for _, f := range files {
		fmt.Fprintf(h, "%s:%d:%d\n", f.key, f.size, f.mtime.UnixNano())
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Fingerprint implements agentsessions.Source (stat-only, no file reads).
func (s *Source) Fingerprint(ctx context.Context) (string, error) {
	files, err := s.walk(ctx)
	if err != nil {
		return "", err
	}
	return fingerprint(files), nil
}
