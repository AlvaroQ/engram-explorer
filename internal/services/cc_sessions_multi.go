package services

import (
	"io/fs"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"
)

// CCAccountSource binds a CCProjectsReader to a named account identity.
// It lets CCSessionsListMulti and CCSessionsStatsMulti fan out across several
// ~/.claude/projects directories and tag each session with its origin.
type CCAccountSource struct {
	ID     string
	Label  string
	Reader CCProjectsReader
}

// maxMergedHeads is the maximum number of session metadata items collected
// across all accounts before pagination is applied. Sessions beyond this limit
// are silently dropped with a warning log — it is a practical guard against
// extremely large installations.
const maxMergedHeads = 5000

// CCSessionsListMulti lists sessions across multiple CC accounts with the same
// pagination contract as CCSessionsList.
//
// Efficiency model: for each account the function performs a cheap HEAD scan
// (identical to the one inside CCSessionsList) that reads only the first
// headLineLimit lines of each transcript. After merging and sorting across all
// accounts, it performs the full-file token/cost scan only for the rows that
// will be returned on the current page — matching single-account efficiency.
func CCSessionsListMulti(accounts []CCAccountSource, p CCSessionListParams) (CCSessionListResult, error) {
	if len(accounts) == 0 {
		return CCSessionListResult{Items: []CCSessionListItem{}}, nil
	}

	// readerByID lets us look up the right reader when computing page usage.
	readerByID := make(map[string]CCProjectsReader, len(accounts))
	for _, a := range accounts {
		readerByID[a.ID] = a.Reader
	}

	var allItems []CCSessionListItem
	projectSet := make(map[string]struct{})

	for _, acc := range accounts {
		items, projects, err := headScanAccount(acc.Reader, p.Project)
		if err != nil {
			return CCSessionListResult{}, err
		}
		for i := range items {
			items[i].Account = acc.Label
			items[i].AccountID = acc.ID
		}
		allItems = append(allItems, items...)
		for _, proj := range projects {
			projectSet[proj] = struct{}{}
		}
	}

	if len(allItems) > maxMergedHeads {
		slog.Warn("cc sessions multi: head scan limit exceeded, truncating",
			"limit", maxMergedHeads, "found", len(allItems))
		allItems = allItems[:maxMergedHeads]
	}

	// Apply optional account filter (post-merge so the project union is always complete).
	if p.Account != "" {
		filtered := allItems[:0]
		for _, item := range allItems {
			if item.AccountID == p.Account {
				filtered = append(filtered, item)
			}
		}
		allItems = filtered
	}

	// Sort: EndedAt desc, tie-break by ID asc for deterministic ordering.
	sort.Slice(allItems, func(i, j int) bool {
		if allItems[i].EndedAt.Equal(allItems[j].EndedAt) {
			return allItems[i].ID < allItems[j].ID
		}
		return allItems[i].EndedAt.After(allItems[j].EndedAt)
	})

	// Integer-offset cursor pagination (same scheme as CCSessionsList).
	limit := p.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	offset := 0
	if p.Cursor != "" {
		if n, err := strconv.Atoi(p.Cursor); err == nil && n > 0 {
			offset = n
		}
	}
	total := len(allItems)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := allItems[offset:end]

	var nextCursor *string
	if end < total {
		nc := strconv.Itoa(end)
		nextCursor = &nc
	}

	// Full-file token/cost scan — only for the rows on the current page.
	for i := range page {
		r := readerByID[page[i].AccountID]
		if r == nil {
			continue
		}
		rc, err := r.OpenSession(page[i].ProjectFolder, page[i].ID+".jsonl")
		if err != nil {
			continue
		}
		page[i].Usage = computeSessionUsage(rc)
		rc.Close()
	}

	// Build sorted project union from all scanned accounts (not filtered).
	allProjects := make([]string, 0, len(projectSet))
	for proj := range projectSet {
		allProjects = append(allProjects, proj)
	}
	sort.Strings(allProjects)

	items := page
	if items == nil {
		items = []CCSessionListItem{}
	}

	// Build the ordered account info list so the UI can render filter chips.
	accountInfos := make([]CCAccountInfo, 0, len(accounts))
	for _, a := range accounts {
		accountInfos = append(accountInfos, CCAccountInfo{ID: a.ID, Label: a.Label})
	}

	return CCSessionListResult{
		Items:      items,
		Projects:   allProjects,
		Accounts:   accountInfos,
		NextCursor: nextCursor,
	}, nil
}

// headScanAccount performs the HEAD scan for a single account reader, collecting
// session metadata without computing token/cost usage.
// It mirrors the scan loop inside CCSessionsList but returns all items (no pagination)
// so CCSessionsListMulti can merge and paginate across accounts.
func headScanAccount(reader CCProjectsReader, projectFilter string) ([]CCSessionListItem, []string, error) {
	projectDirs, err := reader.ProjectEntries()
	if err != nil {
		if isNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	projectSet := make(map[string]struct{})
	var items []CCSessionListItem

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

			fi, err := reader.SessionStat(projectFolder, name)
			if err != nil {
				continue
			}

			f, err := reader.OpenSession(projectFolder, name)
			if err != nil {
				continue
			}
			head, _ := parseSessionHead(f)
			f.Close()

			sessionID := strings.TrimSuffix(name, ".jsonl")
			display := projectDisplayName(head.cwd)
			if display == "" {
				display = projectFolder
			}

			if projectFilter != "" && display != projectFilter {
				continue
			}

			startedAt := head.firstAt
			if startedAt.IsZero() {
				startedAt = fi.ModTime()
			}

			items = append(items, CCSessionListItem{
				ID:             sessionID,
				ProjectFolder:  projectFolder,
				ProjectDisplay: display,
				CWD:            head.cwd,
				GitBranch:      head.gitBranch,
				CCVersion:      head.version,
				FirstPrompt:    head.firstPrompt,
				StartedAt:      startedAt,
				EndedAt:        fi.ModTime(),
				FileSizeBytes:  fi.Size(),
			})
			projectSet[display] = struct{}{}
		}
	}

	projects := make([]string, 0, len(projectSet))
	for p := range projectSet {
		projects = append(projects, p)
	}

	return items, projects, nil
}

