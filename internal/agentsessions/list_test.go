package agentsessions_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
)

func TestListSessionsFiltersAndStampsSource(t *testing.T) {
	tests := []struct {
		name   string
		params agentsessions.ListParams
		want   []string
	}{
		{"default sort ended desc", agentsessions.ListParams{}, []string{"c2", "x2", "c3", "x1", "c1"}},
		{"agent", agentsessions.ListParams{Agent: "codex"}, []string{"x2", "x1"}},
		{"source", agentsessions.ListParams{Source: "cc-main"}, []string{"c2", "c3", "c1"}},
		{"project display", agentsessions.ListParams{Project: "engram"}, []string{"c3", "x1", "c1"}},
		{"project key", agentsessions.ListParams{Project: "/w/explorer"}, []string{"c2"}},
		{"model any position, case-insensitive", agentsessions.ListParams{Model: "HAIKU"}, []string{"c3"}},
		{"query in first prompt", agentsessions.ListParams{Query: "PROMPT X"}, []string{"x2", "x1"}},
		{"query in project", agentsessions.ListParams{Query: "explor"}, []string{"c2"}},
		{"from inclusive", agentsessions.ListParams{From: day.Add(5 * time.Hour)}, []string{"c2", "x2", "c3"}},
		{"to exclusive", agentsessions.ListParams{To: day.Add(5 * time.Hour)}, []string{"x1", "c1"}},
		{"combined", agentsessions.ListParams{Agent: "claude-code", Project: "engram", Model: "opus"}, []string{"c3", "c1"}},
		{"no match", agentsessions.ListParams{Agent: "nope"}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := agentsessions.ListSessions(context.Background(), fixtureSources(), tt.params)
			if err != nil {
				t.Fatal(err)
			}
			if got := keys(res.Items); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("keys = %v, want %v", got, tt.want)
			}
			if res.Total != len(tt.want) {
				t.Fatalf("Total = %d, want %d", res.Total, len(tt.want))
			}
			for _, s := range res.Items {
				wantAgent := map[string]string{"cc-main": "claude-code", "codex": "codex"}[s.Source]
				if wantAgent == "" || s.Agent != wantAgent {
					t.Fatalf("session %s stamped Source=%q Agent=%q", s.Key, s.Source, s.Agent)
				}
			}
			// Facets come from the unfiltered set.
			if !reflect.DeepEqual(res.Projects, []string{"engram", "explorer", "other"}) {
				t.Fatalf("Projects facet = %v", res.Projects)
			}
			if !reflect.DeepEqual(res.Agents, []string{"claude-code", "codex"}) {
				t.Fatalf("Agents facet = %v", res.Agents)
			}
			if !reflect.DeepEqual(res.Models, []string{"gpt-5-codex", "haiku", "o3", "opus", "sonnet"}) {
				t.Fatalf("Models facet = %v", res.Models)
			}
		})
	}
}

