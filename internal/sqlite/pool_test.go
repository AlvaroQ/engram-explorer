package sqlite_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
)

// createTempDB creates a minimal SQLite database in a temp dir and returns the path.
func createTempDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	// Use the read-write DSN to seed the file.
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("seed open: %v", err)
	}
	_, err = db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY)")
	if err != nil {
		db.Close()
		t.Fatalf("seed create: %v", err)
	}
	db.Close()
	return path
}

func TestOpenReadOnly_MissingFile_ErrorMessage(t *testing.T) {
	_, err := sqlite.OpenReadOnly("/nonexistent/path/engram.db")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "Engram database not found at") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestOpenReadOnly_RejectsWrites(t *testing.T) {
	path := createTempDB(t)
	db, err := sqlite.OpenReadOnly(path)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer db.Close()

	_, err = db.Exec("INSERT INTO t VALUES (1)")
	if err == nil {
		t.Error("expected write to fail on read-only connection")
	}
}

func TestOpenReadOnly_AllowsReads(t *testing.T) {
	path := createTempDB(t)
	db, err := sqlite.OpenReadOnly(path)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer db.Close()

	var n int
	if err := db.QueryRow("SELECT 1").Scan(&n); err != nil {
		t.Errorf("read failed: %v", err)
	}
	if n != 1 {
		t.Errorf("got %d, want 1", n)
	}
}

func TestOpenReadWrite_MissingFile_ErrorMessage(t *testing.T) {
	_, err := sqlite.OpenReadWrite("/nonexistent/path/engram.db")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "Engram database not found at") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestOpenReadWrite_AllowsReadsAndWrites(t *testing.T) {
	path := createTempDB(t)
	db, err := sqlite.OpenReadWrite(path)
	if err != nil {
		t.Fatalf("OpenReadWrite: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec("INSERT INTO t VALUES (42)"); err != nil {
		t.Errorf("write failed: %v", err)
	}
	var id int
	if err := db.QueryRow("SELECT id FROM t WHERE id=42").Scan(&id); err != nil {
		t.Errorf("read back failed: %v", err)
	}
}

// Ensure the driver import side-effect is registered (compile guard).
func TestDriverRegistered(t *testing.T) {
	dir := t.TempDir()
	f, err := os.CreateTemp(dir, "*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	db, err := sql.Open("sqlite", "file:"+f.Name())
	if err != nil {
		t.Fatalf("sqlite driver not registered: %v", err)
	}
	db.Close()
}
