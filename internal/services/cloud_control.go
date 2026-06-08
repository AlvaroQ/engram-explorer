package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// projectNameRE mirrors the Node PROJECT_NAME_RE whitelist.
// Alphanumerics, dashes, underscores, dots; max 80 chars.
var projectNameRE = regexp.MustCompile(`^[A-Za-z0-9_.\-]{1,80}$`)

// CloudAction is the cloud operation performed.
type CloudAction string

const (
	CloudActionEnroll   CloudAction = "enroll"
	CloudActionUnenroll CloudAction = "unenroll"
	CloudActionSync     CloudAction = "sync"
)

// CloudResult is returned on a successful cloud mutation.
type CloudResult struct {
	OK      bool        `json:"ok"`
	Project string      `json:"project"`
	Action  CloudAction `json:"action"`
	Output  string      `json:"output"`
}

// CloudControlError is returned by every cloud operation on failure.
type CloudControlError struct {
	Code    string // BAD_INPUT | CLI_NOT_FOUND | CLI_FAILED | CLI_UNSUPPORTED | CLI_TIMEOUT
	Message string
	Stdout  string
	Stderr  string
}

func (e *CloudControlError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// CloudCapabilities is returned by the capabilities probe.
type CloudCapabilities struct {
	Enroll   bool   `json:"enroll"`
	Unenroll bool   `json:"unenroll"`
	Raw      string `json:"raw"`
}

// CloudControlOptions configures the cloud control service.
type CloudControlOptions struct {
	// CliBinary overrides the default "engram" binary name on PATH.
	CliBinary string

	// AuditLogPath is the absolute path for the JSONL audit log.
	// Parent directory is created if it does not exist (best-effort).
	AuditLogPath string

	// EngramDataDir, when non-empty, causes the service to read
	// <EngramDataDir>/cloud.json and forward its "token" field to the
	// subprocess as ENGRAM_CLOUD_TOKEN (if not already set in the env).
	EngramDataDir string

	// RWDB is the read-write SQLite pool. Required for unenroll (which is
	// handled locally via SQL — the engram CLI has no `cloud unenroll`).
	RWDB *sql.DB
}

// CloudControlService exposes the four cloud operations.
type CloudControlService struct {
	opts CloudControlOptions
	cli  string // resolved binary name (default "engram")
}

// NewCloudControlService creates a CloudControlService from opts.
func NewCloudControlService(opts CloudControlOptions) *CloudControlService {
	cli := opts.CliBinary
	if cli == "" {
		cli = "engram"
	}
	return &CloudControlService{opts: opts, cli: cli}
}

// ---------------------------------------------------------------------------
// Public methods
// ---------------------------------------------------------------------------

// Enroll enrolls a project via the CLI.
func (s *CloudControlService) Enroll(ctx context.Context, project string) (CloudResult, error) {
	return s.run(ctx, CloudActionEnroll, project)
}

// Sync runs a cloud sync for a project via the CLI.
func (s *CloudControlService) Sync(ctx context.Context, project string) (CloudResult, error) {
	return s.run(ctx, CloudActionSync, project)
}

// Unenroll handles unenrollment LOCALLY via SQL (the CLI has no `cloud unenroll`
// as of v1.15.x). This is the deliberate "mutations only via CLI" exception.
// Guard: returns 503-equivalent error when RWDB is nil.
func (s *CloudControlService) Unenroll(ctx context.Context, project string) (CloudResult, error) {
	safe, err := s.validate(project)
	if err != nil {
		return CloudResult{}, err
	}
	if s.opts.RWDB == nil {
		return CloudResult{}, &CloudControlError{
			Code:    "CLI_UNSUPPORTED",
			Message: "unenroll requires a writable database adapter; none configured",
		}
	}

	targetKey := "cloud:" + safe
	tx, err := s.opts.RWDB.BeginTx(ctx, nil)
	if err != nil {
		ce := &CloudControlError{Code: "CLI_FAILED", Message: "unenroll failed: " + err.Error()}
		_ = s.record(CloudActionUnenroll, safe, false, ce.Message)
		return CloudResult{}, ce
	}

	var removedEnrollment bool
	var removedStateRows, removedPendingMutations int64

	// Three DELETEs in one transaction, matching the TS transaction exactly.
	if res, execErr := tx.ExecContext(ctx,
		`DELETE FROM sync_enrolled_projects WHERE project = ?`, safe); execErr != nil {
		tx.Rollback()
		ce := &CloudControlError{Code: "CLI_FAILED", Message: "unenroll failed: " + execErr.Error()}
		_ = s.record(CloudActionUnenroll, safe, false, ce.Message)
		return CloudResult{}, ce
	} else if n, _ := res.RowsAffected(); n > 0 {
		removedEnrollment = true
	}

	if res, execErr := tx.ExecContext(ctx,
		`DELETE FROM sync_state WHERE target_key = ?`, targetKey); execErr != nil {
		tx.Rollback()
		ce := &CloudControlError{Code: "CLI_FAILED", Message: "unenroll failed: " + execErr.Error()}
		_ = s.record(CloudActionUnenroll, safe, false, ce.Message)
		return CloudResult{}, ce
	} else {
		removedStateRows, _ = res.RowsAffected()
	}

	if res, execErr := tx.ExecContext(ctx,
		`DELETE FROM sync_mutations WHERE project = ? AND acked_at IS NULL`, safe); execErr != nil {
		tx.Rollback()
		ce := &CloudControlError{Code: "CLI_FAILED", Message: "unenroll failed: " + execErr.Error()}
		_ = s.record(CloudActionUnenroll, safe, false, ce.Message)
		return CloudResult{}, ce
	} else {
		removedPendingMutations, _ = res.RowsAffected()
	}

	if err := tx.Commit(); err != nil {
		ce := &CloudControlError{Code: "CLI_FAILED", Message: "unenroll failed: " + err.Error()}
		_ = s.record(CloudActionUnenroll, safe, false, ce.Message)
		return CloudResult{}, ce
	}

	var output string
	if removedEnrollment {
		output = fmt.Sprintf(
			"removed %s from sync_enrolled_projects (cleared %d sync_state row(s) and %d pending mutation(s))",
			safe, removedStateRows, removedPendingMutations,
		)
	} else {
		output = fmt.Sprintf(
			"%s was not enrolled (cleared %d stale sync_state row(s) and %d pending mutation(s))",
			safe, removedStateRows, removedPendingMutations,
		)
	}

	_ = s.record(CloudActionUnenroll, safe, true, output)
	return CloudResult{OK: true, Project: safe, Action: CloudActionUnenroll, Output: output}, nil
}

// Capabilities probes the CLI to determine which cloud subcommands are available.
// Two sequential probes: `cloud --help` then `cloud __probe_unknown`.
// ENOENT (binary not found) → enroll=false, unenroll=(RWDB!=nil), raw="".
func (s *CloudControlService) Capabilities(ctx context.Context) (CloudCapabilities, error) {
	probes := [][]string{
		{"cloud", "--help"},
		{"cloud", "__probe_unknown"},
	}
	raw := ""
	for _, args := range probes {
		stdout, stderr, err := s.execProbe(ctx, args)
		combined := strings.TrimSpace(stdout + "\n" + stderr)
		if err != nil {
			if isENOENT(err) {
				return CloudCapabilities{
					Enroll:   false,
					Unenroll: s.opts.RWDB != nil,
					Raw:      "",
				}, nil
			}
			// Non-zero exit is normal for `cloud --help`; collect output.
			if combined != "" {
				raw = combined
				break
			}
			continue
		}
		if combined != "" {
			raw = combined
			break
		}
	}

	subcommandsMatch := regexp.MustCompile(`(?i)supported subcommands:\s*([^\n]+)`).FindStringSubmatch(raw)
	sub := ""
	if len(subcommandsMatch) > 1 {
		sub = subcommandsMatch[1]
	}
	subSet := make(map[string]struct{})
	for _, part := range regexp.MustCompile(`[,\s]+`).Split(sub, -1) {
		part = strings.ToLower(strings.TrimSpace(part))
		if part != "" {
			subSet[part] = struct{}{}
		}
	}

	_, hasEnroll := subSet["enroll"]
	enroll := hasEnroll || regexp.MustCompile(`(?i)\benroll\b`).MatchString(raw)

	// unenroll is implemented locally (direct DB delete) when RWDB is
	// available, since the CLI does not expose it as of v1.15.x.
	_, hasUnenroll := subSet["unenroll"]
	var unenroll bool
	if s.opts.RWDB != nil {
		unenroll = true
	} else {
		unenroll = hasUnenroll || regexp.MustCompile(`(?i)\bunenroll\b`).MatchString(raw)
	}

	return CloudCapabilities{Enroll: enroll, Unenroll: unenroll, Raw: raw}, nil
}

// ---------------------------------------------------------------------------
// Private helpers
// ---------------------------------------------------------------------------

func (s *CloudControlService) validate(project string) (string, error) {
	if !projectNameRE.MatchString(project) {
		return "", &CloudControlError{
			Code:    "BAD_INPUT",
			Message: "Invalid project name. Allowed: " + projectNameRE.String(),
		}
	}
	return project, nil
}

// run executes enroll or sync via CLI subprocess.
func (s *CloudControlService) run(ctx context.Context, action CloudAction, project string) (CloudResult, error) {
	safe, err := s.validate(project)
	if err != nil {
		return CloudResult{}, err
	}

	var args []string
	var timeoutMs int
	if action == CloudActionSync {
		args = []string{"sync", "--cloud", "--project", safe}
		timeoutMs = 60_000
	} else {
		args = []string{"cloud", string(action), safe}
		timeoutMs = 10_000
	}

	procEnv, err := s.buildSubprocessEnv()
	if err != nil {
		// non-fatal: just use whatever environment we have
		procEnv = os.Environ()
	}

	timeout := time.Duration(timeoutMs) * time.Millisecond
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	stdout, stderr, execErr := s.execCLI(tctx, args, procEnv)
	if execErr != nil {
		ce := mapCLIError(execErr, string(action), stdout, stderr)
		_ = s.record(action, safe, false, ce.Message+"\n"+ce.Stderr)
		return CloudResult{}, ce
	}

	output := strings.TrimSpace(stdout + "\n" + stderr)
	_ = s.record(action, safe, true, output)
	return CloudResult{OK: true, Project: safe, Action: action, Output: output}, nil
}

// buildSubprocessEnv returns a copy of os.Environ() with ENGRAM_CLOUD_TOKEN
// injected from cloud.json if the variable is not already set.
func (s *CloudControlService) buildSubprocessEnv() ([]string, error) {
	env := os.Environ()

	// If already set in the process environment, keep it as-is.
	for _, kv := range env {
		if strings.HasPrefix(kv, "ENGRAM_CLOUD_TOKEN=") {
			return env, nil
		}
	}

	// Try to read the token from cloud.json.
	if s.opts.EngramDataDir == "" {
		return env, nil
	}
	token, err := readTokenFromCloudJSON(filepath.Join(s.opts.EngramDataDir, "cloud.json"))
	if err != nil || token == "" {
		return env, nil
	}
	return append(env, "ENGRAM_CLOUD_TOKEN="+token), nil
}

// readTokenFromCloudJSON reads the "token" field from a cloud.json file.
// Returns "" when the file is missing, malformed, or has no usable token.
func readTokenFromCloudJSON(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil // non-fatal
	}
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", nil
	}
	rawToken, ok := parsed["token"]
	if !ok {
		return "", nil
	}
	var token string
	if err := json.Unmarshal(rawToken, &token); err != nil {
		return "", nil
	}
	return token, nil
}

