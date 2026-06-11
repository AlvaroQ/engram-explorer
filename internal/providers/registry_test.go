package providers_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/providers/providertest"
)

// makeProfile is a convenience that builds a FakeProfile enabling all given
// provider IDs with a non-empty path so Detect can succeed.
func makeProfile(ids ...string) providers.Profile {
	cfgs := make(map[string]providers.ProviderConfig, len(ids))
	for _, id := range ids {
		cfgs[id] = providers.ProviderConfig{Enabled: true, Path: "/fake/" + id}
	}
	return providertest.NewFakeProfile(cfgs)
}

// TestBoot_NeverFatal asserts that Boot continues when a provider's Detect
// returns unavailable; no panic, no fatal error.
func TestBoot_NeverFatal(t *testing.T) {
	missing := providertest.NewFakeProvider(providertest.FakeProviderOptions{
		ID:              "missing",
		DetectAvailable: false,
	})
	reg := providers.NewRegistry(slog.Default())
	reg.Register(missing)
	reg.Boot(context.Background(), makeProfile()) // no path configured

	entries := reg.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	// Provider that cannot detect should land in Detected state (not Enabled, not Errored).
	if entries[0].State != providers.Detected {
		t.Errorf("state: got %s, want Detected", entries[0].State)
	}
}

// TestBoot_OpenError_SetsErroredNotFatal asserts that an Open error sets the
// provider to Errored but does NOT prevent subsequent providers from loading.
func TestBoot_OpenError_SetsErroredNotFatal(t *testing.T) {
	bad := providertest.NewFakeProvider(providertest.FakeProviderOptions{
		ID:              "bad",
		DetectAvailable: true,
		OpenErr:         errors.New("open failed"),
	})
	good := providertest.NewFakeProvider(providertest.FakeProviderOptions{
		ID:              "good",
		DetectAvailable: true,
	})

	reg := providers.NewRegistry(slog.Default())
	reg.Register(bad)
	reg.Register(good)
	reg.Boot(context.Background(), makeProfile("bad", "good"))

	entries := reg.Entries()
	stateByID := make(map[string]providers.State)
	for _, e := range entries {
		stateByID[e.ID] = e.State
	}

	if stateByID["bad"] != providers.Errored {
		t.Errorf("bad provider: got state %s, want Errored", stateByID["bad"])
	}
	if stateByID["good"] != providers.Enabled {
		t.Errorf("good provider: got state %s, want Enabled", stateByID["good"])
	}
}

// TestBoot_EnabledProvider asserts that a detected + enabled provider ends up
// in Enabled state after Boot.
func TestBoot_EnabledProvider(t *testing.T) {
	fp := providertest.NewFakeProvider(providertest.FakeProviderOptions{
		ID:              "engram",
		Featured:        true,
		DetectAvailable: true,
	})

	reg := providers.NewRegistry(slog.Default())
	reg.Register(fp)
	reg.Boot(context.Background(), makeProfile("engram"))

	if reg.ActiveCount() != 1 {
		t.Errorf("ActiveCount: got %d, want 1", reg.ActiveCount())
	}
}

// TestBoot_DisabledProvider asserts that a detected but cfg.Enabled=false
// provider lands in Disabled state.
func TestBoot_DisabledProvider(t *testing.T) {
	fp := providertest.NewFakeProvider(providertest.FakeProviderOptions{
		ID:              "engram",
		DetectAvailable: true,
	})

	reg := providers.NewRegistry(slog.Default())
	reg.Register(fp)

	// Profile marks provider as detected but NOT enabled.
	profile := providertest.NewFakeProfile(map[string]providers.ProviderConfig{
		"engram": {Enabled: false, Path: "/fake/engram"},
	})
	reg.Boot(context.Background(), profile)

	entries := reg.Entries()
	if entries[0].State != providers.Disabled {
		t.Errorf("state: got %s, want Disabled", entries[0].State)
	}
}

