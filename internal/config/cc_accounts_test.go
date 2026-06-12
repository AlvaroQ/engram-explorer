package config_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
)

// ---------------------------------------------------------------------------
// SeedDefaultAccount
// ---------------------------------------------------------------------------

func TestSeedDefaultAccount_Empty(t *testing.T) {
	store := &config.ProfileStore{}
	config.SeedDefaultAccount(store, "/some/path")
	if len(store.CCAccounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(store.CCAccounts))
	}
	acc := store.CCAccounts[0]
	if acc.ID != "default" {
		t.Errorf("id=%q want %q", acc.ID, "default")
	}
	if acc.Label != "Personal" {
		t.Errorf("label=%q want %q", acc.Label, "Personal")
	}
	if acc.Path != "/some/path" {
		t.Errorf("path=%q want %q", acc.Path, "/some/path")
	}
	if !acc.Enabled {
		t.Error("expected Enabled=true")
	}
}

func TestSeedDefaultAccount_NoopIfPopulated(t *testing.T) {
	store := &config.ProfileStore{
		CCAccounts: []config.CCAccount{
			{ID: "existing", Label: "X", Path: "/x", Enabled: true},
		},
	}
	config.SeedDefaultAccount(store, "/other")
	if len(store.CCAccounts) != 1 {
		t.Fatalf("expected still 1 account, got %d", len(store.CCAccounts))
	}
	if store.CCAccounts[0].ID != "existing" {
		t.Error("SeedDefaultAccount should not overwrite an existing account")
	}
}

// ---------------------------------------------------------------------------
// ListEnabledCCAccounts
// ---------------------------------------------------------------------------

func TestListEnabledCCAccounts_FiltersDisabled(t *testing.T) {
	store := &config.ProfileStore{
		CCAccounts: []config.CCAccount{
			{ID: "a", Enabled: true},
			{ID: "b", Enabled: false},
			{ID: "c", Enabled: true},
		},
	}
	got := config.ListEnabledCCAccounts(store)
	if len(got) != 2 {
		t.Fatalf("want 2, got %d", len(got))
	}
	if got[0].ID != "a" || got[1].ID != "c" {
		t.Errorf("wrong IDs or order: %v", got)
	}
}

