// Package services — Claude Code session analytics.
// Aggregates token/cost usage across ALL transcripts for the charts surface.
// Unlike the list (which reads only HEAD for most files), stats requires a full
// scan of every file because token/cost data lives on every assistant line.
package services

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
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

// ---------------------------------------------------------------------------
// Public result types (JSON-serialized to the charts island)
// ---------------------------------------------------------------------------

// CCStatsResult is the aggregate analytics response for the cc-sessions charts.
type CCStatsResult struct {
	GeneratedAt     string             `json:"generated_at"`
	Sessions        int                `json:"sessions"`
	HasUnknownModel bool               `json:"has_unknown_model"`
	Totals          CCSessionUsage     `json:"totals"`
	ByProject       []CCProjectStat    `json:"by_project"`
	ByModel         []CCModelStat      `json:"by_model"`
	OverTime        []CCDayStat        `json:"over_time"`
	TopSessions     []CCTopSessionStat `json:"top_sessions"`
}

// CCProjectStat is per-project aggregated usage.
type CCProjectStat struct {
	Project  string         `json:"project"`
	Sessions int            `json:"sessions"`
	Usage    CCSessionUsage `json:"usage"`
}

// CCModelStat is per-model-family aggregated usage (opus|sonnet|haiku|unknown).
type CCModelStat struct {
	Model    string         `json:"model"`
	Sessions int            `json:"sessions"` // sessions that used this model at least once
	Usage    CCSessionUsage `json:"usage"`
}

// CCDayStat is per-day aggregated usage, bucketed by date(StartedAt) in UTC.
type CCDayStat struct {
	Day      string         `json:"day"` // "2006-01-02"
	Sessions int            `json:"sessions"`
	Usage    CCSessionUsage `json:"usage"`
}

// CCTopSessionStat carries enough to deep-link to the detail view.
type CCTopSessionStat struct {
	ID            string         `json:"id"`
	ProjectFolder string         `json:"project_folder"`
	Project       string         `json:"project"`
	FirstPrompt   string         `json:"first_prompt"`
	StartedAt     time.Time      `json:"started_at"`
	Usage         CCSessionUsage `json:"usage"`
}

// topSessionsLimit caps the "top sessions by cost" chart.
const topSessionsLimit = 10

// ---------------------------------------------------------------------------
// Usage aggregation helpers
// ---------------------------------------------------------------------------

// add folds another usage bucket into u (used to roll sessions into aggregates).
func (u *CCSessionUsage) add(o CCSessionUsage) {
	u.InputTokens += o.InputTokens
	u.OutputTokens += o.OutputTokens
	u.CacheWrite5m += o.CacheWrite5m
	u.CacheWrite1h += o.CacheWrite1h
	u.CacheRead += o.CacheRead
	u.TotalTokens += o.TotalTokens
	u.CostUSD += o.CostUSD
	if o.UnknownModel {
		u.UnknownModel = true
	}
}

// ccModelKey normalizes a raw model id to a coarse family for charts.
func ccModelKey(model string) string {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "opus"):
		return "opus"
	case strings.Contains(m, "sonnet"):
		return "sonnet"
	case strings.Contains(m, "haiku"):
		return "haiku"
	default:
		return "unknown"
	}
}

// ccStatsScan is the per-file result of a single transcript sweep: head metadata
// plus usage rolled up overall and split per model family.
type ccStatsScan struct {
	cwd         string
	firstAt     time.Time
	firstAtSet  bool
	firstPrompt string
	perModel    map[string]CCSessionUsage
	total       CCSessionUsage
}

// scanSessionForStats reads a whole transcript ONCE, extracting head metadata
// (cwd, first timestamp, first user prompt) from the early lines and folding
// every assistant message's usage into per-model + total accumulators. Dedups
// assistant messages by message id, matching computeSessionUsage.
func scanSessionForStats(r io.Reader) ccStatsScan {
	s := ccStatsScan{perModel: make(map[string]CCSessionUsage)}
	seen := make(map[string]struct{})

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var raw struct {
			Type      string          `json:"type"`
			Timestamp string          `json:"timestamp"`
			CWD       string          `json:"cwd"`
			Message   json.RawMessage `json:"message"`
		}
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}

		if raw.CWD != "" && s.cwd == "" {
			s.cwd = raw.CWD
		}
		if raw.Timestamp != "" && !s.firstAtSet {
			if t, err := time.Parse(time.RFC3339Nano, raw.Timestamp); err == nil {
				s.firstAt = t
				s.firstAtSet = true
			}
		}
		if s.firstPrompt == "" && raw.Type == "user" && len(raw.Message) > 0 {
			if txt := extractFirstUserText(raw.Message); txt != "" {
				s.firstPrompt = txt
			}
		}

		if raw.Type != "assistant" || len(raw.Message) == 0 {
			continue
		}
		var um ccUsageMessage
		if err := json.Unmarshal(raw.Message, &um); err != nil {
			continue
		}
		if um.ID != "" {
			if _, ok := seen[um.ID]; ok {
				continue
			}
			seen[um.ID] = struct{}{}
		}
		addMessageUsage(&s.total, um.Model, um.Usage)
		key := ccModelKey(um.Model)
		mu := s.perModel[key]
		addMessageUsage(&mu, um.Model, um.Usage)
		s.perModel[key] = mu
	}

	s.total.finalize()
	for k, mu := range s.perModel {
		mu.finalize()
		s.perModel[k] = mu
	}
	return s
}

