package codex_test

import (
	"math"
	"os"
	"testing"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
)

func uuid(n string) string { return "0199a000-0000-7000-8000-0000000000" + n }

func ts(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-12 }

type buckets struct{ In, CR, CW, Out, Reas, Total int64 }

func bucketsOf(u agentsessions.Usage) buckets {
	return buckets{u.Input, u.CacheRead, u.CacheWrite, u.Output, u.Reasoning, u.Total()}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	b, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, to, string(b))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
