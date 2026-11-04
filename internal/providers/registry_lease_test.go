package providers_test

import (
	"context"
	"testing"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/providers/providertest"
)

// S4: a request holds a lease on the instances it uses; an instance that a
// toggle, re-open, profile switch or shutdown supersedes is closed only after
// its leases are released, and no new lease can reach it.

func leaseFixture(t *testing.T) (*providers.Registry, *providertest.FakeProvider) {
	t.Helper()
	fp := providertest.NewFakeProvider(providertest.FakeProviderOptions{ID: "fake", DetectAvailable: true})
	profile := providertest.NewFakeProfile(map[string]providers.ProviderConfig{
		"fake": {Enabled: true, Path: "/a"},
	})
	reg := providertest.NewRegistry(t, profile, fp)
	if len(fp.Opened()) != 1 {
		t.Fatalf("boot opened %d instances, want 1", len(fp.Opened()))
	}
	return reg, fp
}

// waitClosed polls until inst is closed or the deadline passes.
func waitClosed(t *testing.T, inst *providertest.FakeInstance) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !inst.Closed() {
		if time.Now().After(deadline) {
			t.Fatal("instance was not closed after its lease was released")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestRegistryLease_SupersededInstanceWaitsForLease covers Disable, a re-open
// through Enable and SwitchProfile: the old instance stays open (and pinned)
// while leased, new leases never reach it, and it closes on release.
func TestRegistryLease_SupersededInstanceWaitsForLease(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name     string
		replace  func(*providers.Registry) error
		nextOpen bool // the new lease yields a freshly opened instance
	}{
		{"disable", func(r *providers.Registry) error { return r.Disable(ctx, "fake") }, false},
		{"re-enable", func(r *providers.Registry) error {
			return r.Enable(ctx, "fake", providers.ProviderConfig{Enabled: true, Path: "/b"})
		}, true},
		{"switch profile", func(r *providers.Registry) error {
			return r.SwitchProfile(ctx, providertest.NewFakeProfile(map[string]providers.ProviderConfig{"fake": {Enabled: true, Path: "/c"}}))
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reg, fp := leaseFixture(t)
			old := fp.Opened()[0]
			l := reg.Acquire()
			if err := tc.replace(reg); err != nil {
				t.Fatal(err)
			}
			if old.Closed() {
				t.Fatal("superseded instance closed while a request still holds it")
			}
			if l.Instance("fake") != providers.Instance(old) || l.Find(func(providers.Instance) bool { return true }) != providers.Instance(old) {
				t.Fatal("lease no longer yields the instance it pinned")
			}
			next := reg.Acquire()
			defer next.Release()
			var want providers.Instance
			if tc.nextOpen {
				want = fp.Opened()[1]
			}
			if got := next.Instance("fake"); got != want {
				t.Fatalf("new lease yields %v, want %v", got, want)
			}
			l.Release()
			l.Release() // idempotent
			waitClosed(t, old)
			if tc.nextOpen && fp.Opened()[1].Closed() {
				t.Fatal("the new instance was closed")
			}
		})
	}
}

func TestRegistryLease_NoLeaseClosesImmediately(t *testing.T) {
	reg, fp := leaseFixture(t)
	reg.Acquire().Release()
	if err := reg.Disable(context.Background(), "fake"); err != nil {
		t.Fatal(err)
	}
	if !fp.Opened()[0].Closed() {
		t.Fatal("instance without leases was not closed synchronously by Disable")
	}
}

func TestRegistryLease_ShutdownWaitsForLease(t *testing.T) {
	reg, fp := leaseFixture(t)
	inst := fp.Opened()[0]

	l := reg.Acquire()
	done := make(chan struct{})
	go func() { reg.Shutdown(context.Background()); close(done) }()
	select {
	case <-done:
		t.Fatal("Shutdown returned while a request still holds a lease")
	case <-time.After(50 * time.Millisecond):
	}
	if inst.Closed() {
		t.Fatal("Shutdown closed a leased instance before the drain timeout")
	}
	l.Release()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown did not return after the lease was released")
	}
	if !inst.Closed() {
		t.Fatal("Shutdown returned without closing the instance")
	}
}

func TestRegistryLease_ShutdownDrainTimeoutIsBounded(t *testing.T) {
	reg, fp := leaseFixture(t)
	reg.SetDrainTimeout(20 * time.Millisecond)
	_ = reg.Acquire() // never released

	start := time.Now()
	reg.Shutdown(context.Background())
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Shutdown took %v with a stuck lease, want it bounded by the drain timeout", elapsed)
	}
	if !fp.Opened()[0].Closed() {
		t.Fatal("Shutdown did not force-close the instance after the drain timeout")
	}
}
