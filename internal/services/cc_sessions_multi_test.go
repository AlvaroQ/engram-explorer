package services_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// ---------------------------------------------------------------------------
// CCSessionsListMulti
// ---------------------------------------------------------------------------

func TestCCSessionsListMulti_EmptyAccounts(t *testing.T) {
	res, err := services.CCSessionsListMulti(nil, services.CCSessionListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) != 0 {
		t.Errorf("want 0 items, got %d", len(res.Items))
	}
	if res.NextCursor != nil {
		t.Error("want nil NextCursor for empty accounts")
	}
}

func TestCCSessionsListMulti_SingleAccount(t *testing.T) {
	reader := newFakeReader()
	mtime := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	reader.addSession("proj", "sess1.jsonl",
		buildSimpleSession("e:/proj", "main", "2.0", "hello"), mtime)

	accounts := []services.CCAccountSource{
		{ID: "personal", Label: "Personal", Reader: reader},
	}
	res, err := services.CCSessionsListMulti(accounts, services.CCSessionListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("want 1 item, got %d", len(res.Items))
	}
	item := res.Items[0]
	if item.AccountID != "personal" {
		t.Errorf("AccountID=%q want %q", item.AccountID, "personal")
	}
	if item.Account != "Personal" {
		t.Errorf("Account=%q want %q", item.Account, "Personal")
	}
}

func TestCCSessionsListMulti_MergesAcrossAccounts(t *testing.T) {
	base := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)

	r1 := newFakeReader()
	r1.addSession("proj-a", "s1.jsonl",
		buildSimpleSession("e:/proj-a", "", "", "p1"), base.Add(2*time.Hour))

	r2 := newFakeReader()
	r2.addSession("proj-b", "s2.jsonl",
		buildSimpleSession("e:/proj-b", "", "", "p2"), base.Add(1*time.Hour))
	r2.addSession("proj-b", "s3.jsonl",
		buildSimpleSession("e:/proj-b", "", "", "p3"), base.Add(3*time.Hour))

	accounts := []services.CCAccountSource{
		{ID: "acc1", Label: "Account 1", Reader: r1},
		{ID: "acc2", Label: "Account 2", Reader: r2},
	}
	res, err := services.CCSessionsListMulti(accounts, services.CCSessionListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) != 3 {
		t.Fatalf("want 3 merged items, got %d", len(res.Items))
	}
	// Sorted by EndedAt desc: s3 (base+3h) > s1 (base+2h) > s2 (base+1h)
	if res.Items[0].ID != "s3" {
		t.Errorf("first item should be s3 (newest), got %q", res.Items[0].ID)
	}
	if res.Items[2].ID != "s2" {
		t.Errorf("last item should be s2 (oldest), got %q", res.Items[2].ID)
	}
}

func TestCCSessionsListMulti_ProjectsUnion(t *testing.T) {
	mtime := time.Now()
	r1 := newFakeReader()
	r1.addSession("fa", "s1.jsonl", buildSimpleSession("e:/alpha", "", "", "p"), mtime)

	r2 := newFakeReader()
	r2.addSession("fb", "s2.jsonl", buildSimpleSession("e:/beta", "", "", "p"), mtime)

	accounts := []services.CCAccountSource{
		{ID: "a1", Label: "L1", Reader: r1},
		{ID: "a2", Label: "L2", Reader: r2},
	}
	res, err := services.CCSessionsListMulti(accounts, services.CCSessionListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Projects) != 2 {
		t.Errorf("want 2 projects in union, got %d: %v", len(res.Projects), res.Projects)
	}
}

