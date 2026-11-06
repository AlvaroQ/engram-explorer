package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
)

// GET /api/observations?review=due lists only memories whose review date has
// passed; without the parameter (or with another value) the list is unchanged.
func TestObservationsAPI_ReviewDueParam(t *testing.T) {
	c := newWriteContainer(t)
	execOnDB(t, c.Config.EngramDbPath,
		`INSERT INTO observations (id, session_id, type, title, content, project, review_after) VALUES
			(2, 'sess-1', 'note', 'Due', 'x', 'myproject', '2020-01-01 00:00:00'),
			(3, 'sess-1', 'note', 'Later', 'x', 'myproject', '2999-01-01 00:00:00')`)
	handler := httpapi.NewServeMux(c)

	ids := func(path string) []int64 {
		t.Helper()
		rec := serveReq(handler, http.MethodGet, path)
		var body struct {
			Items []struct {
				ID int64 `json:"id"`
			} `json:"items"`
		}
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &body) != nil {
			t.Fatalf("%s: status %d body %s", path, rec.Code, rec.Body.String())
		}
		out := []int64{}
		for _, it := range body.Items {
			out = append(out, it.ID)
		}
		return out
	}

	if got := ids("/api/observations?review=due"); len(got) != 1 || got[0] != 2 {
		t.Errorf("review=due: ids %v, want [2]", got)
	}
	if got := ids("/api/observations"); len(got) != 3 {
		t.Errorf("no param: ids %v, want all 3", got)
	}
	if got := ids("/api/observations?review=other"); len(got) != 3 {
		t.Errorf("review=other: ids %v, want all 3 (only \"due\" filters)", got)
	}
}
