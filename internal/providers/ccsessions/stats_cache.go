package ccsessions

import (
	"sync"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// statsCache memoizes the deprecated Claude-only stats (a full sweep of every
// transcript) behind the accounts' directory fingerprints.
type statsCache struct {
	mu  sync.Mutex
	key string
	val *services.CCStatsResult
}
