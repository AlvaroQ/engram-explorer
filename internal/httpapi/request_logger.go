package httpapi

import (
	"log/slog"
	"net/http"
	"time"
)

// responseRecorder wraps ResponseWriter to capture the status code.
type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.status = code
	rr.ResponseWriter.WriteHeader(code)
}

// requestLogger logs each request with method, path, status, and latency.
func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rr := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rr, r)
		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rr.status,
			"latency_ms", time.Since(start).Milliseconds(),
		)
	})
}
