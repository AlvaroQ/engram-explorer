package agentsessions

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"
)

// Paging defaults for ListSessions.
const (
	DefaultPageSize = 50
	MaxPageSize     = 500
)

// Sort keys accepted by ListParams.Sort. Anything else falls back to SortEnded.
const (
	SortEnded   = "ended"
	SortStarted = "started"
	SortCost    = "cost"
	SortTokens  = "tokens"
	SortSize    = "size"
	SortProject = "project"
)

// ListParams filters, sorts and paginates the merged session list. Zero
// values mean "no filter". Stats applies the same filters and ignores the
// sort/paging fields.
type ListParams struct {
	Query   string    // case-insensitive substring of prompt, project, cwd, branch or ID
	Agent   string    // exact agent kind
	Source  string    // exact source ID
	Project string    // exact Project display name or ProjectKey
	Model   string    // case-insensitive exact match on Models or ModelUsage keys
	From    time.Time // StartedAt >= From
	To      time.Time // StartedAt < To
	Sort    string    // one of the Sort* constants (default SortEnded)
	Order   string    // "asc" or "desc" (default "desc")
	// Page is 1-based (values < 1 mean 1); PageSize defaults to
	// DefaultPageSize and is capped at MaxPageSize.
	Page     int
	PageSize int
}

// ListResult is one page of the merged list. Facets (Projects, Models,
// Agents, Sources) are computed over every session of the healthy sources,
// before filters, so filter selectors stay complete.
type ListResult struct {
	Items    []Session     `json:"items"`
	Total    int           `json:"total"` // matches after filters
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
	Pages    int           `json:"pages"`
	HasMore  bool          `json:"has_more"`
	Projects []string      `json:"projects"`
	Models   []string      `json:"models"`
	Agents   []string      `json:"agents"`
	Sources  []SourceInfo  `json:"sources"`
	Errors   []SourceError `json:"errors,omitempty"`
}

func (p ListParams) match(s Session) bool {
	if p.Agent != "" && s.Agent != p.Agent {
		return false
	}
	if p.Source != "" && s.Source != p.Source {
		return false
	}
	if p.Project != "" && s.Project != p.Project && s.ProjectKey != p.Project {
		return false
	}
	if p.Model != "" && !usesModel(s, p.Model) {
		return false
	}
	if !p.From.IsZero() && s.StartedAt.Before(p.From) {
		return false
	}
	if !p.To.IsZero() && !s.StartedAt.Before(p.To) {
		return false
	}
	if q := strings.ToLower(strings.TrimSpace(p.Query)); q != "" {
		hay := strings.ToLower(strings.Join([]string{s.FirstPrompt, s.Project, s.CWD, s.GitBranch, s.ID}, "\x00"))
		if !strings.Contains(hay, q) {
			return false
		}
	}
	return true
}

func usesModel(s Session, model string) bool {
	for _, m := range s.Models {
		if strings.EqualFold(m, model) {
			return true
		}
	}
	for m := range s.ModelUsage {
		if strings.EqualFold(m, model) {
			return true
		}
	}
	return false
}

func filter(all []Session, p ListParams) []Session {
	out := make([]Session, 0, len(all))
	for _, s := range all {
		if p.match(s) {
			out = append(out, s)
		}
	}
	return out
}

// tieBreak orders by Source then Key ascending, regardless of sort order.
func tieBreak(a, b Session) int {
	return cmp.Or(cmp.Compare(a.Source, b.Source), cmp.Compare(a.Key, b.Key))
}

func sortSessions(ss []Session, by, order string) {
	var primary func(a, b Session) int
	switch by {
	case SortStarted:
		primary = func(a, b Session) int { return a.StartedAt.Compare(b.StartedAt) }
	case SortCost:
		primary = func(a, b Session) int { return cmp.Compare(a.Usage.CostUSD, b.Usage.CostUSD) }
	case SortTokens:
		primary = func(a, b Session) int { return cmp.Compare(a.Usage.Total(), b.Usage.Total()) }
	case SortSize:
		primary = func(a, b Session) int { return cmp.Compare(a.SizeBytes, b.SizeBytes) }
	case SortProject:
		primary = func(a, b Session) int { return cmp.Compare(a.Project, b.Project) }
	default:
		primary = func(a, b Session) int { return a.EndedAt.Compare(b.EndedAt) }
	}
	desc := order != "asc"
	slices.SortStableFunc(ss, func(a, b Session) int {
		c := primary(a, b)
		if desc {
			c = -c
		}
		if c != 0 {
			return c
		}
		return tieBreak(a, b)
	})
}

func sortedUnique(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		if k != "" {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

// ListSessions merges every source, filters, sorts and returns one page.
// A failing source is reported in ListResult.Errors and never hides the
// others; the error return is non-nil only when ctx is done.
func ListSessions(ctx context.Context, sources []Source, p ListParams) (ListResult, error) {
	c, err := Collect(ctx, sources)
	if err != nil {
		return ListResult{}, err
	}
	all := c.Sessions

	projects, models, agents := map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}
	for _, s := range all {
		projects[s.Project] = struct{}{}
		agents[s.Agent] = struct{}{}
		for _, m := range s.Models {
			models[m] = struct{}{}
		}
	}

	matched := filter(all, p)
	sortSessions(matched, p.Sort, p.Order)

	page, size := max(p.Page, 1), p.PageSize
	if size <= 0 {
		size = DefaultPageSize
	}
	size = min(size, MaxPageSize)
	total := len(matched)
	start := min((page-1)*size, total)
	end := min(start+size, total)

	items := make([]Session, end-start)
	copy(items, matched[start:end])
	return ListResult{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: size,
		Pages:    (total + size - 1) / size,
		HasMore:  end < total,
		Projects: sortedUnique(projects),
		Models:   sortedUnique(models),
		Agents:   sortedUnique(agents),
		Sources:  c.Sources,
		Errors:   c.Errors,
	}, nil
}
