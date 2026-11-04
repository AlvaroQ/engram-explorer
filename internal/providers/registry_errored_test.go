package providers_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/providers"
)

// S12 (Phase 2 follow-up C2): a failed re-open leaves the entry Errored and no
// longer serving, so its previous instance must be retired like any other
// superseded instance — closed once its leases drain — instead of staying open
// until Shutdown. Disable on that Errored entry must not leave it open either.

// TestRegistryEnable_FailedReopenRetiresOldInstance: Enable whose Open fails
// moves the entry to Errored and closes the previous instance, after the
// request still holding it releases its lease.
func TestRegistryEnable_FailedReopenRetiresOldInstance(t *testing.T) {
	ctx := context.Background()
	reg, fp := leaseFixture(t)
	old := fp.Opened()[0]
	lease := reg.Acquire()

	fp.SetOpenErr(errors.New("boom"))
	if err := reg.Enable(ctx, "fake", providers.ProviderConfig{Enabled: true, Path: "/b"}); err == nil {
		t.Fatal("Enable with a failing Open returned nil")
	}
	if got := reg.Entries()[0].State; got != providers.Errored {
		t.Fatalf("state after failed re-open = %s, want errored", got)
	}
	if reg.Instance("fake") != nil {
		t.Error("an Errored entry still serves an instance")
	}
	if old.Closed() {
		t.Fatal("old instance closed while a request still leases it")
	}
	if lease.Instance("fake") != old {
		t.Error("the lease lost its pinned instance")
	}

	lease.Release()
	waitClosed(t, old)
}

// TestRegistryDisable_ErroredEntryClosesKeptInstance: Disable on an entry left
// Errored by a failed re-open closes no later than the drained lease, and the
// entry ends Disabled. Shutdown afterwards closes nothing twice.
func TestRegistryDisable_ErroredEntryClosesKeptInstance(t *testing.T) {
	ctx := context.Background()
	reg, fp := leaseFixture(t)
	old := fp.Opened()[0]

	fp.SetOpenErr(errors.New("boom"))
	_ = reg.Enable(ctx, "fake", providers.ProviderConfig{Enabled: true, Path: "/b"})
	if err := reg.Disable(ctx, "fake"); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if got := reg.Entries()[0].State; got != providers.Disabled {
		t.Fatalf("state after Disable = %s, want disabled", got)
	}
	waitClosed(t, old)

	reg.Shutdown(ctx)
	if n := len(fp.Opened()); n != 1 {
		t.Errorf("opened %d instances, want 1 (the failed re-open opened none)", n)
	}
}