func TestListEnabledCCAccounts_Empty(t *testing.T) {
	store := &config.ProfileStore{}
	got := config.ListEnabledCCAccounts(store)
	if len(got) != 0 {
		t.Errorf("want 0, got %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// FindCCAccount
// ---------------------------------------------------------------------------

func TestFindCCAccount_Found(t *testing.T) {
	store := &config.ProfileStore{
		CCAccounts: []config.CCAccount{
			{ID: "foo", Label: "Foo"},
			{ID: "bar", Label: "Bar"},
		},
	}
	acc, ok := config.FindCCAccount(store, "bar")
	if !ok {
		t.Fatal("expected account to be found")
	}
	if acc.Label != "Bar" {
		t.Errorf("expected label Bar, got %q", acc.Label)
	}
}

func TestFindCCAccount_NotFound(t *testing.T) {
	store := &config.ProfileStore{
		CCAccounts: []config.CCAccount{
			{ID: "foo", Label: "Foo"},
		},
	}
	_, ok := config.FindCCAccount(store, "missing")
	if ok {
		t.Error("expected ok=false for unknown id")
	}
}

// ---------------------------------------------------------------------------
// AddCCAccount
// ---------------------------------------------------------------------------

func TestAddCCAccount_Success(t *testing.T) {
	dir := t.TempDir()
	store := &config.ProfileStore{Version: 1, ActiveProfile: "default"}

	acc, err := config.AddCCAccount(dir, store, "Work", "/home/user/.claude-work/projects")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acc.Label != "Work" {
		t.Errorf("label=%q want Work", acc.Label)
	}
	if acc.ID == "" {
		t.Error("ID should not be empty")
	}
	if !acc.Enabled {
		t.Error("new account should be enabled")
	}
	if len(store.CCAccounts) != 1 {
		t.Fatalf("expected 1 account in store, got %d", len(store.CCAccounts))
	}

	// Verify persisted.
	loaded, err := config.LoadProfileStore(dir)
	if err != nil {
		t.Fatalf("load after add: %v", err)
	}
	if len(loaded.CCAccounts) != 1 {
		t.Fatalf("expected 1 persisted account, got %d", len(loaded.CCAccounts))
	}
	if loaded.CCAccounts[0].ID != acc.ID {
		t.Errorf("persisted ID mismatch: got %q want %q", loaded.CCAccounts[0].ID, acc.ID)
	}
}

func TestAddCCAccount_UniqueID_WhenLabelCollides(t *testing.T) {
	dir := t.TempDir()
	store := &config.ProfileStore{Version: 1, ActiveProfile: "default"}

	acc1, err := config.AddCCAccount(dir, store, "Personal", "/a")
	if err != nil {
		t.Fatalf("add 1: %v", err)
	}
	acc2, err := config.AddCCAccount(dir, store, "Personal", "/b")
	if err != nil {
		t.Fatalf("add 2: %v", err)
	}
	if acc1.ID == acc2.ID {
		t.Errorf("IDs must be unique, both got %q", acc1.ID)
	}
}

func TestAddCCAccount_ValidationErrors(t *testing.T) {
	dir := t.TempDir()
	store := &config.ProfileStore{Version: 1, ActiveProfile: "default"}

	if _, err := config.AddCCAccount(dir, store, "", "/some/path"); err == nil {
		t.Error("expected error for empty label")
	}
	if _, err := config.AddCCAccount(dir, store, "Label", ""); err == nil {
		t.Error("expected error for empty path")
	}
}

// ---------------------------------------------------------------------------
// UpdateCCAccount
// ---------------------------------------------------------------------------

func TestUpdateCCAccount_Success(t *testing.T) {
	dir := t.TempDir()
	store := &config.ProfileStore{
		Version:       1,
		ActiveProfile: "default",
		CCAccounts: []config.CCAccount{
			{ID: "default", Label: "Personal", Path: "/old/path", Enabled: true},
		},
	}
	if err := config.SaveProfileStore(dir, store); err != nil {
		t.Fatalf("setup save: %v", err)
	}

	if err := config.UpdateCCAccount(dir, store, "default", "Home", "/new/path", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	acc, _ := config.FindCCAccount(store, "default")
	if acc.Label != "Home" || acc.Path != "/new/path" || acc.Enabled {
		t.Errorf("in-memory state not updated: %+v", acc)
	}

	loaded, _ := config.LoadProfileStore(dir)
	la := loaded.CCAccounts[0]
	if la.Label != "Home" || la.Path != "/new/path" || la.Enabled {
		t.Errorf("persisted state not updated: %+v", la)
	}
}

func TestUpdateCCAccount_NotFound(t *testing.T) {
	dir := t.TempDir()
	store := &config.ProfileStore{Version: 1, ActiveProfile: "default"}
	if err := config.SaveProfileStore(dir, store); err != nil {
		t.Fatalf("setup: %v", err)
	}
	err := config.UpdateCCAccount(dir, store, "ghost", "X", "/x", true)
	if !errors.Is(err, config.ErrCCAccountNotFound) {
		t.Errorf("expected ErrCCAccountNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// RemoveCCAccount
// ---------------------------------------------------------------------------

func TestRemoveCCAccount_Success(t *testing.T) {
	dir := t.TempDir()
	store := &config.ProfileStore{
		Version:       1,
		ActiveProfile: "default",
		CCAccounts: []config.CCAccount{
			{ID: "a", Label: "A", Path: "/a", Enabled: true},
			{ID: "b", Label: "B", Path: "/b", Enabled: true},
		},
	}
	if err := config.SaveProfileStore(dir, store); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := config.RemoveCCAccount(dir, store, "a"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.CCAccounts) != 1 {
		t.Fatalf("expected 1 remaining account, got %d", len(store.CCAccounts))
	}
	if store.CCAccounts[0].ID != "b" {
		t.Errorf("wrong account remaining: %q", store.CCAccounts[0].ID)
	}

	loaded, _ := config.LoadProfileStore(dir)
	if len(loaded.CCAccounts) != 1 || loaded.CCAccounts[0].ID != "b" {
		t.Errorf("persisted state wrong: %+v", loaded.CCAccounts)
	}
}

func TestRemoveCCAccount_RefusesLast(t *testing.T) {
	dir := t.TempDir()
	store := &config.ProfileStore{
		Version:       1,
		ActiveProfile: "default",
		CCAccounts: []config.CCAccount{
			{ID: "only", Label: "Only", Path: "/x", Enabled: true},
		},
	}
	if err := config.SaveProfileStore(dir, store); err != nil {
		t.Fatalf("setup: %v", err)
	}
	err := config.RemoveCCAccount(dir, store, "only")
	if err == nil {
		t.Error("expected error when removing the last account")
	}
}

func TestRemoveCCAccount_NotFound(t *testing.T) {
	dir := t.TempDir()
	store := &config.ProfileStore{
		Version:       1,
		ActiveProfile: "default",
		CCAccounts: []config.CCAccount{
			{ID: "a", Label: "A", Path: "/a", Enabled: true},
			{ID: "b", Label: "B", Path: "/b", Enabled: true},
		},
	}
	if err := config.SaveProfileStore(dir, store); err != nil {
		t.Fatalf("setup: %v", err)
	}
	err := config.RemoveCCAccount(dir, store, "ghost")
	if !errors.Is(err, config.ErrCCAccountNotFound) {
		t.Errorf("expected ErrCCAccountNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// EnsureConfig seeds a default CCAccount on new installs
// ---------------------------------------------------------------------------

func TestEnsureConfig_SeedsDefaultCCAccount(t *testing.T) {
	dir := t.TempDir()
	claudeProjects := filepath.Join(dir, "claude", "projects")

	cfg := config.Config{
		ClaudeProjectsDir: claudeProjects,
	}

	store, err := config.EnsureConfig(dir, cfg)
	if err != nil {
		t.Fatalf("EnsureConfig error: %v", err)
	}
	if len(store.CCAccounts) != 1 {
		t.Fatalf("expected 1 seeded CCAccount, got %d", len(store.CCAccounts))
	}
	if store.CCAccounts[0].Path != claudeProjects {
		t.Errorf("seeded path=%q want %q", store.CCAccounts[0].Path, claudeProjects)
	}

	// Calling EnsureConfig again (file exists) must not overwrite.
	store2, err := config.EnsureConfig(dir, cfg)
	if err != nil {
		t.Fatalf("second EnsureConfig error: %v", err)
	}
	if len(store2.CCAccounts) != 1 {
		t.Errorf("second call should preserve existing CCAccounts, got %d", len(store2.CCAccounts))
	}
}
