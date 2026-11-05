package agentsessions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
)

// SourceError reports a source that failed; the other sources still answer.
type SourceError struct {
	Source string `json:"source"`
	Agent  string `json:"agent"`
	Err    error  `json:"-"`
}

func (e SourceError) Error() string {
	return fmt.Sprintf("source %s (%s): %v", e.Source, e.Agent, e.Err)
}

// Unwrap exposes the underlying error to errors.Is/As.
func (e SourceError) Unwrap() error { return e.Err }

// SourceInfo describes a participating source (for filter chips).
type SourceInfo struct {
	ID    string `json:"id"`
	Agent string `json:"agent"`
}

// Collected is the merged, unfiltered output of every source.
type Collected struct {
	Sessions []Session
	Sources  []SourceInfo // every source, in input order
	Errors   []SourceError
}

// Collect lists every source concurrently, stamping Source/Agent on each
// session. Failures (errors or panics) are isolated per source and returned
// in source order. Only ctx cancellation fails the whole call.
func Collect(ctx context.Context, sources []Source) (Collected, error) {
	type result struct {
		sessions []Session
		err      error
	}
	results := make([]result, len(sources))
	var wg sync.WaitGroup
	for i, src := range sources {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results[i] = result{err: fmt.Errorf("panic: %v", r)}
				}
			}()
			ss, err := src.List(ctx)
			results[i] = result{sessions: ss, err: err}
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return Collected{}, err
	}

	c := Collected{Sources: make([]SourceInfo, 0, len(sources))}
	for i, src := range sources {
		id, agent := src.ID(), src.Agent()
		c.Sources = append(c.Sources, SourceInfo{ID: id, Agent: agent})
		if results[i].err != nil {
			c.Errors = append(c.Errors, SourceError{Source: id, Agent: agent, Err: results[i].err})
			continue
		}
		for _, s := range results[i].sessions {
			s.Source, s.Agent = id, agent
			c.Sessions = append(c.Sessions, s)
		}
	}
	return c, nil
}

// Fingerprint combines every source fingerprint (in order) into one cache key.
// A failing source contributes its error text, so recovery changes the key.
func Fingerprint(ctx context.Context, sources []Source) string {
	h := sha256.New()
	for _, src := range sources {
		fp, err := src.Fingerprint(ctx)
		if err != nil {
			fp = "error:" + err.Error()
		}
		fmt.Fprintf(h, "%s\x00%s\x00%s\n", src.ID(), src.Agent(), fp)
	}
	return hex.EncodeToString(h.Sum(nil))
}
