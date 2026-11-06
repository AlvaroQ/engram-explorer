package services

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"
	"time"
)

// CCSessionSummary is one Claude Code transcript summarized by a full read:
// head metadata, exact usage (overall and per model id) and turn counts. It is
// what the vendor-neutral agent-session adapter maps into agentsessions.Session.
type CCSessionSummary struct {
	ProjectFolder  string
	ID             string // file name without .jsonl
	CWD            string
	ProjectDisplay string
	GitBranch      string
	Version        string
	FirstPrompt    string
	StartedAt      time.Time
	EndedAt        time.Time // file mtime
	SizeBytes      int64
	Usage          CCSessionUsage
	// ModelUsage splits Usage per raw model id. Only models that carried
	// tokens are listed (Claude Code's zero-token "<synthetic>" turns are not).
	ModelUsage map[string]CCSessionUsage
	Models     []string // sorted keys of ModelUsage
	// UserTurns counts human prompts (user text that is not marker-only);
	// Messages counts those plus unique assistant messages.
	UserTurns int
	Messages  int
}

// CCSessionSummaries reads EVERY transcript under reader in full. It is the
// heavy path (like CCSessionsStats); callers cache the result by
// CCSessionsFingerprint. A missing projects directory yields no sessions.
func CCSessionSummaries(reader CCProjectsReader) ([]CCSessionSummary, error) {
	projectDirs, err := reader.ProjectEntries()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list projects dir: %w", err)
	}
	var out []CCSessionSummary
	for _, pd := range projectDirs {
		if !pd.IsDir() {
			continue
		}
		folder := pd.Name()
		entries, err := reader.SessionEntries(folder)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".jsonl") {
				continue
			}
			fi, err := reader.SessionStat(folder, name)
			if err != nil {
				continue
			}
			f, err := reader.OpenSession(folder, name)
			if err != nil {
				continue
			}
			s := summarizeSession(f)
			_ = f.Close()
			s.ProjectFolder, s.ID = folder, strings.TrimSuffix(name, ".jsonl")
			s.EndedAt, s.SizeBytes = fi.ModTime(), fi.Size()
			if s.StartedAt.IsZero() {
				s.StartedAt = fi.ModTime()
			}
			if s.ProjectDisplay = projectDisplayName(s.CWD); s.ProjectDisplay == "" {
				s.ProjectDisplay = folder
			}
			out = append(out, s)
		}
	}
	return out, nil
}

// summarizeSession reads one transcript, deduping assistant usage by message
// id exactly like computeSessionUsage.
func summarizeSession(r io.Reader) CCSessionSummary {
	s := CCSessionSummary{ModelUsage: map[string]CCSessionUsage{}}
	seen := map[string]struct{}{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var raw ccRawLine
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}
		if s.CWD == "" {
			s.CWD = raw.CWD
		}
		if s.GitBranch == "" {
			s.GitBranch = raw.GitBranch
		}
		if s.Version == "" {
			s.Version = raw.Version
		}
		if s.StartedAt.IsZero() && raw.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339Nano, raw.Timestamp); err == nil {
				s.StartedAt = t
			}
		}
		if len(raw.Message) == 0 {
			continue
		}
		switch raw.Type {
		case "user":
			if text := extractFirstUserText(raw.Message); strings.TrimSpace(text) != "" && !IsMarkerOnly(text) {
				s.UserTurns++
				s.Messages++
				if s.FirstPrompt == "" {
					s.FirstPrompt = text
				}
			}
		case "assistant":
			var um ccUsageMessage
			if err := json.Unmarshal(raw.Message, &um); err != nil {
				continue
			}
			if um.ID != "" {
				if _, dup := seen[um.ID]; dup {
					continue
				}
				seen[um.ID] = struct{}{}
			}
			s.Messages++
			addMessageUsage(&s.Usage, um.Model, um.Usage)
			if um.Model != "" && carriesTokens(um.Usage) {
				mu := s.ModelUsage[um.Model]
				addMessageUsage(&mu, um.Model, um.Usage)
				s.ModelUsage[um.Model] = mu
			}
		}
	}
	s.Usage.finalize()
	for m, mu := range s.ModelUsage {
		mu.finalize()
		s.ModelUsage[m] = mu
		s.Models = append(s.Models, m)
	}
	sort.Strings(s.Models)
	return s
}

func carriesTokens(u ccRawUsage) bool {
	return u.InputTokens > 0 || u.OutputTokens > 0 || u.CacheCreationInputTokens > 0 ||
		u.CacheReadInputTokens > 0 || u.CacheCreation.Ephemeral5m > 0 || u.CacheCreation.Ephemeral1h > 0
}