// TestRegistry_FeaturedFirst asserts that featured providers appear before
// non-featured ones in Entries and NavGroups regardless of registration order.
func TestRegistry_FeaturedFirst(t *testing.T) {
	regular := providertest.NewFakeProvider(providertest.FakeProviderOptions{
		ID:              "cc-sessions",
		DetectAvailable: true,
	})
	featured := providertest.NewFakeProvider(providertest.FakeProviderOptions{
		ID:              "engram",
		Featured:        true,
		DetectAvailable: true,
	})

	reg := providers.NewRegistry(slog.Default())
	// Register regular BEFORE featured to prove ordering is not registration-order.
	reg.Register(regular)
	reg.Register(featured)
	reg.Boot(context.Background(), makeProfile("engram", "cc-sessions"))

	entries := reg.Entries()
	if len(entries) < 2 {
		t.Fatalf("expected ≥2 entries, got %d", len(entries))
	}
	if entries[0].ID != "engram" {
		t.Errorf("first entry: got %q, want engram (featured first)", entries[0].ID)
	}
}

// TestSwitchProfile_AbortOnError asserts that SwitchProfile keeps the old
// profile active when any provider fails to open for the next profile.
func TestSwitchProfile_AbortOnError(t *testing.T) {
	fp := providertest.NewFakeProvider(providertest.FakeProviderOptions{
		ID:              "engram",
		Featured:        true,
		DetectAvailable: true,
	})

	reg := providers.NewRegistry(slog.Default())
	reg.Register(fp)
	reg.Boot(context.Background(), makeProfile("engram"))

	// Now make Open fail for the next profile.
	fp.SetOpenErr(errors.New("next profile open failed"))

	nextProfile := providertest.NewFakeProfile(map[string]providers.ProviderConfig{
		"engram": {Enabled: true, Path: "/fake/next"},
	})

	err := reg.SwitchProfile(context.Background(), nextProfile)
	if err == nil {
		t.Fatal("SwitchProfile: expected error, got nil")
	}

	// Old profile must still be active.
	if reg.ActiveCount() != 1 {
		t.Errorf("ActiveCount after failed switch: got %d, want 1", reg.ActiveCount())
	}

	entries := reg.Entries()
	if entries[0].State != providers.Enabled {
		t.Errorf("provider state after failed switch: got %s, want Enabled", entries[0].State)
	}
}

// TestSwitchProfile_Success asserts that SwitchProfile swaps instances when
// all opens succeed.
func TestSwitchProfile_Success(t *testing.T) {
	fp := providertest.NewFakeProvider(providertest.FakeProviderOptions{
		ID:              "engram",
		Featured:        true,
		DetectAvailable: true,
	})

	reg := providers.NewRegistry(slog.Default())
	reg.Register(fp)
	reg.Boot(context.Background(), makeProfile("engram"))

	nextProfile := providertest.NewFakeProfile(map[string]providers.ProviderConfig{
		"engram": {Enabled: true, Path: "/fake/next"},
	})

	err := reg.SwitchProfile(context.Background(), nextProfile)
	if err != nil {
		t.Fatalf("SwitchProfile: unexpected error: %v", err)
	}

	if reg.ActiveCount() != 1 {
		t.Errorf("ActiveCount after switch: got %d, want 1", reg.ActiveCount())
	}
}

// TestNavGroups_OnlyEnabled asserts that NavGroups returns entries only for
// Enabled providers.
func TestNavGroups_OnlyEnabled(t *testing.T) {
	enabled := providertest.NewFakeProvider(providertest.FakeProviderOptions{
		ID:              "engram",
		Featured:        true,
		DetectAvailable: true,
	})
	disabled := providertest.NewFakeProvider(providertest.FakeProviderOptions{
		ID:              "cc-sessions",
		DetectAvailable: false,
	})

	reg := providers.NewRegistry(slog.Default())
	reg.Register(enabled)
	reg.Register(disabled)
	reg.Boot(context.Background(), makeProfile("engram"))

	groups := reg.NavGroups()
	if len(groups) != 1 {
		t.Fatalf("NavGroups: got %d groups, want 1", len(groups))
	}
	if groups[0].ID != "engram" {
		t.Errorf("NavGroups[0].ID: got %q, want engram", groups[0].ID)
	}
}