// ---------------------------------------------------------------------------
// CCSessionsStats — full sweep aggregation
// ---------------------------------------------------------------------------

// CCSessionsStats scans EVERY transcript and returns pre-aggregated analytics.
// This is the heavy path (a full read of all files); callers should cache it by
// CCSessionsFingerprint and only recompute when a transcript changes.
func CCSessionsStats(reader CCProjectsReader) (*CCStatsResult, error) {
	res := &CCStatsResult{
		GeneratedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		ByProject:   []CCProjectStat{},
		ByModel:     []CCModelStat{},
		OverTime:    []CCDayStat{},
		TopSessions: []CCTopSessionStat{},
	}

	projectDirs, err := reader.ProjectEntries()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
			return res, nil
		}
		return nil, fmt.Errorf("list projects dir: %w", err)
	}

	byProject := make(map[string]*CCProjectStat)
	byModel := make(map[string]*CCModelStat)
	byDay := make(map[string]*CCDayStat)

	for _, pd := range projectDirs {
		if !pd.IsDir() {
			continue
		}
		projectFolder := pd.Name()
		entries, err := reader.SessionEntries(projectFolder)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(name, ".jsonl") {
				continue
			}

			f, err := reader.OpenSession(projectFolder, name)
			if err != nil {
				continue
			}
			scan := scanSessionForStats(f)
			f.Close()

			display := projectDisplayName(scan.cwd)
			if display == "" {
				display = projectFolder
			}

			res.Sessions++
			res.Totals.add(scan.total)
			if scan.total.UnknownModel {
				res.HasUnknownModel = true
			}

			p := byProject[display]
			if p == nil {
				p = &CCProjectStat{Project: display}
				byProject[display] = p
			}
			p.Sessions++
			p.Usage.add(scan.total)

			for mk, mu := range scan.perModel {
				m := byModel[mk]
				if m == nil {
					m = &CCModelStat{Model: mk}
					byModel[mk] = m
				}
				m.Sessions++
				m.Usage.add(mu)
			}

			if scan.firstAtSet {
				day := scan.firstAt.UTC().Format("2006-01-02")
				d := byDay[day]
				if d == nil {
					d = &CCDayStat{Day: day}
					byDay[day] = d
				}
				d.Sessions++
				d.Usage.add(scan.total)
			}

			res.TopSessions = append(res.TopSessions, CCTopSessionStat{
				ID:            strings.TrimSuffix(name, ".jsonl"),
				ProjectFolder: projectFolder,
				Project:       display,
				FirstPrompt:   truncateForLabel(scan.firstPrompt, 80),
				StartedAt:     scan.firstAt,
				Usage:         scan.total,
			})
		}
	}

	for _, p := range byProject {
		res.ByProject = append(res.ByProject, *p)
	}
	sort.Slice(res.ByProject, func(i, j int) bool {
		return moreUsage(res.ByProject[i].Usage, res.ByProject[j].Usage)
	})

	for _, m := range byModel {
		res.ByModel = append(res.ByModel, *m)
	}
	sort.Slice(res.ByModel, func(i, j int) bool {
		return res.ByModel[i].Usage.TotalTokens > res.ByModel[j].Usage.TotalTokens
	})

	for _, d := range byDay {
		res.OverTime = append(res.OverTime, *d)
	}
	sort.Slice(res.OverTime, func(i, j int) bool {
		return res.OverTime[i].Day < res.OverTime[j].Day
	})

	sort.Slice(res.TopSessions, func(i, j int) bool {
		return moreUsage(res.TopSessions[i].Usage, res.TopSessions[j].Usage)
	})
	if len(res.TopSessions) > topSessionsLimit {
		res.TopSessions = res.TopSessions[:topSessionsLimit]
	}

	return res, nil
}

// moreUsage orders by cost desc, then total tokens desc (stable headline order).
func moreUsage(a, b CCSessionUsage) bool {
	if a.CostUSD != b.CostUSD {
		return a.CostUSD > b.CostUSD
	}
	return a.TotalTokens > b.TotalTokens
}

// truncateForLabel collapses whitespace and caps runes with an ellipsis.
func truncateForLabel(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// ---------------------------------------------------------------------------
// Fingerprint — cheap cache key (stat-only, no file reads)
// ---------------------------------------------------------------------------

// CCSessionsFingerprint returns a hash over (path, size, mtime) of every
// transcript. It opens no files (SessionStat only), so it is cheap to compute on
// every request; callers reuse a cached CCSessionsStats result while it is
// unchanged and recompute when any transcript is added/removed/modified.
func CCSessionsFingerprint(reader CCProjectsReader) string {
	projectDirs, err := reader.ProjectEntries()
	if err != nil {
		return ""
	}

	folders := make([]string, 0, len(projectDirs))
	for _, pd := range projectDirs {
		if pd.IsDir() {
			folders = append(folders, pd.Name())
		}
	}
	sort.Strings(folders)

	h := sha256.New()
	for _, folder := range folders {
		entries, err := reader.SessionEntries(folder)
		if err != nil {
			continue
		}
		files := make([]string, 0, len(entries))
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
				files = append(files, e.Name())
			}
		}
		sort.Strings(files)
		for _, name := range files {
			fi, err := reader.SessionStat(folder, name)
			if err != nil {
				continue
			}
			fmt.Fprintf(h, "%s/%s:%d:%d\n", folder, name, fi.Size(), fi.ModTime().UnixNano())
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