func TestCCSessionsListMulti_AccountFilter(t *testing.T) {
	mtime := time.Now()
	r1 := newFakeReader()
	r1.addSession("p", "s1.jsonl", buildSimpleSession("e:/p", "", "", "a1"), mtime)

	r2 := newFakeReader()
	r2.addSession("p", "s2.jsonl", buildSimpleSession("e:/p", "", "", "a2"), mtime)

	accounts := []services.CCAccountSource{
		{ID: "acc1", Label: "L1", Reader: r1},
		{ID: "acc2", Label: "L2", Reader: r2},
	}
	res, err := services.CCSessionsListMulti(accounts, services.CCSessionListParams{Account: "acc1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("want 1 item after account filter, got %d", len(res.Items))
	}
	if res.Items[0].AccountID != "acc1" {
		t.Errorf("item should be from acc1, got %q", res.Items[0].AccountID)
	}
}

func TestCCSessionsListMulti_Pagination(t *testing.T) {
	base := time.Now()
	r1 := newFakeReader()
	for i := 0; i < 5; i++ {
		mtime := base.Add(time.Duration(i) * time.Minute)
		r1.addSession("p", fmt.Sprintf("s%02d.jsonl", i),
			buildSimpleSession("e:/p", "", "", fmt.Sprintf("prompt %d", i)), mtime)
	}

	accounts := []services.CCAccountSource{
		{ID: "a", Label: "A", Reader: r1},
	}

	p1, err := services.CCSessionsListMulti(accounts, services.CCSessionListParams{Limit: 2})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(p1.Items) != 2 {
		t.Fatalf("want 2 items on page 1, got %d", len(p1.Items))
	}
	if p1.NextCursor == nil {
		t.Fatal("want NextCursor on page 1")
	}

	p2, err := services.CCSessionsListMulti(accounts, services.CCSessionListParams{Limit: 2, Cursor: *p1.NextCursor})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(p2.Items) != 2 {
		t.Fatalf("want 2 items on page 2, got %d", len(p2.Items))
	}

	p3, err := services.CCSessionsListMulti(accounts, services.CCSessionListParams{Limit: 2, Cursor: *p2.NextCursor})
	if err != nil {
		t.Fatalf("page 3: %v", err)
	}
	if len(p3.Items) != 1 {
		t.Fatalf("want 1 item on last page, got %d", len(p3.Items))
	}
	if p3.NextCursor != nil {
		t.Error("want nil NextCursor on last page")
	}
}

func TestCCSessionsListMulti_UsageOnlyForPage(t *testing.T) {
	// Create two sessions; with Limit=1 only the first page item should have
	// usage populated (non-zero CostUSD or token counts).
	base := time.Now()
	r1 := newFakeReader()

	older := base.Add(1 * time.Hour)
	newer := base.Add(2 * time.Hour)

	// Both sessions have assistant usage lines.
	s1Content := buildSimpleSession("e:/p", "", "", "first") +
		assistantUsageLine("m1", "claude-sonnet-4-6", 1000, 500, 0, 0, 0) + "\n"
	s2Content := buildSimpleSession("e:/p", "", "", "second") +
		assistantUsageLine("m2", "claude-sonnet-4-6", 2000, 1000, 0, 0, 0) + "\n"

	r1.addSession("p", "s1.jsonl", s1Content, older)
	r1.addSession("p", "s2.jsonl", s2Content, newer)

	accounts := []services.CCAccountSource{
		{ID: "a", Label: "A", Reader: r1},
	}

	// Get only the first page (1 item = the newer session s2).
	res, err := services.CCSessionsListMulti(accounts, services.CCSessionListParams{Limit: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("want 1 item, got %d", len(res.Items))
	}
	if res.Items[0].Usage.InputTokens == 0 {
		t.Error("page item should have usage populated (non-zero InputTokens)")
	}
}

func TestCCSessionsListMulti_SortByEndedAtDesc(t *testing.T) {
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	r1 := newFakeReader()
	r1.addSession("p", "old.jsonl", buildSimpleSession("e:/p", "", "", "old"), base)

	r2 := newFakeReader()
	r2.addSession("p", "new.jsonl", buildSimpleSession("e:/p", "", "", "new"), base.Add(time.Hour))

	accounts := []services.CCAccountSource{
		{ID: "a", Label: "A", Reader: r1},
		{ID: "b", Label: "B", Reader: r2},
	}
	res, err := services.CCSessionsListMulti(accounts, services.CCSessionListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) < 2 {
		t.Fatalf("want >=2 items, got %d", len(res.Items))
	}
	if res.Items[0].ID != "new" {
		t.Errorf("first item should be newest, got %q", res.Items[0].ID)
	}
}

func TestCCSessionsListMulti_ProjectFilterAcrossAccounts(t *testing.T) {
	mtime := time.Now()
	r1 := newFakeReader()
	r1.addSession("alpha-folder", "s1.jsonl",
		buildSimpleSession("e:/alpha", "", "", "alpha"), mtime)

	r2 := newFakeReader()
	r2.addSession("beta-folder", "s2.jsonl",
		buildSimpleSession("e:/beta", "", "", "beta"), mtime)
	r2.addSession("alpha-folder", "s3.jsonl",
		buildSimpleSession("e:/alpha", "", "", "alpha 2"), mtime)

	accounts := []services.CCAccountSource{
		{ID: "a1", Label: "L1", Reader: r1},
		{ID: "a2", Label: "L2", Reader: r2},
	}
	res, err := services.CCSessionsListMulti(accounts, services.CCSessionListParams{Project: "alpha"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, item := range res.Items {
		if item.ProjectDisplay != "alpha" {
			t.Errorf("project filter should only return 'alpha', got %q", item.ProjectDisplay)
		}
	}
	if len(res.Items) != 2 {
		t.Errorf("want 2 alpha sessions, got %d", len(res.Items))
	}
}

// ---------------------------------------------------------------------------
// CCSessionsStatsMulti
// ---------------------------------------------------------------------------

func TestCCSessionsStatsMulti_EmptyAccounts(t *testing.T) {
	res, err := services.CCSessionsStatsMulti(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Total.Sessions != 0 {
		t.Errorf("want 0 sessions, got %d", res.Total.Sessions)
	}
	if len(res.ByAccount) != 0 {
		t.Errorf("want empty ByAccount, got %d", len(res.ByAccount))
	}
}

func TestCCSessionsStatsMulti_MergesStats(t *testing.T) {
	mtime := time.Now()

	r1 := newFakeReader()
	r1.addSession("proj-a", "s1.jsonl",
		buildSimpleSession("e:/x/proj-a", "", "", "hi")+
			assistantUsageLine("m1", "claude-opus-4-8", 1_000_000, 1_000_000, 0, 0, 0)+"\n", mtime)

	r2 := newFakeReader()
	r2.addSession("proj-b", "s2.jsonl",
		buildSimpleSession("e:/x/proj-b", "", "", "hello")+
			assistantUsageLine("m2", "claude-sonnet-4-6", 2000, 1000, 0, 0, 0)+"\n", mtime)

	accounts := []services.CCAccountSource{
		{ID: "acc1", Label: "Personal", Reader: r1},
		{ID: "acc2", Label: "Work", Reader: r2},
	}

	res, err := services.CCSessionsStatsMulti(accounts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Total.Sessions != 2 {
		t.Errorf("want 2 total sessions, got %d", res.Total.Sessions)
	}
	if len(res.ByAccount) != 2 {
		t.Errorf("want 2 ByAccount entries, got %d", len(res.ByAccount))
	}

	// Total tokens = 2,000,000 (opus) + 3,000 (sonnet) = 2,003,000
	wantTokens := int64(2_003_000)
	if res.Total.Totals.TotalTokens != wantTokens {
		t.Errorf("want TotalTokens %d, got %d", wantTokens, res.Total.Totals.TotalTokens)
	}

	// Verify ByAccount has per-account stats.
	accounts2IDSet := map[string]bool{"acc1": false, "acc2": false}
	for _, ba := range res.ByAccount {
		accounts2IDSet[ba.ID] = true
	}
	for id, found := range accounts2IDSet {
		if !found {
			t.Errorf("ByAccount missing entry for %q", id)
		}
	}
}

func TestCCSessionsStatsMulti_TopSessionsCapped(t *testing.T) {
	mtime := time.Now()
	r1 := newFakeReader()
	// Create 15 sessions — more than topSessionsLimit (10).
	for i := 0; i < 15; i++ {
		r1.addSession("proj", fmt.Sprintf("s%02d.jsonl", i),
			buildSimpleSession("e:/proj", "", "", fmt.Sprintf("p%d", i))+
				assistantUsageLine(fmt.Sprintf("m%d", i), "claude-sonnet-4-6", int64(i*100), int64(i*50), 0, 0, 0)+"\n",
			mtime)
	}

	accounts := []services.CCAccountSource{
		{ID: "a", Label: "A", Reader: r1},
	}
	res, err := services.CCSessionsStatsMulti(accounts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Total.TopSessions) > 10 {
		t.Errorf("TopSessions should be capped at 10, got %d", len(res.Total.TopSessions))
	}
}

func TestCCSessionsStatsMulti_UnknownModelPropagates(t *testing.T) {
	mtime := time.Now()
	r1 := newFakeReader()
	r1.addSession("p", "s.jsonl",
		buildSimpleSession("e:/p", "", "", "hi")+
			assistantUsageLine("m", "future-unknown-model", 1000, 500, 0, 0, 0)+"\n", mtime)

	accounts := []services.CCAccountSource{
		{ID: "a", Label: "A", Reader: r1},
	}
	res, err := services.CCSessionsStatsMulti(accounts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Total.HasUnknownModel {
		t.Error("expected HasUnknownModel to propagate to Total")
	}
}