// TestAggregateHealth_AllEnabled asserts that AggregateHealth returns ok=true
// when all enabled providers are healthy.
func TestAggregateHealth_AllEnabled(t *testing.T) {
	fp := providertest.NewFakeProvider(providertest.FakeProviderOptions{
		ID:              "engram",
		Featured:        true,
		DetectAvailable: true,
	})

	reg := providers.NewRegistry(slog.Default())
	reg.Register(fp)
	reg.Boot(context.Background(), makeProfile("engram"))

	ok, healths := reg.AggregateHealth(context.Background())
	if !ok {
		t.Error("AggregateHealth: got ok=false, want true")
	}
	if len(healths) == 0 {
		t.Fatal("AggregateHealth: no health entries returned")
	}
	found := false
	for _, h := range healths {
		if h.ID == "engram" {
			found = true
			if !h.OK {
				t.Error("engram health: got ok=false, want true")
			}
			if !h.Active {
				t.Error("engram health: got active=false, want true")
			}
		}
	}
	if !found {
		t.Error("AggregateHealth: engram entry not found")
	}
}

// TestActiveCount_ZeroWhenNoneEnabled asserts that ActiveCount returns 0 when
// no providers are enabled.
func TestActiveCount_ZeroWhenNoneEnabled(t *testing.T) {
	missing := providertest.NewFakeProvider(providertest.FakeProviderOptions{
		ID:              "engram",
		DetectAvailable: false,
	})
	reg := providers.NewRegistry(slog.Default())
	reg.Register(missing)
	reg.Boot(context.Background(), providertest.NewFakeProfile(nil))

	if reg.ActiveCount() != 0 {
		t.Errorf("ActiveCount: got %d, want 0", reg.ActiveCount())
	}
}

// TestRegistry_DuplicateIDPanics asserts that registering two providers with
// the same ID panics with an informative message.
func TestRegistry_DuplicateIDPanics(t *testing.T) {
	fp1 := providertest.NewFakeProvider(providertest.FakeProviderOptions{ID: "engram"})
	fp2 := providertest.NewFakeProvider(providertest.FakeProviderOptions{ID: "engram"})

	reg := providers.NewRegistry(slog.Default())
	reg.Register(fp1)

	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic for duplicate provider ID, got none")
		}
	}()
	reg.Register(fp2)
}

// ---------------------------------------------------------------------------
// WU-8: Registry.Validate, Enable, Disable, ProviderMeta
// ---------------------------------------------------------------------------

// TestRegistry_Validate_DelegatestoProvider asserts that Registry.Validate
// delegates to the underlying Provider.Validate and returns its error.
func TestRegistry_Validate_DelegatestoProvider(t *testing.T) {
	validateErr := errors.New("schema-invalid")
	fp := providertest.NewFakeProvider(providertest.FakeProviderOptions{
		ID:          "engram",
		ValidateErr: validateErr,
	})
	reg := providers.NewRegistry(slog.Default())
	reg.Register(fp)

	cfg := providers.ProviderConfig{Path: "/some/path"}
	err := reg.Validate(context.Background(), "engram", cfg)
	if !errors.Is(err, validateErr) {
		t.Errorf("Validate: got %v, want %v", err, validateErr)
	}
}

// TestRegistry_Validate_UnknownID asserts that Registry.Validate returns an
// error for an unknown provider ID.
func TestRegistry_Validate_UnknownID(t *testing.T) {
	reg := providers.NewRegistry(slog.Default())
	err := reg.Validate(context.Background(), "nonexistent", providers.ProviderConfig{Path: "/x"})
	if err == nil {
		t.Error("Validate with unknown ID: expected error, got nil")
	}
}

// TestRegistry_Enable_OpensAndSetsEnabled asserts that Enable transitions a
// provider from Detected/Disabled state to Enabled.
func TestRegistry_Enable_OpensAndSetsEnabled(t *testing.T) {
	fp := providertest.NewFakeProvider(providertest.FakeProviderOptions{
		ID:              "engram",
		DetectAvailable: false, // not detected at boot → Detected state
	})
	reg := providers.NewRegistry(slog.Default())
	reg.Register(fp)
	// Boot without a valid path → provider stays in Detected (not auto-opened).
	reg.Boot(context.Background(), providertest.NewFakeProfile(nil))

	// Now the user provides a path and enables the provider manually.
	fp.SetDetectAvailable(true) // path is now valid
	cfg := providers.ProviderConfig{Enabled: true, Path: "/now/valid"}
	if err := reg.Enable(context.Background(), "engram", cfg); err != nil {
		t.Fatalf("Enable: unexpected error: %v", err)
	}
	if reg.ActiveCount() != 1 {
		t.Errorf("ActiveCount after Enable: got %d, want 1", reg.ActiveCount())
	}
}

