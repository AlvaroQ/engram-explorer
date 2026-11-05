package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/services"
	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
)

// Rate limits for write routes, per client and route pattern per minute.
const (
	importRateLimit = 5
	exportRateLimit = 10
	writeRateLimit  = 120
)

// writeRoutes registers all write (mutating) routes on mux.
// Every handler guards a writable store before proceeding, and is
// wrapped in a per-client, per-route fixed-window rate limiter.
func writeRoutes(mux routeMux, c *Container) {
	writeRL := newRateLimiter(time.Minute, writeRateLimit)
	exportRL := newRateLimiter(time.Minute, exportRateLimit)
	importRL := newRateLimiter(time.Minute, importRateLimit)
	limited := func(rl *rateLimiter, h http.Handler) http.Handler {
		return withRateLimit(rl, c.Config.ExposeDetails, h)
	}

	// Observation PATCH
	mux.Handle("PATCH /api/observations/{id}", limited(writeRL, handleObservationPatch(c)))

	// Assignment: PATCH /:entity/:id/project
	mux.Handle("PATCH /api/observations/{id}/project", limited(writeRL, handleAssignProject(c, services.EntityKindObservation)))
	mux.Handle("PATCH /api/sessions/{id}/project", limited(writeRL, handleAssignProject(c, services.EntityKindSession)))
	mux.Handle("PATCH /api/prompts/{id}/project", limited(writeRL, handleAssignProject(c, services.EntityKindPrompt)))

	// Deletion: DELETE /:entity/:id
	mux.Handle("DELETE /api/observations/{id}", limited(writeRL, handleDeleteEntity(c, services.EntityKindObservation)))
	mux.Handle("DELETE /api/sessions/{id}", limited(writeRL, handleDeleteEntity(c, services.EntityKindSession)))
	mux.Handle("DELETE /api/prompts/{id}", limited(writeRL, handleDeleteEntity(c, services.EntityKindPrompt)))

	// Project rename
	mux.Handle("POST /api/projects/{project}/rename", limited(writeRL, handleProjectRename(c)))

	// Database export / import
	mux.Handle("GET /api/db/export", limited(exportRL, handleDBExport(c)))
	mux.Handle("GET /api/projects/{project}/export", limited(exportRL, handleProjectExport(c)))
	mux.Handle("POST /api/db/import", limited(importRL, handleDBImport(c)))
}

