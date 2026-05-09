// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"context"
	"testing"

	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-wallet/wallet/walletdb"
)

// TestCompressWalletInternalNoLocksOnEarlyExit pins the H2 fix from the
// 2026-05-05 review: compressWalletInternal must return ([], nil) for
// lockedOps when it exits before the lock loop (e.g. via the
// "too few outputs to consolidate" early return). The earlier code held the
// outpoint-unlock defer inside the inner function — masked correct only by
// the outer mutex's full-function scope. Now the inner function reports
// every outpoint it locked, and the outer compressWallet is the sole owner
// of the release. Pin the empty-lockedOps invariant on the unhappy path so
// future refactors cannot silently leak a lock through an early return.
func TestCompressWalletInternalNoLocksOnEarlyExit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := basicWalletConfig
	w, teardown := testWallet(ctx, t, &cfg, nil)
	defer teardown()

	// Wallet is freshly created with no eligible UTXOs. Calling
	// compressWalletInternal should hit the "too few outputs to
	// consolidate" branch BEFORE the lock loop runs, returning a nil
	// lockedOps slice and leaving w.lockedOutpoints untouched.
	if got := len(w.lockedOutpoints); got != 0 {
		t.Fatalf("freshly-created wallet should start with empty lockedOutpoints; got %d", got)
	}

	var lockedOps []byte // sentinel: rebuilt below via the inner call
	err := walletdb.Update(ctx, w.db, func(dbtx walletdb.ReadWriteTx) error {
		_, _, ops, err := w.compressWalletInternal(ctx, "test.compressWalletInternal", dbtx, 100, 0, nil, cointype.CoinTypeVAR)
		if len(ops) != 0 {
			t.Fatalf("early-return path must report zero locked outpoints; got %d", len(ops))
		}
		lockedOps = make([]byte, len(ops))
		return err
	})
	if err == nil {
		t.Fatalf("expected 'too few outputs to consolidate' error on empty wallet")
	}

	// Critical invariant: w.lockedOutpoints stayed empty across the call.
	// If a future refactor reintroduces an inner-function lock without a
	// matching unlock, this will catch it.
	if got := len(w.lockedOutpoints); got != 0 {
		t.Fatalf("compressWalletInternal early-return leaked %d outpoint(s) into lockedOutpoints", got)
	}
	_ = lockedOps
}