func TestListSessionsSort(t *testing.T) {
	tests := []struct {
		sort, order string
		want        []string
	}{
		{"started", "asc", []string{"c1", "x1", "c3", "x2", "c2"}},
		{"started", "desc", []string{"c2", "x2", "c3", "x1", "c1"}},
		{"cost", "desc", []string{"c2", "x1", "c1", "c3", "x2"}},
		{"tokens", "desc", []string{"x1", "c1", "c2", "c3", "x2"}},
		{"tokens", "asc", []string{"x2", "c3", "c2", "c1", "x1"}},
		{"bogus", "", []string{"c2", "x2", "c3", "x1", "c1"}},
	}
	for _, tt := range tests {
		t.Run(tt.sort+"/"+tt.order, func(t *testing.T) {
			res, err := agentsessions.ListSessions(context.Background(), fixtureSources(),
				agentsessions.ListParams{Sort: tt.sort, Order: tt.order})
			if err != nil {
				t.Fatal(err)
			}
			if got := keys(res.Items); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("keys = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestListSessionsTieBreakIsDeterministic(t *testing.T) {
	same := agentsessions.Usage{Input: 1}
	a := &fakeSource{id: "b-src", agent: "codex", sessions: []agentsessions.Session{
		sess("k2", "p", 1, 1, nil, same), sess("k1", "p", 1, 1, nil, same),
	}}
	b := &fakeSource{id: "a-src", agent: "codex", sessions: []agentsessions.Session{sess("k9", "p", 1, 1, nil, same)}}
	for _, order := range []string{"asc", "desc"} {
		res, err := agentsessions.ListSessions(context.Background(), []agentsessions.Source{a, b},
			agentsessions.ListParams{Order: order})
		if err != nil {
			t.Fatal(err)
		}
		if got := keys(res.Items); !reflect.DeepEqual(got, []string{"k9", "k1", "k2"}) {
			t.Fatalf("order %s: ties must break by source then key asc, got %v", order, got)
		}
	}
}

func TestListSessionsPaginationBounds(t *testing.T) {
	tests := []struct {
		name                        string
		page, size                  int
		wantKeys                    []string
		wantPage, wantSize, wantPgs int
		wantHasMore                 bool
	}{
		{"defaults", 0, 0, []string{"c2", "x2", "c3", "x1", "c1"}, 1, 50, 1, false},
		{"first page", 1, 2, []string{"c2", "x2"}, 1, 2, 3, true},
		{"middle", 2, 2, []string{"c3", "x1"}, 2, 2, 3, true},
		{"last partial", 3, 2, []string{"c1"}, 3, 2, 3, false},
		{"beyond end", 9, 2, []string{}, 9, 2, 3, false},
		{"negative page", -4, 2, []string{"c2", "x2"}, 1, 2, 3, true},
		{"size capped", 1, 10_000, []string{"c2", "x2", "c3", "x1", "c1"}, 1, agentsessions.MaxPageSize, 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := agentsessions.ListSessions(context.Background(), fixtureSources(),
				agentsessions.ListParams{Page: tt.page, PageSize: tt.size})
			if err != nil {
				t.Fatal(err)
			}
			if got := keys(res.Items); !reflect.DeepEqual(got, tt.wantKeys) {
				t.Fatalf("keys = %v, want %v", got, tt.wantKeys)
			}
			if res.Total != 5 || res.Page != tt.wantPage || res.PageSize != tt.wantSize ||
				res.Pages != tt.wantPgs || res.HasMore != tt.wantHasMore {
				t.Fatalf("got total=%d page=%d size=%d pages=%d more=%v", res.Total, res.Page, res.PageSize, res.Pages, res.HasMore)
			}
		})
	}
}

func TestListSessionsIsolatesFailingSources(t *testing.T) {
	boom := errors.New("disk on fire")
	srcs := append(fixtureSources(),
		&fakeSource{id: "broken", agent: "codex", err: boom},
		&fakeSource{id: "panicky", agent: "claude-code", panicMsg: "kaboom"},
	)
	res, err := agentsessions.ListSessions(context.Background(), srcs, agentsessions.ListParams{})
	if err != nil {
		t.Fatalf("a failing source must not fail the whole list: %v", err)
	}
	if res.Total != 5 {
		t.Fatalf("healthy sources hidden: total=%d", res.Total)
	}
	if len(res.Errors) != 2 {
		t.Fatalf("Errors = %+v, want 2", res.Errors)
	}
	if res.Errors[0].Source != "broken" || res.Errors[0].Agent != "codex" || !errors.Is(res.Errors[0], boom) {
		t.Fatalf("Errors[0] = %+v", res.Errors[0])
	}
	if res.Errors[1].Source != "panicky" || res.Errors[1].Err == nil {
		t.Fatalf("Errors[1] = %+v", res.Errors[1])
	}
}

func TestListSessionsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := agentsessions.ListSessions(ctx, fixtureSources(), agentsessions.ListParams{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestListSessionsNoSources(t *testing.T) {
	res, err := agentsessions.ListSessions(context.Background(), nil, agentsessions.ListParams{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Items == nil || len(res.Items) != 0 || res.Total != 0 || res.Page != 1 || res.Pages != 0 {
		t.Fatalf("empty result = %+v", res)
	}
}
