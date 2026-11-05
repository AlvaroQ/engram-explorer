package codex

import (
	"cmp"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
)

// fileSummary collects list metadata and usage events from one rollout.
type fileSummary struct {
	file rolloutFile

	metaSeen, legacy                       bool
	id, rootID, cwd, branch, version, fork string
	metaTS                                 time.Time
	tcCWD, model                           string
	models                                 []string
	first, last                            time.Time
	firstPrompt                            string
	turns, messages, skipped               int

	counts, records []usageEvent
	prev            tokenUsage
	havePrev        bool
}

func (f *fileSummary) meta(_ time.Time, m sessionMeta) {
	if f.metaSeen {
		return // resume may append another session_meta: the first wins
	}
	f.metaSeen, f.legacy = true, m.legacy
	f.id, f.rootID, f.cwd, f.version, f.fork = m.ID, m.SessionID, m.CWD, m.CLIVersion, m.ForkedFromID
	if m.Git != nil {
		f.branch = m.Git.Branch
	}
	f.metaTS, _ = time.Parse(time.RFC3339Nano, m.Timestamp)
}

func (f *fileSummary) addModel(m string) {
	if m != "" && !slices.Contains(f.models, m) {
		f.models = append(f.models, m)
	}
}

func (f *fileSummary) turnContext(_ time.Time, tc turnContext) {
	if tc.Model != "" {
		f.model = tc.Model
		f.addModel(tc.Model)
	}
	if f.tcCWD == "" {
		f.tcCWD = tc.CWD
	}
}

func (f *fileSummary) item(_ time.Time, it item) {
	if it.Type != "message" {
		return
	}
	switch it.Role {
	case "user":
		text := contentText(it.Content)
		if text == "" || isSynthetic(text) {
			return
		}
		f.turns++
		f.messages++
		if f.firstPrompt == "" {
			f.firstPrompt = text
		}
	case "assistant":
		f.messages++
	}
}

// tokenCount turns cumulative totals into deltas: an unchanged total is a
// re-emit, a decreasing total starts a new baseline (delta = current).
func (f *fileSummary) tokenCount(_ time.Time, total tokenUsage) {
	if f.havePrev && total == f.prev {
		return
	}
	delta := total
	if f.havePrev && !total.decreasedFrom(f.prev) {
		delta = total.minus(f.prev)
	}
	f.prev, f.havePrev = total, true
	f.counts = append(f.counts, usageEvent{key: total.key(), model: f.model, usage: delta})
}

func (f *fileSummary) record(_ time.Time, r usageRecord) {
	model := cmp.Or(r.Model, f.model)
	f.addModel(model)
	key := ""
	if r.ResponseID != "" {
		key = "resp:" + r.ResponseID
	}
	f.records = append(f.records, usageEvent{key: key, model: model, usage: r.Usage})
}

func (f *fileSummary) stamp(ts time.Time) {
	if f.first.IsZero() {
		f.first = ts
	}
	if ts.After(f.last) {
		f.last = ts
	}
}

func (f *fileSummary) skip() { f.skipped++ }

// events are the usage increments of the file: token_usage_record lines when
// present (S4), otherwise token_count deltas.
func (f *fileSummary) events() []usageEvent {
	if len(f.records) > 0 {
		return f.records
	}
	return f.counts
}

var rolloutName = regexp.MustCompile(`^rollout-\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}-(.+)\.jsonl$`)

func (f *fileSummary) sessionID() string {
	if f.id != "" {
		return f.id
	}
	name := f.file.key[strings.LastIndex(f.file.key, "/")+1:]
	if m := rolloutName.FindStringSubmatch(name); m != nil {
		return m[1]
	}
	return strings.TrimSuffix(name, ".jsonl")
}

func (f *fileSummary) root() string { return cmp.Or(f.rootID, f.sessionID()) }

func (f *fileSummary) started() time.Time {
	for _, t := range []time.Time{f.first, f.metaTS, f.file.mtime} {
		if !t.IsZero() {
			return t
		}
	}
	return time.Time{}
}

// ended is the last line timestamp; legacy files only stamp their first
// line, so they fall back to the file modification time.
func (f *fileSummary) ended() time.Time {
	end := f.last
	if f.legacy || end.IsZero() {
		end = f.file.mtime
	}
	if s := f.started(); end.Before(s) {
		return s
	}
	return end
}

// buildSessions groups files by root session ID, orders each group by
// session start and counts every usage event key once per group (S4/S6):
// fork and resume replays are attributed to the earliest file.
func buildSessions(files []*fileSummary) []agentsessions.Session {
	groups := map[string][]*fileSummary{}
	for _, f := range files {
		groups[f.root()] = append(groups[f.root()], f)
	}
	usage := make(map[*fileSummary]agentsessions.Usage, len(files))
	modelUsage := make(map[*fileSummary]map[string]agentsessions.Usage, len(files))
	for _, g := range groups {
		slices.SortFunc(g, func(a, b *fileSummary) int {
			return cmp.Or(a.started().Compare(b.started()), cmp.Compare(a.file.key, b.file.key))
		})
		seen := map[string]bool{}
		for _, f := range g {
			var total agentsessions.Usage
			per := map[string]agentsessions.Usage{}
			for _, ev := range f.events() {
				if ev.key != "" {
					if seen[ev.key] {
						continue
					}
					seen[ev.key] = true
				}
				u := priced(ev.usage, ev.model)
				total.Add(u)
				m := cmp.Or(ev.model, agentsessions.UnknownModelKey)
				mu := per[m]
				mu.Add(u)
				per[m] = mu
			}
			usage[f] = total
			if len(per) > 0 {
				modelUsage[f] = per
			}
		}
	}

	out := make([]agentsessions.Session, 0, len(files))
	for _, f := range files {
		cwd := cmp.Or(f.cwd, f.tcCWD)
		out = append(out, agentsessions.Session{
			Agent: Agent, Key: f.file.key, ID: f.sessionID(),
			Project: agentsessions.ProjectName(cwd), ProjectKey: cwd, CWD: cwd,
			GitBranch: f.branch, AgentVersion: f.version, Models: f.models,
			StartedAt: f.started(), EndedAt: f.ended(), FirstPrompt: f.firstPrompt,
			TurnCount: f.turns, MessageCount: f.messages, SizeBytes: f.file.size,
			Usage: usage[f], ModelUsage: modelUsage[f], ForkedFrom: f.fork, SkippedLines: f.skipped,
		})
	}
	return out
}
