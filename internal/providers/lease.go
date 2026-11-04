package providers

import (
	"context"
	"sync"
	"time"
)

// DefaultDrainTimeout bounds how long a superseded instance waits for the
// requests still using it before it is closed anyway.
const DefaultDrainTimeout = 30 * time.Second

// handle is one opened instance plus the number of leases (requests) holding
// it. Leases are taken under the registry read lock and only on current
// handles; a handle is retired once, under the write lock, when a toggle,
// re-open, profile switch or shutdown supersedes it. So a retired handle never
// gains a lease, and drained closes when it is retired and has none left.
type handle struct {
	inst    Instance
	mu      sync.Mutex
	leases  int
	retired bool
	drained chan struct{}
}

func newHandle(inst Instance) *handle { return &handle{inst: inst, drained: make(chan struct{})} }

func (h *handle) acquire() { h.mu.Lock(); h.leases++; h.mu.Unlock() }

func (h *handle) release() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.leases--; h.leases == 0 && h.retired {
		close(h.drained)
	}
}

// retiredHandle is a superseded instance waiting to be closed.
type retiredHandle struct {
	id string
	h  *handle
}

// retire detaches e's handle (if any), marks it retired and queues it. The
// caller holds the registry write lock and passes the queue to closeRetired
// after unlocking.
func (e *activeEntry) retire(queue []retiredHandle) []retiredHandle {
	h := e.h
	if h == nil {
		return queue
	}
	e.h = nil
	h.mu.Lock()
	h.retired = true
	if h.leases == 0 {
		close(h.drained)
	}
	h.mu.Unlock()
	return append(queue, retiredHandle{id: e.p.ID(), h: h})
}

// closeRetired runs outside the registry lock. A handle without leases is
// closed before it returns; the others close in the background when their
// last lease is released, or when the drain timeout passes (logged).
func (r *Registry) closeRetired(ctx context.Context, queue []retiredHandle, why string) {
	for _, rh := range queue {
		select {
		case <-rh.h.drained:
			r.closeHandle(ctx, rh, why)
			continue
		default:
		}
		r.pending.Add(1)
		timeout := r.drainTimeout()
		go func(rh retiredHandle) {
			defer r.pending.Done()
			t := time.NewTimer(timeout)
			defer t.Stop()
			select {
			case <-rh.h.drained:
			case <-t.C:
				if r.log != nil {
					r.log.Warn("closing provider instance with requests still in flight", "id", rh.id, "drain_timeout", timeout)
				}
			}
			r.closeHandle(context.Background(), rh, why)
		}(rh)
	}
}

func (r *Registry) closeHandle(ctx context.Context, rh retiredHandle, why string) {
	if err := rh.h.inst.Close(ctx); err != nil && r.log != nil {
		r.log.Warn(why, "id", rh.id, "err", err)
	}
}

// SetDrainTimeout sets how long a superseded instance waits for its leases
// before it is closed anyway (DefaultDrainTimeout when not positive).
func (r *Registry) SetDrainTimeout(d time.Duration) {
	r.mu.Lock()
	r.drain = d
	r.mu.Unlock()
}

func (r *Registry) drainTimeout() time.Duration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.drain <= 0 {
		return DefaultDrainTimeout
	}
	return r.drain
}

// waitPending waits at most timeout for the background closes to finish.
func (r *Registry) waitPending(timeout time.Duration) {
	done := make(chan struct{})
	go func() { r.pending.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

// Lease pins the instances that were Enabled when it was acquired. The host
// takes one per request and releases it when the handler returns, so an
// instance superseded mid-request stays open for that request and is closed
// after it. A Lease never yields an instance whose close has begun.
type Lease struct {
	pins []*handle // registry order
	ids  []string
	once sync.Once
}

// Acquire pins every Enabled instance. Release it when done with them.
func (r *Registry) Acquire() *Lease {
	r.mu.RLock()
	defer r.mu.RUnlock()
	l := &Lease{}
	for _, id := range r.order {
		if e := r.entries[id]; e.state == Enabled && e.h != nil {
			e.h.acquire()
			l.pins, l.ids = append(l.pins, e.h), append(l.ids, id)
		}
	}
	return l
}

// Release unpins the instances; a superseded one closes once its last lease
// is released. Safe to call more than once and on a nil Lease.
func (l *Lease) Release() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		for _, h := range l.pins {
			h.release()
		}
	})
}

// Instance returns the pinned instance of the named provider, or nil when it
// was not Enabled at Acquire.
func (l *Lease) Instance(providerID string) Instance {
	if l == nil {
		return nil
	}
	for i, id := range l.ids {
		if id == providerID {
			return l.pins[i].inst
		}
	}
	return nil
}

// Find returns the first pinned instance, in registry order, for which match
// returns true, or nil (the Lease counterpart of Registry.Find).
func (l *Lease) Find(match func(Instance) bool) Instance {
	if l == nil {
		return nil
	}
	for _, h := range l.pins {
		if match(h.inst) {
			return h.inst
		}
	}
	return nil
}

// FindAll returns every pinned instance, in registry order, for which match
// returns true, or nil (the Lease counterpart of Registry.FindAll). It never
// sees an instance enabled after the lease was acquired.
func (l *Lease) FindAll(match func(Instance) bool) []Instance {
	if l == nil {
		return nil
	}
	var out []Instance
	for _, h := range l.pins {
		if match(h.inst) {
			out = append(out, h.inst)
		}
	}
	return out
}
