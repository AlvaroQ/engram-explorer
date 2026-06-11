// export_test.go exposes package-internal symbols for whitebox testing.
// This file is only compiled when running tests (package engram_test).
package engram

// CheckRODSN exposes the internal checkRODSN DSN builder so tests can verify
// that Detect and Validate use the correct pragmas (busy_timeout) to be
// resilient against WAL checkpoint pressure from the Engram daemon.
var CheckRODSN = checkRODSN
