package codex

import (
	"context"
	"os"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
)

// List implements agentsessions.Source. Every rollout is parsed (usage
// dedupe spans files); the result is cached until the fingerprint changes.
func (s *Source) List(ctx context.Context) ([]agentsessions.Session, error) {
	files, err := s.walk(ctx)
	if err != nil {
		return nil, err
	}
	fp := fingerprint(files)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions != nil && fp == s.fp {
		return s.sessions, nil
	}
	summaries := make([]*fileSummary, 0, len(files))
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		sum := &fileSummary{file: f}
		if err := scanFile(f.path, sum); err != nil {
			continue // vanished or unreadable file: skip it
		}
		summaries = append(summaries, sum)
	}
	sessions := buildSessions(summaries)
	for i := range sessions {
		sessions[i].Source = s.id
	}
	s.fp, s.sessions = fp, sessions
	return sessions, nil
}

func scanFile(p string, v visitor) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return scan(f, v)
}