// isNotExist reports whether err indicates a missing directory/file, handling
// both the stdlib fs.ErrNotExist sentinel and os.IsNotExist for older errors.
func isNotExist(err error) bool {
	return os.IsNotExist(err) || isErrNotExist(err)
}

func isErrNotExist(err error) bool {
	return err != nil && strings.Contains(err.Error(), fs.ErrNotExist.Error())
}

// ---------------------------------------------------------------------------
// CCSessionsStatsMulti — aggregate analytics across multiple accounts
// ---------------------------------------------------------------------------

// CCStatsMultiResult is the response for a multi-account stats query.
// Total holds merged analytics across all accounts; ByAccount breaks them down
// per account.
type CCStatsMultiResult struct {
	Total     *CCStatsResult   `json:"total"`
	ByAccount []CCAccountStats `json:"by_account"`
}

// CCAccountStats is the per-account slice of CCStatsMultiResult.
type CCAccountStats struct {
	ID    string         `json:"id"`
	Label string         `json:"label"`
	Stats *CCStatsResult `json:"stats"`
}

// CCSessionsStatsMulti aggregates analytics from multiple CC account readers.
// Each account is scanned independently via CCSessionsStats, then their results
// are merged into a single Total with cross-account ByProject, ByModel, OverTime,
// and TopSessions (capped at topSessionsLimit).
func CCSessionsStatsMulti(accounts []CCAccountSource) (*CCStatsMultiResult, error) {
	result := &CCStatsMultiResult{
		Total:     &CCStatsResult{},
		ByAccount: []CCAccountStats{},
	}

	if len(accounts) == 0 {
		return result, nil
	}

	byProject := make(map[string]*CCProjectStat)
	byModel := make(map[string]*CCModelStat)
	byDay := make(map[string]*CCDayStat)
	var topSessions []CCTopSessionStat
	latestGenAt := ""

	for _, acc := range accounts {
		stats, err := CCSessionsStats(acc.Reader)
		if err != nil {
			return nil, err
		}

		result.ByAccount = append(result.ByAccount, CCAccountStats{
			ID:    acc.ID,
			Label: acc.Label,
			Stats: stats,
		})

		result.Total.Sessions += stats.Sessions
		result.Total.Totals.add(stats.Totals)
		if stats.HasUnknownModel {
			result.Total.HasUnknownModel = true
		}
		if stats.GeneratedAt > latestGenAt {
			latestGenAt = stats.GeneratedAt
		}

		for _, ps := range stats.ByProject {
			if entry, ok := byProject[ps.Project]; ok {
				entry.Sessions += ps.Sessions
				entry.Usage.add(ps.Usage)
			} else {
				cp := ps
				byProject[ps.Project] = &cp
			}
		}

		for _, ms := range stats.ByModel {
			if entry, ok := byModel[ms.Model]; ok {
				entry.Sessions += ms.Sessions
				entry.Usage.add(ms.Usage)
			} else {
				cm := ms
				byModel[ms.Model] = &cm
			}
		}

		for _, ds := range stats.OverTime {
			if entry, ok := byDay[ds.Day]; ok {
				entry.Sessions += ds.Sessions
				entry.Usage.add(ds.Usage)
			} else {
				cd := ds
				byDay[ds.Day] = &cd
			}
		}

		topSessions = append(topSessions, stats.TopSessions...)
	}

	result.Total.GeneratedAt = latestGenAt

	for _, ps := range byProject {
		result.Total.ByProject = append(result.Total.ByProject, *ps)
	}
	sort.Slice(result.Total.ByProject, func(i, j int) bool {
		return moreUsage(result.Total.ByProject[i].Usage, result.Total.ByProject[j].Usage)
	})

	for _, ms := range byModel {
		result.Total.ByModel = append(result.Total.ByModel, *ms)
	}
	sort.Slice(result.Total.ByModel, func(i, j int) bool {
		return result.Total.ByModel[i].Usage.TotalTokens > result.Total.ByModel[j].Usage.TotalTokens
	})

	for _, ds := range byDay {
		result.Total.OverTime = append(result.Total.OverTime, *ds)
	}
	sort.Slice(result.Total.OverTime, func(i, j int) bool {
		return result.Total.OverTime[i].Day < result.Total.OverTime[j].Day
	})

	sort.Slice(topSessions, func(i, j int) bool {
		return moreUsage(topSessions[i].Usage, topSessions[j].Usage)
	})
	if len(topSessions) > topSessionsLimit {
		topSessions = topSessions[:topSessionsLimit]
	}
	result.Total.TopSessions = topSessions

	if result.Total.ByProject == nil {
		result.Total.ByProject = []CCProjectStat{}
	}
	if result.Total.ByModel == nil {
		result.Total.ByModel = []CCModelStat{}
	}
	if result.Total.OverTime == nil {
		result.Total.OverTime = []CCDayStat{}
	}
	if result.Total.TopSessions == nil {
		result.Total.TopSessions = []CCTopSessionStat{}
	}

	return result, nil
}