// guardRWDB returns true and writes a 503 if the RW pool is unavailable: the
// live store does not implement providers.Writable or reports false.
func guardRWDB(w http.ResponseWriter, r *http.Request, c *Container) bool {
	if !c.writable(r) {
		writeError(w, http.StatusServiceUnavailable, "RW_UNAVAILABLE",
			"The read-write database is not available. The server started with a read-only connection.",
			nil, c.Config.ExposeDetails)
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// handleObservationPatch — PATCH /api/observations/{id}
// ---------------------------------------------------------------------------

func handleObservationPatch(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if guardRWDB(w, r, c) {
			return
		}

		idStr := r.PathValue("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "BAD_INPUT", "id must be an integer", nil, c.Config.ExposeDetails)
			return
		}

		var body map[string]json.RawMessage
		if !decodeJSONBody(w, r, &body, c.Config.ExposeDetails) {
			return
		}

		patch, err := parseObservationPatch(body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "BAD_INPUT", err.Error(), nil, c.Config.ExposeDetails)
			return
		}

		result, err := services.UpdateObservation(r.Context(), c.rw(r), id, patch)
		if err != nil {
			var we *services.WriteError
			if errors.As(err, &we) && we.Code == "NOT_FOUND" {
				writeError(w, http.StatusNotFound, "NOT_FOUND",
					"observation "+idStr+" not found", nil, c.Config.ExposeDetails)
				return
			}
			writeDBError(w, err, c.Config.ExposeDetails)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// parseObservationPatch decodes the raw JSON map into an ObservationPatch using
// KEY-PRESENCE semantics: a field is "provided" only if it appears in the JSON.
func parseObservationPatch(body map[string]json.RawMessage) (services.ObservationPatch, error) {
	var p services.ObservationPatch

	if raw, ok := body["type"]; ok {
		p.TypeProvided = true
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return p, errors.New("type must be a string")
		}
		p.Type = s
	}

	if raw, ok := body["title"]; ok {
		p.TitleProvided = true
		// Accept string or null.
		var s *string
		if string(raw) == "null" {
			s = nil
		} else {
			var sv string
			if err := json.Unmarshal(raw, &sv); err != nil {
				return p, errors.New("title must be a string or null")
			}
			s = &sv
		}
		p.Title = s
	}

	if raw, ok := body["topic_key"]; ok {
		p.TopicKeyProvided = true
		var s *string
		if string(raw) == "null" {
			s = nil
		} else {
			var sv string
			if err := json.Unmarshal(raw, &sv); err != nil {
				return p, errors.New("topic_key must be a string or null")
			}
			s = &sv
		}
		p.TopicKey = s
	}

	if raw, ok := body["content"]; ok {
		p.ContentProvided = true
		var s *string
		if string(raw) == "null" {
			s = nil
		} else {
			var sv string
			if err := json.Unmarshal(raw, &sv); err != nil {
				return p, errors.New("content must be a string or null")
			}
			s = &sv
		}
		p.Content = s
	}

	return p, nil
}

// ---------------------------------------------------------------------------
// handleAssignProject — PATCH /api/{entities}/{id}/project
// ---------------------------------------------------------------------------

func handleAssignProject(c *Container, entity services.EntityKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if guardRWDB(w, r, c) {
			return
		}

		idStr := r.PathValue("id")
		var id any
		if entity == services.EntityKindSession {
			if idStr == "" {
				writeError(w, http.StatusBadRequest, "BAD_INPUT", "id is required", nil, c.Config.ExposeDetails)
				return
			}
			id = idStr
		} else {
			n, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				writeError(w, http.StatusBadRequest, "BAD_INPUT", "id must be an integer", nil, c.Config.ExposeDetails)
				return
			}
			id = n
		}

		var body struct {
			Project string `json:"project"`
		}
		if !decodeJSONBody(w, r, &body, c.Config.ExposeDetails) {
			return
		}
		if strings.TrimSpace(body.Project) == "" {
			writeError(w, http.StatusBadRequest, "BAD_INPUT", "project is required", nil, c.Config.ExposeDetails)
			return
		}

		result, err := services.AssignProject(r.Context(), c.rw(r), entity, id, body.Project)
		if err != nil {
			var we *services.WriteError
			if errors.As(err, &we) && we.Code == "NOT_FOUND" {
				writeError(w, http.StatusNotFound, "NOT_FOUND",
					string(entity)+" "+idStr+" not found", nil, c.Config.ExposeDetails)
				return
			}
			writeDBError(w, err, c.Config.ExposeDetails)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// ---------------------------------------------------------------------------
// handleDeleteEntity — DELETE /api/{entities}/{id}
// ---------------------------------------------------------------------------

func handleDeleteEntity(c *Container, entity services.EntityKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if guardRWDB(w, r, c) {
			return
		}

		idStr := r.PathValue("id")
		var id any
		if entity == services.EntityKindSession {
			if idStr == "" {
				writeError(w, http.StatusBadRequest, "BAD_INPUT", "id is required", nil, c.Config.ExposeDetails)
				return
			}
			id = idStr
		} else {
			n, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				writeError(w, http.StatusBadRequest, "BAD_INPUT", "id must be an integer", nil, c.Config.ExposeDetails)
				return
			}
			id = n
		}

		result, err := services.DeleteEntity(r.Context(), c.rw(r), entity, id)
		if err != nil {
			var we *services.WriteError
			if errors.As(err, &we) {
				switch we.Code {
				case "NOT_FOUND":
					writeError(w, http.StatusNotFound, "NOT_FOUND",
						string(entity)+" "+idStr+" not found", nil, c.Config.ExposeDetails)
				case "ALREADY_DELETED":
					writeError(w, http.StatusConflict, "BAD_INPUT",
						string(entity)+" "+idStr+" is already deleted", nil, c.Config.ExposeDetails)
				case "HAS_PROMPTS":
					writeError(w, http.StatusConflict, "BAD_INPUT",
						"cannot delete session: it has user_prompts attached", nil, c.Config.ExposeDetails)
				case "HAS_OBSERVATIONS":
					writeError(w, http.StatusConflict, "BAD_INPUT",
						"cannot delete session: it has active observations", nil, c.Config.ExposeDetails)
				default:
					writeError(w, http.StatusInternalServerError, "INTERNAL", we.Message, nil, c.Config.ExposeDetails)
				}
				return
			}
			writeDBError(w, err, c.Config.ExposeDetails)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// ---------------------------------------------------------------------------
// handleProjectRename — POST /api/projects/{project}/rename
// ---------------------------------------------------------------------------

func handleProjectRename(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if guardRWDB(w, r, c) {
			return
		}

		source := r.PathValue("project")

		var body struct {
			Target string `json:"target"`
			Mode   string `json:"mode"`
		}
		if !decodeJSONBody(w, r, &body, c.Config.ExposeDetails) {
			return
		}
		if strings.TrimSpace(body.Target) == "" {
			writeError(w, http.StatusBadRequest, "BAD_INPUT", "target is required", nil, c.Config.ExposeDetails)
			return
		}
		if body.Mode != "rename" && body.Mode != "merge" {
			writeError(w, http.StatusBadRequest, "BAD_INPUT", `mode must be "rename" or "merge"`, nil, c.Config.ExposeDetails)
			return
		}

		result, err := services.RenameProject(r.Context(), c.rw(r), services.RenameProjectParams{
			Source: source,
			Target: body.Target,
			Mode:   body.Mode,
		})
		if err != nil {
			var we *services.WriteError
			if errors.As(err, &we) {
				switch we.Code {
				case "SAME_NAME":
					writeError(w, http.StatusBadRequest, "BAD_INPUT",
						"source and target project names are the same", nil, c.Config.ExposeDetails)
				case "SOURCE_NOT_FOUND":
					writeError(w, http.StatusNotFound, "NOT_FOUND",
						"project '"+source+"' not found", nil, c.Config.ExposeDetails)
				case "TARGET_EXISTS":
					writeError(w, http.StatusConflict, "BAD_INPUT",
						"project '"+body.Target+"' already exists; use mode='merge' to combine", nil, c.Config.ExposeDetails)
				case "TARGET_NOT_FOUND":
					writeError(w, http.StatusBadRequest, "BAD_INPUT",
						"merge target '"+body.Target+"' does not exist", nil, c.Config.ExposeDetails)
				case "ENROLLED_SOURCE_UNSUPPORTED":
					writeError(w, http.StatusConflict, "BAD_INPUT", we.Message, nil, c.Config.ExposeDetails)
				case "ENROLLED_TARGET_UNSUPPORTED":
					writeError(w, http.StatusConflict, "BAD_INPUT", we.Message, nil, c.Config.ExposeDetails)
				default:
					writeError(w, http.StatusInternalServerError, "INTERNAL", we.Message, nil, c.Config.ExposeDetails)
				}
				return
			}
			writeDBError(w, err, c.Config.ExposeDetails)
			return
		}

		// Response shape matches the Node route: flat affected counts.
		writeJSON(w, http.StatusOK, map[string]any{
			"observations": result.Affected.Observations,
			"sessions":     result.Affected.Sessions,
			"userPrompts":  result.Affected.UserPrompts,
		})
	}
}

// ---------------------------------------------------------------------------
// handleDBExport — GET /api/db/export
// ---------------------------------------------------------------------------

func handleDBExport(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if guardRWDB(w, r, c) {
			return
		}
		serveSnapshot(w, r, c, "engram-backup-"+exportStamp()+".db", services.ExportSnapshot)
	}
}

// exportStamp is the backup filename stamp, matching the Node route format:
// "YYYY-MM-DDTHH-MM-SS".
func exportStamp() string {
	return strings.NewReplacer(":", "-", " ", "T").Replace(services.NowSqlite())
}

// serveSnapshot streams the snapshot built by build as an attachment named
// filename. HEAD (routed here by the GET pattern) only describes the
// download: building a full snapshot just to discard it would let HEAD
// requests run VACUUM INTO for free.
func serveSnapshot(w http.ResponseWriter, r *http.Request, c *Container, filename string,
	build func(ctx context.Context, db sqlite.Querier) (string, func(), error)) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}

	tmpPath, cleanup, err := build(r.Context(), c.rw(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "export failed", err.Error(), c.Config.ExposeDetails)
		return
	}
	defer cleanup()

	f, err := os.Open(tmpPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "could not read export file", err.Error(), c.Config.ExposeDetails)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "could not stat export file", err.Error(), c.Config.ExposeDetails)
		return
	}

	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f)
}