// TestRegistry_Disable_ClosesAndSetsDisabled asserts that Disable transitions
// an Enabled provider to Disabled state and decrements ActiveCount.
func TestRegistry_Disable_ClosesAndSetsDisabled(t *testing.T) {
	fp := providertest.NewFakeProvider(providertest.FakeProviderOptions{
		ID:              "engram",
		DetectAvailable: true,
	})
	reg := providers.NewRegistry(slog.Default())
	reg.Register(fp)
	reg.Boot(context.Background(), makeProfile("engram"))

	if reg.ActiveCount() != 1 {
		t.Fatalf("pre-condition: want 1 active provider, got %d", reg.ActiveCount())
	}

	if err := reg.Disable(context.Background(), "engram"); err != nil {
		t.Fatalf("Disable: unexpected error: %v", err)
	}
	if reg.ActiveCount() != 0 {
		t.Errorf("ActiveCount after Disable: got %d, want 0", reg.ActiveCount())
	}
	entries := reg.Entries()
	if entries[0].State != providers.Disabled {
		t.Errorf("state after Disable: got %s, want Disabled", entries[0].State)
	}
}

// TestRegistry_ProviderMeta_ReturnsInfo asserts that ProviderMeta returns the
// display name, tier, and detection info for a known provider.
func TestRegistry_ProviderMeta_ReturnsInfo(t *testing.T) {
	fp := providertest.NewFakeProvider(providertest.FakeProviderOptions{
		ID:              "engram",
		Featured:        true,
		DetectAvailable: true,
	})
	reg := providers.NewRegistry(slog.Default())
	reg.Register(fp)
	reg.Boot(context.Background(), makeProfile("engram"))

	meta, ok := reg.ProviderMeta("engram")
	if !ok {
		t.Fatal("ProviderMeta: expected ok=true for registered provider, got false")
	}
	if meta.ID != "engram" {
		t.Errorf("meta.ID: got %q, want engram", meta.ID)
	}
	if meta.DisplayName == "" {
		t.Error("meta.DisplayName: expected non-empty")
	}
}

// TestRegistry_ProviderMeta_UnknownID asserts that ProviderMeta returns
// ok=false for a provider that is not registered.
func TestRegistry_ProviderMeta_UnknownID(t *testing.T) {
	reg := providers.NewRegistry(slog.Default())
	_, ok := reg.ProviderMeta("nonexistent")
	if ok {
		t.Error("ProviderMeta with unknown ID: expected ok=false, got true")
	}
}

// TestDetectValidate_Contract asserts the basic Detect / Validate contract on
// FakeProvider (the stub used for framework tests).
func TestDetectValidate_Contract(t *testing.T) {
	t.Run("detect available", func(t *testing.T) {
		fp := providertest.NewFakeProvider(providertest.FakeProviderOptions{
			ID:              "engram",
			DetectAvailable: true,
		})
		det := fp.Detect(context.Background(), providers.ProviderConfig{Path: "/a/b"})
		if !det.Available {
			t.Error("expected Available=true")
		}
		if det.Reason != "ok" {
			t.Errorf("Reason: got %q, want ok", det.Reason)
		}
	})
	t.Run("detect unavailable", func(t *testing.T) {
		fp := providertest.NewFakeProvider(providertest.FakeProviderOptions{
			ID:              "engram",
			DetectAvailable: false,
		})
		det := fp.Detect(context.Background(), providers.ProviderConfig{Path: ""})
		if det.Available {
			t.Error("expected Available=false")
		}
		if det.Reason != "path-missing" {
			t.Errorf("Reason: got %q, want path-missing", det.Reason)
		}
	})
	t.Run("validate ok", func(t *testing.T) {
		fp := providertest.NewFakeProvider(providertest.FakeProviderOptions{ID: "engram"})
		err := fp.Validate(context.Background(), providers.ProviderConfig{Path: "/a/b"})
		if err != nil {
			t.Errorf("Validate: unexpected error: %v", err)
		}
	})
	t.Run("validate error", func(t *testing.T) {
		fp := providertest.NewFakeProvider(providertest.FakeProviderOptions{
			ID:          "engram",
			ValidateErr: errors.New("schema-invalid"),
		})
		err := fp.Validate(context.Background(), providers.ProviderConfig{Path: "/a/b"})
		if err == nil {
			t.Error("Validate: expected error, got nil")
		}
	})
}
