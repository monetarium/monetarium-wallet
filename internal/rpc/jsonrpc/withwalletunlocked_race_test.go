// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package jsonrpc

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// gateState is a thread-safe stand-in for the wallet's locked-state machine.
// It captures the contract withWalletPassphraseGate depends on: Locked() reads
// the current state, Unlock transitions to unlocked, Lock transitions to
// locked. The mock also records call counts so tests can assert ordering
// invariants.
type gateState struct {
	mu          sync.Mutex
	locked      bool
	unlockCalls int64 // atomic
	lockCalls   int64 // atomic
	unlockErr   error // returned by Unlock when set
	lockErr     error // returned by Lock when set
	// onUnlock and onLock fire after the state transition while holding mu —
	// useful for forcing a goroutine to wait between Unlock and fn().
	onUnlock func()
	onLock   func()
}

func newGate(initiallyLocked bool) *gateState {
	return &gateState{locked: initiallyLocked}
}

func (g *gateState) Locked() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.locked
}

func (g *gateState) Unlock(_ context.Context, _ []byte, _ <-chan time.Time) error {
	atomic.AddInt64(&g.unlockCalls, 1)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.unlockErr != nil {
		return g.unlockErr
	}
	g.locked = false
	if g.onUnlock != nil {
		g.onUnlock()
	}
	return nil
}

func (g *gateState) Lock() error {
	atomic.AddInt64(&g.lockCalls, 1)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.lockErr != nil {
		return g.lockErr
	}
	g.locked = true
	if g.onLock != nil {
		g.onLock()
	}
	return nil
}

func (g *gateState) Counts() (unlock, lock int64) {
	return atomic.LoadInt64(&g.unlockCalls), atomic.LoadInt64(&g.lockCalls)
}

// TestWithWalletPassphraseGateAlwaysRelocks pins the post-M4 contract: every
// successful Unlock is followed by a Lock on return, regardless of whether the
// gate was already unlocked at entry. This closes the privileged-bystander
// surface where an emission/burn call would silently inherit (and leave open)
// an ambient walletpassphrase window opened by an unrelated client.
func TestWithWalletPassphraseGateAlwaysRelocks(t *testing.T) {
	t.Run("starts locked", func(t *testing.T) {
		g := newGate(true)
		_, err := withWalletPassphraseGate(context.Background(), g, "pp", func() (any, error) {
			if g.Locked() {
				t.Errorf("fn() ran while gate was still locked")
			}
			return nil, nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !g.Locked() {
			t.Errorf("gate must be relocked after fn(); was %v", g.Locked())
		}
		uc, lc := g.Counts()
		if uc != 1 || lc != 1 {
			t.Errorf("unlock=%d lock=%d; want 1, 1", uc, lc)
		}
	})

	t.Run("starts unlocked: ambient window is terminated", func(t *testing.T) {
		g := newGate(false)
		_, err := withWalletPassphraseGate(context.Background(), g, "pp", func() (any, error) {
			return nil, nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !g.Locked() {
			t.Errorf("gate must be relocked even when an ambient walletpassphrase window was open at entry")
		}
		_, lc := g.Counts()
		if lc != 1 {
			t.Errorf("Lock() must run exactly once; got %d calls", lc)
		}
	})
}

// TestWithWalletPassphraseGateReturnsPassphraseError confirms an Unlock
// failure is surfaced as ErrRPCWalletPassphraseIncorrect rather than the
// underlying internal error string. This keeps the RPC contract stable and
// avoids leaking DB/cipher diagnostics to clients.
func TestWithWalletPassphraseGateReturnsPassphraseError(t *testing.T) {
	g := newGate(true)
	g.unlockErr = fmt.Errorf("bcrypt: hashedPassword is not the hash of the given password")
	_, err := withWalletPassphraseGate(context.Background(), g, "wrong", func() (any, error) {
		t.Fatalf("fn() must not run when Unlock fails")
		return nil, nil
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if msg := err.Error(); msg == g.unlockErr.Error() {
		t.Errorf("internal Unlock error leaked to caller: %q", msg)
	}
	_, lc := g.Counts()
	if lc != 0 {
		t.Errorf("Lock() must not be called when Unlock failed; got %d calls", lc)
	}
}

// TestWithWalletPassphraseGateRelockFailureSurfaces pins the contract that a
// relock failure on a successful fn is reported to the caller (so the
// operator can see that the wallet may still be unlocked). Conversely, when
// fn itself fails, fn's error wins — it is the more useful diagnostic.
func TestWithWalletPassphraseGateRelockFailureSurfaces(t *testing.T) {
	t.Run("fn ok, lock fails: surfaces lock error", func(t *testing.T) {
		g := newGate(true)
		g.lockErr = fmt.Errorf("manager.Lock: bbolt: I/O error")
		_, err := withWalletPassphraseGate(context.Background(), g, "pp", func() (any, error) {
			return "result", nil
		})
		if err == nil {
			t.Fatalf("expected relock error to surface, got nil")
		}
	})

	t.Run("fn fails, lock also fails: fn error wins", func(t *testing.T) {
		g := newGate(true)
		g.lockErr = fmt.Errorf("manager.Lock: bbolt: I/O error")
		fnErr := fmt.Errorf("emission already happened")
		_, err := withWalletPassphraseGate(context.Background(), g, "pp", func() (any, error) {
			return nil, fnErr
		})
		if err != fnErr {
			t.Errorf("expected fn error to win, got %v", err)
		}
	})
}

// TestWithWalletPassphraseGateConcurrentLockedStart drives N concurrent
// callers from a starting locked state and pins the per-call contract:
//
//   - Every caller calls Unlock exactly once (count == N).
//   - Every caller calls Lock exactly once on its way out (count == N).
//   - Under -race no data race fires inside the helper itself.
//
// Run under -race.
func TestWithWalletPassphraseGateConcurrentLockedStart(t *testing.T) {
	const N = 32
	g := newGate(true)

	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_, _ = withWalletPassphraseGate(context.Background(), g, "pp", func() (any, error) {
				return nil, nil
			})
		}()
	}
	wg.Wait()

	uc, lc := g.Counts()
	if uc != N {
		t.Errorf("Unlock call count = %d, want %d (one per caller)", uc, N)
	}
	if lc != N {
		t.Errorf("Lock call count = %d, want %d (one per caller — always-relock contract)", lc, N)
	}
	if !g.Locked() {
		t.Errorf("gate must be locked at the end")
	}
}

// TestWithWalletPassphraseGateConcurrentUnlockedStart drives N concurrent
// callers from a starting UNLOCKED state. Under the M4 always-relock
// contract, the gate must end LOCKED — the prior ambient walletpassphrase
// window is intentionally terminated by the per-call gate. Run under -race.
func TestWithWalletPassphraseGateConcurrentUnlockedStart(t *testing.T) {
	const N = 32
	g := newGate(false)
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_, _ = withWalletPassphraseGate(context.Background(), g, "pp", func() (any, error) {
				return nil, nil
			})
		}()
	}
	wg.Wait()

	_, lc := g.Counts()
	if lc != N {
		t.Errorf("Lock call count = %d, want %d (always-relock contract)", lc, N)
	}
	if !g.Locked() {
		t.Errorf("gate must be locked at the end — ambient window must not survive the gate")
	}
}
