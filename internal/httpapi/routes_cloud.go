package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// cloudRoutes registers the cloud CLI control endpoints.
//
// Rate limits:
//   - enroll / unenroll / sync  → 10 req / 60 s
//   - sync-all                  → 5 req / 60 s
func cloudRoutes(mux *http.ServeMux, c *Container) {
	mutationRL := newRateLimiter(60_000, 10)
	syncAllRL := newRateLimiter(60_000, 5)

	cloud := services.NewCloudControlService(services.CloudControlOptions{
		AuditLogPath:  c.Config.AuditLogPath,
		EngramDataDir: c.Config.EngramDataDir,
		RWDB:          c.RWDB,
	})

	mux.Handle("POST /api/cloud/enroll",
		withRateLimit(mutationRL, c.Config.ExposeDetails, handleCloudEnroll(c, cloud)))
	mux.Handle("POST /api/cloud/unenroll",
		withRateLimit(mutationRL, c.Config.ExposeDetails, handleCloudUnenroll(c, cloud)))
	mux.Handle("POST /api/cloud/sync",
		withRateLimit(mutationRL, c.Config.ExposeDetails, handleCloudSync(c, cloud)))
	mux.Handle("POST /api/cloud/sync-all",
		withRateLimit(syncAllRL, c.Config.ExposeDetails, handleCloudSyncAll(c, cloud)))
	mux.HandleFunc("GET /api/cloud/capabilities", handleCloudCapabilities(c, cloud))
}

// ---------------------------------------------------------------------------
// Request body shapes
// ---------------------------------------------------------------------------

// mutationBody is the request body for enroll/unenroll.
// `confirm` must be exactly `true` (mirrors the Node Zod MutationBody).
type mutationBody struct {
	Project string `json:"project"`
	Confirm *bool  `json:"confirm"`
}

// syncBody is the request body for sync.
type syncBody struct {
	Project string `json:"project"`
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func handleCloudEnroll(c *Container, cloud *services.CloudControlService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		project, ok := parseMutationBody(w, r, c)
		if !ok {
			return
		}
		result, err := cloud.Enroll(r.Context(), project)
		if err != nil {
			writeCloudError(w, err, c.Config.ExposeDetails)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}

func handleCloudUnenroll(c *Container, cloud *services.CloudControlService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		project, ok := parseMutationBody(w, r, c)
		if !ok {
			return
		}
		// Guard: unenroll requires RWDB.
		if c.RWDB == nil {
			writeError(w, http.StatusServiceUnavailable, "RW_UNAVAILABLE",
				"The read-write database is not available.", nil, c.Config.ExposeDetails)
			return
		}
		result, err := cloud.Unenroll(r.Context(), project)
		if err != nil {
			writeCloudError(w, err, c.Config.ExposeDetails)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}

func handleCloudSync(c *Container, cloud *services.CloudControlService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body syncBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_INPUT", "expected JSON body", nil, c.Config.ExposeDetails)
			return
		}
		if strings.TrimSpace(body.Project) == "" {
			writeError(w, http.StatusBadRequest, "BAD_INPUT", "project is required", nil, c.Config.ExposeDetails)
			return
		}
		result, err := cloud.Sync(r.Context(), body.Project)
		if err != nil {
			writeCloudError(w, err, c.Config.ExposeDetails)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}

// syncAllProjectResult is one project entry in the sync-all response.
type syncAllProjectResult struct {
	Project string         `json:"project"`
	OK      bool           `json:"ok"`
	Output  string         `json:"output,omitempty"`
	Error   *syncAllError  `json:"error,omitempty"`
}

type syncAllError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Stderr  string `json:"stderr,omitempty"`
}

func handleCloudSyncAll(c *Container, cloud *services.CloudControlService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read the enrolled project list via the read-only path.
		syncData, err := services.SyncListProjects(c.RoDB)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "database error", err.Error(), c.Config.ExposeDetails)
			return
		}
		var enrolled []string
		for _, p := range syncData.Projects {
			if p.Enrolled {
				enrolled = append(enrolled, p.Project)
			}
		}

		// Bounded concurrency with errgroup + semaphore (N=5), preserving order.
		const maxConcurrency = 5
		results := make([]syncAllProjectResult, len(enrolled))

		// Use a semaphore channel to bound concurrency.
		sem := make(chan struct{}, maxConcurrency)
		eg, gctx := errgroup.WithContext(r.Context())

		// Each goroutine writes only its own results[i]; no shared state, no mutex needed.
		for i, proj := range enrolled {
			i, proj := i, proj // capture loop vars
			eg.Go(func() error {
				sem <- struct{}{}
				defer func() { <-sem }()

				res, syncErr := cloud.Sync(gctx, proj)
				if syncErr != nil {
					var ce *services.CloudControlError
					var errEntry syncAllError
					if casted, ok := syncErr.(*services.CloudControlError); ok {
						ce = casted
						errEntry = syncAllError{Code: ce.Code, Message: ce.Message}
						if c.Config.ExposeDetails && ce.Stderr != "" {
							errEntry.Stderr = ce.Stderr
						}
					} else {
						msg := "internal error"
						if c.Config.ExposeDetails && syncErr != nil {
							msg = syncErr.Error()
						}
						errEntry = syncAllError{Code: "INTERNAL", Message: msg}
					}
					results[i] = syncAllProjectResult{Project: proj, OK: false, Error: &errEntry}
				} else {
					results[i] = syncAllProjectResult{Project: proj, OK: true, Output: res.Output}
				}
				return nil // never fail the group — each project is independent
			})
		}
		// Wait; since goroutines never return an error, eg.Wait() is always nil.
		_ = eg.Wait()

		okCount := 0
		for _, res := range results {
			if res.OK {
				okCount++
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"total":   len(enrolled),
			"ok":      okCount,
			"failed":  len(enrolled) - okCount,
			"results": results,
		})
	})
}

