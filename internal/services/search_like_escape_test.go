package services_test

import (
	"fmt"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// Regression: under trigram, an all-short query takes the title LIKE path.
// LIKE wildcards (% and _) and the escape character are matched literally,
// and quotes or SQL keywords in the query are inert data.
func TestObservationsSearch_TrigramLikeEscaping(t *testing.T) {
	db := newSearchDB(t, tokenizeTrigram)
	for _, title := range []string{"100% go", "1000 go", "a_b", "axb", `c\d`, "other"} {
		addObs(t, db, title, "body", "")
	}
	for _, tc := range []struct {
		q          string
		want       string
		matchedAny bool
	}{
		{"%", "[1]", false},
		{"0%", "[1]", false},
		{"_", "[3]", false},
		{"a_", "[3]", false},
		{`c\`, "[5]", false},
		{"x' OR 1=1", "[]", false},
		{"go a_", "[3 2 1]", true},
	} {
		res, err := services.ObservationsSearch(db, tc.q, 10)
		if err != nil {
			t.Errorf("%q: %v", tc.q, err)
			continue
		}
		if got := fmt.Sprint(searchIDs(res.Items)); got != tc.want || res.MatchedAny != tc.matchedAny {
			t.Errorf("%q: got %s MatchedAny=%v, want %s MatchedAny=%v", tc.q, got, res.MatchedAny, tc.want, tc.matchedAny)
		}
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM observations`).Scan(&n); err != nil || n != 6 {
		t.Errorf("observations count = %d (%v), want 6 unchanged", n, err)
	}
}