// ---------------------------------------------------------------------------
// handleProjectExport — GET /api/projects/{project}/export
// ---------------------------------------------------------------------------

// handleProjectExport downloads a backup of one project: a full-schema
// snapshot holding only that project's rows (services.ExportProjectSnapshot),
// which POST /api/db/import merges back. It answers like GET /api/db/export
// (same-origin only, 503 in read-only mode, HEAD without a snapshot), plus 404
// for an unknown project.
func handleProjectExport(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Guard already refuses cross-site exports; kept as defense in depth for
		// callers that mount the handler without Guard (tests, embedding).
		if !sameOriginRequest(r, trustedDevOrigins(c.Config.Env)) {
			logger := c.Logger
			if logger == nil {
				logger = slog.Default()
			}
			logger.Warn("blocked cross-origin request", "method", r.Method, "path", r.URL.Path,
				"origin", r.Header.Get("Origin"), "sec_fetch_site", r.Header.Get("Sec-Fetch-Site"))
			writeError(w, http.StatusForbidden, "CROSS_ORIGIN_REQUEST", "cross-origin request blocked", nil, false)
			return
		}
		if guardRWDB(w, r, c) {
			return
		}
		project := r.PathValue("project")
		exists, err := services.ProjectExistsDB(c.rw(r), project)
		if err != nil {
			writeDBError(w, err, c.Config.ExposeDetails)
			return
		}
		if !exists {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "project not found", nil, c.Config.ExposeDetails)
			return
		}
		serveSnapshot(w, r, c, "engram-backup-"+backupNamePart(project)+"-"+exportStamp()+".db",
			func(ctx context.Context, db sqlite.Querier) (string, func(), error) {
				return services.ExportProjectSnapshot(ctx, db, project)
			})
	}
}