func handleCloudCapabilities(c *Container, cloud *services.CloudControlService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caps, err := cloud.Capabilities(context.Background())
		if err != nil {
			writeCloudError(w, err, c.Config.ExposeDetails)
			return
		}
		writeJSON(w, http.StatusOK, caps)
	}
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// parseMutationBody decodes and validates the enroll/unenroll request body.
// Returns (project, true) on success; writes the error and returns ("", false) on failure.
func parseMutationBody(w http.ResponseWriter, r *http.Request, c *Container) (string, bool) {
	var body mutationBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_INPUT", "expected JSON body", nil, c.Config.ExposeDetails)
		return "", false
	}
	if body.Confirm == nil || !*body.Confirm {
		writeError(w, http.StatusBadRequest, "BAD_INPUT", "confirm must be exactly `true`", nil, c.Config.ExposeDetails)
		return "", false
	}
	if strings.TrimSpace(body.Project) == "" {
		writeError(w, http.StatusBadRequest, "BAD_INPUT", "project is required", nil, c.Config.ExposeDetails)
		return "", false
	}
	return body.Project, true
}

// writeCloudError maps a CloudControlError (or any other error) to an HTTP
// response matching the Node mapErrorToResponse shape.
func writeCloudError(w http.ResponseWriter, err error, exposeDetails bool) {
	if ce, ok := err.(*services.CloudControlError); ok {
		var status int
		switch ce.Code {
		case "BAD_INPUT":
			status = http.StatusBadRequest
		case "CLI_UNSUPPORTED":
			status = http.StatusNotImplemented
		default:
			status = http.StatusBadGateway
		}
		detail := map[string]any{"code": ce.Code}
		if exposeDetails && ce.Stderr != "" {
			detail["stderr"] = ce.Stderr
		}
		writeError(w, status, "CLI_ERROR", ce.Message, detail, exposeDetails)
		return
	}
	msg := "Internal server error"
	if exposeDetails && err != nil {
		msg = err.Error()
	}
	writeError(w, http.StatusInternalServerError, "INTERNAL", msg, nil, exposeDetails)
}