// record appends a JSONL entry to the audit log (best-effort).
func (s *CloudControlService) record(action CloudAction, project string, ok bool, output string) error {
	if s.opts.AuditLogPath == "" {
		return nil
	}
	entry, err := json.Marshal(map[string]any{
		"at":      time.Now().UTC().Format(time.RFC3339),
		"action":  string(action),
		"project": project,
		"ok":      ok,
		"output":  truncateStr(output, 500),
	})
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.opts.AuditLogPath)
	// best-effort mkdir — swallow errors
	_ = os.MkdirAll(dir, 0o755)
	f, err := os.OpenFile(s.opts.AuditLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, _ = f.Write(append(entry, '\n'))
	return nil
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// ---------------------------------------------------------------------------
// Error classification — mirrors TS isCliUnsupportedSignal / mapError
// ---------------------------------------------------------------------------

// isCliUnsupportedSignal returns true when the haystack indicates that the
// requested cloud subcommand does not exist in the installed CLI.
func isCliUnsupportedSignal(haystack string) bool {
	return regexp.MustCompile(`(?i)unknown\s+(?:\w+\s+)?(?:sub)?command`).MatchString(haystack) ||
		regexp.MustCompile(`(?i)unrecognized`).MatchString(haystack) ||
		regexp.MustCompile(`(?i)not\s+(?:a\s+)?supported\s+(?:sub)?command`).MatchString(haystack) ||
		regexp.MustCompile(`(?i)supported\s+subcommands?:`).MatchString(haystack)
}

// mapCLIError converts a subprocess error to a CloudControlError.
func mapCLIError(err error, action, stdout, stderr string) *CloudControlError {
	if ce, ok := err.(*CloudControlError); ok {
		return ce
	}
	if isENOENT(err) {
		return &CloudControlError{
			Code:    "CLI_NOT_FOUND",
			Message: "`engram` CLI not found in PATH",
		}
	}
	if isKilled(err) {
		return &CloudControlError{
			Code:    "CLI_TIMEOUT",
			Message: "engram cloud " + action + " timed out",
			Stdout:  stdout,
			Stderr:  stderr,
		}
	}
	haystack := stderr + "\n" + err.Error()
	if isCliUnsupportedSignal(haystack) {
		return &CloudControlError{
			Code:    "CLI_UNSUPPORTED",
			Message: "engram cloud " + action + " is not supported by the installed CLI",
			Stdout:  stdout,
			Stderr:  stderr,
		}
	}
	return &CloudControlError{
		Code:    "CLI_FAILED",
		Message: "engram cloud " + action + " failed: " + err.Error(),
		Stdout:  stdout,
		Stderr:  stderr,
	}
}
