// Package sqlite opens and configures database/sql pools backed by modernc.org/sqlite.
package sqlite

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite" // register the "sqlite" driver
)

// OpenReadOnly opens a read-only *sql.DB.
// It returns a descriptive error if the database file does not exist, mirroring
// the Node backend message so callers get actionable guidance.
func OpenReadOnly(path string) (*sql.DB, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf(
			"Engram database not found at %s. Set ENGRAM_DATA_DIR or install/run engram first",
			path,
		)
	}

	dsn := fmt.Sprintf(
		"file:%s?mode=ro&_pragma=query_only(true)&_pragma=busy_timeout(5000)",
		path,
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open read-only db: %w", err)
	}
	db.SetMaxOpenConns(4)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping read-only db: %w", err)
	}
	return db, nil
}

// OpenReadWrite opens a read-write *sql.DB with a single connection (serialised
// writes, matching Node's better-sqlite3 synchronous behaviour).
func OpenReadWrite(path string) (*sql.DB, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf(
			"Engram database not found at %s. Set ENGRAM_DATA_DIR or install/run engram first",
			path,
		)
	}

	dsn := fmt.Sprintf(
		"file:%s?_txlock=immediate&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)",
		path,
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open read-write db: %w", err)
	}
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping read-write db: %w", err)
	}
	return db, nil
}
