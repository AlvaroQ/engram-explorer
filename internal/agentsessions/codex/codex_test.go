package codex_test

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
	"github.com/AlvaroQ/engram-explorer/internal/agentsessions/codex"
)

// Example lists the Codex sessions of a home directory through the neutral
// agentsessions API. Usage buckets are disjoint and reasoning is a subset of
// output, so it is not part of the total.
func Example() {
	var src agentsessions.Source = codex.New("codex", filepath.Join("testdata", "home"))
	res, err := agentsessions.ListSessions(context.Background(), []agentsessions.Source{src},
		agentsessions.ListParams{Project: "engram-explorer", Sort: agentsessions.SortStarted, Order: "asc"})
	if err != nil {
		panic(err)
	}
	for _, s := range res.Items {
		fmt.Printf("%s %s total=%d reasoning=%d forked=%t\n",
			s.StartedAt.Format("2006-01-02T15:04"), s.Agent, s.Usage.Total(), s.Usage.Reasoning, s.ForkedFrom != "")
	}
	// Output:
	// 2025-10-01T10:00 codex total=5200 reasoning=100 forked=false
	// 2025-10-02T10:00 codex total=2800 reasoning=0 forked=false
	// 2025-10-02T11:00 codex total=1700 reasoning=0 forked=true
}
