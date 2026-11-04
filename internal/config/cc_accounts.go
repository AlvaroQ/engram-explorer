package config

import (
	"regexp"
	"strings"

	"github.com/AlvaroQ/engram-explorer/internal/usererr"
)

// CCAccount represents one Claude Code installation that the explorer tracks.
// Multiple accounts allow users who maintain several ~/.claude directories
// (personal, work, etc.) to view all sessions in a single UI.
type CCAccount struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Path    string `json:"path"`
	Enabled bool   `json:"enabled"`
}

// ErrCCAccountNotFound is returned when an account lookup by ID finds no match.
// It is a user-safe sentinel: compare it with errors.Is.
var ErrCCAccountNotFound = usererr.New("CC account not found")

var reNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// slugID converts a human label into a compact, filesystem-safe identifier.
// It lowercases the string, replaces runs of non-alphanumeric characters with
// a single dash, and strips leading/trailing dashes.
func slugID(label string) string {
	s := strings.ToLower(label)
	s = reNonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "account"
	}
	return s
}

// SeedDefaultAccount appends a single "Personal" account to store when
// CCAccounts is empty. It is a no-op when the slice already has entries.
// The caller is responsible for saving the store after calling this function.
func SeedDefaultAccount(store *ProfileStore, defaultPath string) {
	if len(store.CCAccounts) > 0 {
		return
	}
	store.CCAccounts = []CCAccount{
		{
			ID:      "default",
			Label:   "Personal",
			Path:    defaultPath,
			Enabled: true,
		},
	}
}

// ListEnabledCCAccounts returns the accounts from store that have Enabled=true,
// preserving the original order.
func ListEnabledCCAccounts(store *ProfileStore) []CCAccount {
	var out []CCAccount
	for _, a := range store.CCAccounts {
		if a.Enabled {
			out = append(out, a)
		}
	}
	return out
}

// FindCCAccount looks up an account by ID. It returns the account pointer and
// true on success, or nil and false when no match is found.
func FindCCAccount(store *ProfileStore, id string) (*CCAccount, bool) {
	for i := range store.CCAccounts {
		if store.CCAccounts[i].ID == id {
			return &store.CCAccounts[i], true
		}
	}
	return nil, false
}

// AddCCAccount creates a new CCAccount, assigns a unique slug-based ID, appends
// it to store, and persists the change via SaveProfileStore. On a save error
// store is left unchanged. It is not safe for concurrent use; servers go
// through Store.AddCCAccount instead.
// It returns an error when label or path is empty, or when the save fails.
func AddCCAccount(configHome string, store *ProfileStore, label, path string) (*CCAccount, error) {
	next := cloneProfileStore(*store)
	if _, err := addCCAccount(&next, label, path); err != nil {
		return nil, err
	}
	if err := SaveProfileStore(configHome, &next); err != nil {
		return nil, err
	}
	*store = next
	return &store.CCAccounts[len(store.CCAccounts)-1], nil
}

// UpdateCCAccount finds the account with the given id, updates its fields, and
// persists via SaveProfileStore. Returns ErrCCAccountNotFound when the id is
// not present. Not safe for concurrent use (see Store.UpdateCCAccount).
func UpdateCCAccount(configHome string, store *ProfileStore, id, label, path string, enabled bool) error {
	next := cloneProfileStore(*store)
	if err := updateCCAccount(&next, id, label, path, enabled); err != nil {
		return err
	}
	if err := SaveProfileStore(configHome, &next); err != nil {
		return err
	}
	*store = next
	return nil
}

// RemoveCCAccount removes the account with the given id from store and persists
// via SaveProfileStore. It refuses to remove the last remaining account and
// returns ErrCCAccountNotFound when the id is absent. Not safe for concurrent
// use (see Store.RemoveCCAccount).
func RemoveCCAccount(configHome string, store *ProfileStore, id string) error {
	next := cloneProfileStore(*store)
	if err := removeCCAccount(&next, id); err != nil {
		return err
	}
	if err := SaveProfileStore(configHome, &next); err != nil {
		return err
	}
	*store = next
	return nil
}

// addCCAccount validates and appends a new account to st (no I/O).
func addCCAccount(st *ProfileStore, label, path string) (CCAccount, error) {
	if strings.TrimSpace(label) == "" {
		return CCAccount{}, usererr.New("label must not be empty")
	}
	if strings.TrimSpace(path) == "" {
		return CCAccount{}, usererr.New("path must not be empty")
	}

	// Ensure uniqueness with a numeric suffix: base, base-2, base-3, ...
	base := slugID(label)
	existing := make(map[string]struct{}, len(st.CCAccounts))
	for _, a := range st.CCAccounts {
		existing[a.ID] = struct{}{}
	}
	id := base
	for n := 2; ; n++ {
		if _, taken := existing[id]; !taken {
			break
		}
		id = base + "-" + itoa(n)
	}

	acc := CCAccount{ID: id, Label: label, Path: path, Enabled: true}
	st.CCAccounts = append(st.CCAccounts, acc)
	return acc, nil
}

// updateCCAccount changes the fields of the account with the given id (no I/O).
func updateCCAccount(st *ProfileStore, id, label, path string, enabled bool) error {
	acc, ok := FindCCAccount(st, id)
	if !ok {
		return ErrCCAccountNotFound
	}
	acc.Label = label
	acc.Path = path
	acc.Enabled = enabled
	return nil
}

// removeCCAccount deletes the account with the given id from st (no I/O).
func removeCCAccount(st *ProfileStore, id string) error {
	if len(st.CCAccounts) <= 1 {
		return usererr.New("cannot remove the last CC account")
	}
	for i, a := range st.CCAccounts {
		if a.ID == id {
			st.CCAccounts = append(st.CCAccounts[:i:i], st.CCAccounts[i+1:]...)
			return nil
		}
	}
	return ErrCCAccountNotFound
}

// itoa converts a non-negative integer to its decimal string representation
// without importing strconv (avoids a dependency cycle risk).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}
	// Reverse.
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
