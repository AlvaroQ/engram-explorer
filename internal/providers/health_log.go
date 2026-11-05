package providers

import (
	"log/slog"
	"sync"
)

// LoggerSetter is implemented by providers that log through the host's
// logger. Register hands every such provider the registry logger before any
// Detect or Open, so provider logs never go through the global slog default.
type LoggerSetter interface {
	SetLogger(log *slog.Logger)
}

// HealthLog dedupes the health-check logging of one instance. /api/health and
// the status pill poll Health every few seconds, so logging each failing call
// would flood the log: HealthLog writes one warning when the instance goes
// from healthy to failing and one info line when it recovers, and nothing for
// repeated outcomes. A nil HealthLog or one without a logger logs nothing.
type HealthLog struct {
	log     *slog.Logger
	mu      sync.Mutex
	failing bool
}

// NewHealthLog returns a HealthLog writing to log (nil logs nothing).
func NewHealthLog(log *slog.Logger) *HealthLog { return &HealthLog{log: log} }

// Failing records a failed check; it logs msg and args as a warning only when
// the previous check succeeded (or this is the first one).
func (h *HealthLog) Failing(msg string, args ...any) {
	if h == nil {
		return
	}
	h.mu.Lock()
	first := !h.failing
	h.failing = true
	h.mu.Unlock()
	if first && h.log != nil {
		h.log.Warn(msg, args...)
	}
}

// Healthy records a successful check; it logs msg and args as info only when
// the previous check failed.
func (h *HealthLog) Healthy(msg string, args ...any) {
	if h == nil {
		return
	}
	h.mu.Lock()
	recovered := h.failing
	h.failing = false
	h.mu.Unlock()
	if recovered && h.log != nil {
		h.log.Info(msg, args...)
	}
}
