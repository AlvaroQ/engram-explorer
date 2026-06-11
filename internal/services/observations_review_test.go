package services_test

import (
	"testing"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// strPtr is a test helper that returns a pointer to s.
func strPtr(s string) *string { return &s }

// sqliteLayout is the timestamp format used in Engram's DB TEXT columns.
const sqliteLayout = "2006-01-02 15:04:05"

// TestReviewDue covers the three cases: past, future, and null review_after.
func TestReviewDue(t *testing.T) {
	now := time.Now().UTC()

	past := now.Add(-24 * time.Hour).Format(sqliteLayout)
	future := now.Add(24 * time.Hour).Format(sqliteLayout)

	tests := []struct {
		name        string
		reviewAfter *string
		wantDue     bool
	}{
		{
			name:        "null review_after → not due",
			reviewAfter: nil,
			wantDue:     false,
		},
		{
			name:        "empty string review_after → not due",
			reviewAfter: strPtr(""),
			wantDue:     false,
		},
		{
			name:        "past review_after → due",
			reviewAfter: strPtr(past),
			wantDue:     true,
		},
		{
			name:        "future review_after → not due",
			reviewAfter: strPtr(future),
			wantDue:     false,
		},
		{
			name:        "malformed timestamp → not due (parse error is safe)",
			reviewAfter: strPtr("not-a-date"),
			wantDue:     false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			obs := services.ObservationRow{ReviewAfter: tt.reviewAfter}
			got := obs.ReviewDue()
			if got != tt.wantDue {
				t.Errorf("ReviewDue() = %v, want %v (review_after=%v)", got, tt.wantDue, tt.reviewAfter)
			}
		})
	}
}

// TestReviewDue_DBRoundtrip verifies that an observation with a past
// review_after scanned from a real SQLite DB has ReviewDue() == true,
// and one with a future date has ReviewDue() == false.
func TestReviewDue_DBRoundtrip(t *testing.T) {
	db, _ := newWriteDB(t)

	now := time.Now().UTC()
	past := now.Add(-time.Hour).Format(sqliteLayout)
	future := now.Add(time.Hour).Format(sqliteLayout)

	// Ensure the session exists for the FK.
	if _, err := db.Exec(
		`INSERT OR IGNORE INTO sessions (id, project, directory) VALUES ('review-sess', 'proj', '/tmp')`,
	); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	// Insert one observation with a past review_after and one with a future.
	_, err := db.Exec(
		`INSERT INTO observations (session_id, type, title, content, project, scope, created_at, updated_at, review_after)
		 VALUES ('review-sess', 'note', 'Past', 'c', 'proj', 'project', '2026-01-01 10:00:00', '2026-01-01 10:00:00', ?)`,
		past,
	)
	if err != nil {
		t.Fatalf("insert past: %v", err)
	}
	_, err = db.Exec(
		`INSERT INTO observations (session_id, type, title, content, project, scope, created_at, updated_at, review_after)
		 VALUES ('review-sess', 'note', 'Future', 'c', 'proj', 'project', '2026-01-01 10:00:00', '2026-01-01 10:00:00', ?)`,
		future,
	)
	if err != nil {
		t.Fatalf("insert future: %v", err)
	}
	// Insert one with null review_after.
	_, err = db.Exec(
		`INSERT INTO observations (session_id, type, title, content, project, scope, created_at, updated_at)
		 VALUES ('review-sess', 'note', 'Null', 'c', 'proj', 'project', '2026-01-01 10:00:00', '2026-01-01 10:00:00')`,
	)
	if err != nil {
		t.Fatalf("insert null: %v", err)
	}

	items, _, err := services.ObservationsList(db, services.ObservationListParams{
		Projects: []string{"proj"},
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("ObservationsList: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	byTitle := make(map[string]services.ObservationRow, len(items))
	for _, obs := range items {
		if obs.Title != nil {
			byTitle[*obs.Title] = obs
		}
	}

	if got := byTitle["Past"].ReviewDue(); !got {
		t.Errorf("Past: ReviewDue() = false, want true")
	}
	if got := byTitle["Future"].ReviewDue(); got {
		t.Errorf("Future: ReviewDue() = true, want false")
	}
	if got := byTitle["Null"].ReviewDue(); got {
		t.Errorf("Null: ReviewDue() = true, want false")
	}
}
