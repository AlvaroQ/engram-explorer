package agentsessions_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
)

func TestUsageTotalExcludesReasoning(t *testing.T) {
	tests := []struct {
		name string
		u    agentsessions.Usage
		want int64
	}{
		{"zero", agentsessions.Usage{}, 0},
		{"all buckets", agentsessions.Usage{Input: 100, CacheRead: 20, CacheWrite: 5, Output: 40, Reasoning: 30}, 165},
		{"reasoning only", agentsessions.Usage{Reasoning: 99}, 0},
		{"output with reasoning subset", agentsessions.Usage{Output: 10, Reasoning: 10}, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.u.Total(); got != tt.want {
				t.Fatalf("Total() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestUsageAddMergesBucketsFlagsAndBreakdown(t *testing.T) {
	a := agentsessions.Usage{
		Input: 1, CacheRead: 2, CacheWrite: 3, Output: 4, Reasoning: 1, CostUSD: 0.5,
		Breakdown: map[string]int64{"cache_write_5m": 3},
	}
	shared := a.Breakdown
	b := agentsessions.Usage{
		Input: 10, CacheRead: 20, CacheWrite: 30, Output: 40, Reasoning: 5, CostUSD: 1.25,
		UnknownModel: true,
		Breakdown:    map[string]int64{"cache_write_5m": 7, "cache_write_1h": 23},
	}
	a.Add(b)

	want := agentsessions.Usage{Input: 11, CacheRead: 22, CacheWrite: 33, Output: 44, Reasoning: 6, CostUSD: 1.75, UnknownModel: true}
	if a.Input != want.Input || a.CacheRead != want.CacheRead || a.CacheWrite != want.CacheWrite ||
		a.Output != want.Output || a.Reasoning != want.Reasoning || a.CostUSD != want.CostUSD || !a.UnknownModel {
		t.Fatalf("Add() = %+v, want %+v", a, want)
	}
	if a.Breakdown["cache_write_5m"] != 10 || a.Breakdown["cache_write_1h"] != 23 {
		t.Fatalf("Breakdown = %v", a.Breakdown)
	}
	if shared["cache_write_5m"] != 3 || len(shared) != 1 {
		t.Fatalf("Add mutated a breakdown map shared by a copy: %v", shared)
	}
	if b.Breakdown["cache_write_5m"] != 7 {
		t.Fatalf("Add mutated the argument breakdown: %v", b.Breakdown)
	}

	// UnknownModel is sticky: adding a clean usage keeps it set.
	a.Add(agentsessions.Usage{Input: 1})
	if !a.UnknownModel {
		t.Fatal("UnknownModel must stay true after merging a known-model usage")
	}
}

func TestProjectName(t *testing.T) {
	tests := map[string]string{
		"":                             "",
		"/Users/me/code/engram":        "engram",
		"/Users/me/code/engram/":       "engram",
		`C:\Users\me\code\engram`:      "engram",
		"engram":                       "engram",
		"/":                            "",
		"/Users/me/code/engram-x//":    "engram-x",
		`e:\Repos\engram-explorer\\`:   "engram-explorer",
		"/home/u/work/with space/proj": "proj",
	}
	for in, want := range tests {
		if got := agentsessions.ProjectName(in); got != want {
			t.Errorf("ProjectName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestErrInvalidKeyIsNotFound(t *testing.T) {
	wrapped := fmt.Errorf("codex: %w", agentsessions.ErrInvalidKey)
	if !errors.Is(wrapped, agentsessions.ErrNotFound) {
		t.Fatal("ErrInvalidKey must match ErrNotFound so hosts can answer 404")
	}
	if !errors.Is(wrapped, agentsessions.ErrInvalidKey) {
		t.Fatal("ErrInvalidKey must stay distinguishable")
	}
}
