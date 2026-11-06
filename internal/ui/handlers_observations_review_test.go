package ui_test

import (
	"database/sql"
	"html"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// openReviewDueDB seeds two due memories (alpha, beta), one deleted due
// memory, one with a future review date and one without.
func openReviewDueDB(t *testing.T) *sql.DB {
	t.Helper()
	db := openObservationsTestDB(t)
	if _, err := db.Exec(`INSERT INTO observations (id, type, title, project, created_at, updated_at, review_after, deleted_at) VALUES
		(1, 'note', 'Due alpha', 'alpha', '2026-01-01 10:00:00', '2026-01-01 10:00:00', '2020-01-01 00:00:00', NULL),
		(2, 'note', 'Due beta', 'beta', '2026-01-02 10:00:00', '2026-01-02 10:00:00', '2020-01-01 00:00:00', NULL),
		(3, 'note', 'Due deleted', 'alpha', '2026-01-03 10:00:00', '2026-01-03 10:00:00', '2020-01-01 00:00:00', '2026-01-04 00:00:00'),
		(4, 'note', 'Later alpha', 'alpha', '2026-01-05 10:00:00', '2026-01-05 10:00:00', '2999-01-01 00:00:00', NULL),
		(5, 'note', 'Plain alpha', 'alpha', '2026-01-06 10:00:00', '2026-01-06 10:00:00', NULL, NULL)`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return db
}

func getReviewPage(t *testing.T, db *sql.DB, path string) string {
	t.Helper()
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db, AdvancedView: func() bool { return false }})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d", path, w.Code)
	}
	return w.Body.String()
}

var reviewChipRE = regexp.MustCompile(`<a[^>]*id="obs-review-due"[^>]*>[^<]*</a>`)

// reviewChip returns the chip element, its unescaped href and its text.
func reviewChip(t *testing.T, body string) (tag, href, text string) {
	t.Helper()
	tag = reviewChipRE.FindString(body)
	if tag == "" {
		t.Fatalf("review due chip missing; body: %.800s", body)
	}
	if m := regexp.MustCompile(`href="([^"]*)"`).FindStringSubmatch(tag); m != nil {
		href = html.UnescapeString(m[1])
	}
	text = strings.TrimSpace(html.UnescapeString(tag[strings.Index(tag, ">")+1 : strings.LastIndex(tag, "<")]))
	return tag, href, text
}

func TestObservationsPage_ReviewDueChipCountsAndLinks(t *testing.T) {
	db := openReviewDueDB(t)

	tag, href, text := reviewChip(t, getReviewPage(t, db, "/observations"))
	if text != "Review due (2)" || !strings.Contains(tag, `aria-pressed="false"`) || href != "/observations?review=due" {
		t.Errorf("all projects: text %q href %q tag %s; want Review due (2), not pressed, link enabling the filter", text, href, tag)
	}

	// The count and the link keep the other active filters.
	tag, href, text = reviewChip(t, getReviewPage(t, db, "/observations?project=alpha&type=note"))
	if text != "Review due (1)" || !strings.Contains(href, "project=alpha") || !strings.Contains(href, "type=note") || !strings.Contains(href, "review=due") {
		t.Errorf("alpha: text %q href %q tag %s", text, href, tag)
	}

	// The list partial (filter changes) re-renders the chip with the new count.
	_, _, text = reviewChip(t, getReviewPage(t, db, "/observations/list?project=beta"))
	if text != "Review due (1)" {
		t.Errorf("list partial beta: text %q, want Review due (1)", text)
	}
}

func TestObservationsPage_ReviewDueActive(t *testing.T) {
	db := openReviewDueDB(t)
	body := getReviewPage(t, db, "/observations?review=due&project=alpha&project=beta&limit=1")

	tag, href, text := reviewChip(t, body)
	if text != "Review due (2)" || !strings.Contains(tag, `aria-pressed="true"`) {
		t.Errorf("active chip: text %q tag %s, want pressed with the count", text, tag)
	}
	if strings.Contains(href, "review=") || !strings.Contains(href, "project=alpha") || !strings.Contains(href, "project=beta") {
		t.Errorf("active chip href %q must drop review and keep the projects", href)
	}
	if !strings.Contains(body, `type="hidden" name="review" value="due"`) {
		t.Error("filter form must carry review=due so other filter changes keep it")
	}
	if !strings.Contains(body, "Due beta") || strings.Contains(body, "Plain alpha") || strings.Contains(body, "Later alpha") {
		t.Errorf("active filter must list only due memories; body: %.1500s", body)
	}
	m := regexp.MustCompile(`hx-get="(/observations/list\?[^"]*)"`).FindAllStringSubmatch(body, -1)
	var loadMore string
	for _, s := range m {
		if u := html.UnescapeString(s[1]); strings.Contains(u, "cursor=") {
			loadMore = u
		}
	}
	if !strings.Contains(loadMore, "review=due") || !strings.Contains(loadMore, "project=beta") {
		t.Errorf("load-more URL %q must keep review=due and the projects", loadMore)
	}
}

func TestObservationsPage_ReviewDueChipHiddenWhenNothingDue(t *testing.T) {
	db := openReviewDueDB(t)
	if body := getReviewPage(t, db, "/observations?project=gamma"); strings.Contains(body, `id="obs-review-due"`) {
		t.Error("chip must be hidden when nothing is due and the filter is off")
	}
	// Active with no results still offers the chip to turn the filter off.
	tag, _, text := reviewChip(t, getReviewPage(t, db, "/observations?review=due&project=gamma"))
	if text != "Review due (0)" || !strings.Contains(tag, `aria-pressed="true"`) {
		t.Errorf("active empty: text %q tag %s", text, tag)
	}
}
