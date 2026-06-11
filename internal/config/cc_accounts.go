package config

import (
	"errors"
	"regexp"
	"strings"
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
var ErrCCAccountNotFound = errors.New("CC account not found")

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
// it to store, and persists the change via SaveProfileStore.
// It returns an error when label or path is empty, or when the save fails.
func AddCCAccount(configHome string, store *ProfileStore, label, path string) (*CCAccount, error) {
	if strings.TrimSpace(label) == "" {
		return nil, errors.New("label must not be empty")
	}
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("path must not be empty")
	}

	base := slugID(label)
	id := base
	// Ensure uniqueness by checking against existing IDs.
	existing := make(map[string]struct{}, len(store.CCAccounts))
	for _, a := range store.CCAccounts {
		existing[a.ID] = struct{}{}
	}
	for n := 2; ; n++ {
		if _, taken := existing[id]; !taken {
			break
		}
		id = base + "-" + strings.TrimLeft(strings.Repeat("0", 0)+string(rune('0'+n%10)), "")
		// Use a simple numeric suffix: base-2, base-3, ...
		id = base + "-" + itoa(n)
	}

	acc := CCAccount{
		ID:      id,
		Label:   label,
		Path:    path,
		Enabled: true,
	}
	store.CCAccounts = append(store.CCAccounts, acc)

	if err := SaveProfileStore(configHome, store); err != nil {
		// Roll back the in-memory append.
		store.CCAccounts = store.CCAccounts[:len(store.CCAccounts)-1]
		return nil, err
	}
	return &store.CCAccounts[len(store.CCAccounts)-1], nil
}

// UpdateCCAccount finds the account with the given id, updates its fields, and
// persists via SaveProfileStore. Returns ErrCCAccountNotFound when the id is
// not present.
func UpdateCCAccount(configHome string, store *ProfileStore, id, label, path string, enabled bool) error {
	acc, ok := FindCCAccount(store, id)
	if !ok {
		return ErrCCAccountNotFound
	}
	old := *acc
	acc.Label = label
	acc.Path = path
	acc.Enabled = enabled
	if err := SaveProfileStore(configHome, store); err != nil {
		// Roll back.
		*acc = old
		return err
	}
	return nil
}

// RemoveCCAccount removes the account with the given id from store and persists
// via SaveProfileStore. It refuses to remove the last remaining account and
// returns ErrCCAccountNotFound when the id is absent.
func RemoveCCAccount(configHome string, store *ProfileStore, id string) error {
	if len(store.CCAccounts) <= 1 {
		return errors.New("cannot remove the last CC account")
	}

	idx := -1
	for i, a := range store.CCAccounts {
		if a.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrCCAccountNotFound
	}

	original := store.CCAccounts
	store.CCAccounts = append(store.CCAccounts[:idx:idx], store.CCAccounts[idx+1:]...)
	if err := SaveProfileStore(configHome, store); err != nil {
		store.CCAccounts = original
		return err
	}
	return nil
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