// backupNamePart turns a project name into a filename-safe fragment: runs of
// characters other than ASCII letters, digits, '.', '_' and '-' become one
// '-', leading/trailing separators are trimmed, and the result is capped at
// 64 bytes ("project" when nothing is left).
func backupNamePart(project string) string {
	var b strings.Builder
	dash := false
	for _, r := range project {
		ok := r < 128 && (r == '.' || r == '_' || r == '-' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))
		if ok {
			b.WriteRune(r)
			dash = false
		} else if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-.")
	if len(out) > 64 {
		out = strings.Trim(out[:64], "-.")
	}
	if out == "" {
		return "project"
	}
	return out
}

// ---------------------------------------------------------------------------
// handleDBImport — POST /api/db/import
// ---------------------------------------------------------------------------

// maxUploadSize caps the multipart upload at 512 MB.
const maxUploadSize = 512 << 20

func handleDBImport(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if guardRWDB(w, r, c) {
			return
		}

		// Cap body size to prevent memory exhaustion.
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_INPUT",
				"could not parse multipart form (file too large or malformed)", nil, c.Config.ExposeDetails)
			return
		}
		defer r.MultipartForm.RemoveAll() //nolint:errcheck

		f, fh, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "BAD_INPUT",
				`Expected a database file in the "file" field.`, nil, c.Config.ExposeDetails)
			return
		}
		defer f.Close()
		_ = fh // filename available but not needed

		// Stream to temp file.
		tmpFile, err := os.CreateTemp("", "engram-import-*.db")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL",
				"could not create temp file", err.Error(), c.Config.ExposeDetails)
			return
		}
		tmpPath := tmpFile.Name()

		if _, err := io.Copy(tmpFile, f); err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			writeError(w, http.StatusInternalServerError, "INTERNAL",
				"could not write upload to disk", err.Error(), c.Config.ExposeDetails)
			return
		}
		// Close before passing path to ImportMerge (Windows file-locking).
		if err := tmpFile.Close(); err != nil {
			os.Remove(tmpPath)
			writeError(w, http.StatusInternalServerError, "INTERNAL",
				"could not finalise upload file", err.Error(), c.Config.ExposeDetails)
			return
		}
		defer os.Remove(tmpPath)

		// Determine data dir for safety backup (same dir as the DB file).
		dataDir := c.Paths.DataDir()
		if dataDir == "" {
			writeError(w, http.StatusInternalServerError, "INTERNAL",
				"data directory not configured", nil, c.Config.ExposeDetails)
			return
		}

		result, err := services.ImportMerge(r.Context(), c.rw(r), tmpPath, dataDir)
		if err != nil {
			var die *services.DatabaseImportError
			if errors.As(err, &die) {
				switch die.Code {
				case services.DatabaseImportCodeBusy:
					writeError(w, http.StatusServiceUnavailable, "INTERNAL", die.Message, nil, c.Config.ExposeDetails)
				default:
					// BAD_SQLITE | INTEGRITY | SCHEMA_MISMATCH — user-facing message
					writeError(w, http.StatusBadRequest, "BAD_INPUT", die.Message, nil, c.Config.ExposeDetails)
				}
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL",
				"import failed", err.Error(), c.Config.ExposeDetails)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}
