// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package jsonrpc

import (
	"context"
	"errors"
	"testing"

	"github.com/monetarium/monetarium-node/chaincfg/chainhash"
	"github.com/monetarium/monetarium-node/cointype"
	mondtypes "github.com/monetarium/monetarium-node/rpc/jsonrpc/types"
	"github.com/monetarium/monetarium-node/wire"
	"github.com/monetarium/monetarium-wallet/rpc/jsonrpc/types"
	"github.com/monetarium/monetarium-wallet/wallet/udb"
)

// TestRedeemMultiSigOutsCollectPartialFailure verifies that per-output errors
// are recorded on the response with Complete=false instead of aborting the
// batch (MEDIUM-#3 from the 2026-05-02 review).
func TestRedeemMultiSigOutsCollectPartialFailure(t *testing.T) {
	hashFromByte := func(b byte) chainhash.Hash {
		var h chainhash.Hash
		h[0] = b
		return h
	}
	hashA := hashFromByte(0x11)
	hashB := hashFromByte(0x22)
	hashC := hashFromByte(0x33)

	mso := func(h chainhash.Hash, idx uint32) *udb.MultisigCredit {
		return &udb.MultisigCredit{OutPoint: &wire.OutPoint{Hash: h, Index: idx}}
	}
	msos := []*udb.MultisigCredit{
		mso(hashA, 0),
		mso(hashB, 1),
		mso(hashC, 2),
	}

	// Stub redeemer: succeeds for hashA and hashC, fails for hashB.
	redeem := func(_ context.Context, req *types.RedeemMultiSigOutCmd) (types.RedeemMultiSigOutResult, error) {
		if req.Hash == hashB.String() {
			return types.RedeemMultiSigOutResult{}, errors.New("synthetic failure")
		}
		return types.RedeemMultiSigOutResult{Hex: "deadbeef-" + req.Hash, Complete: true}, nil
	}

	results, truncated := redeemMultiSigOutsCollect(context.Background(), msos, 3, nil, nil, redeem)

	if len(results) != 3 {
		t.Fatalf("want 3 results (one per output), got %d", len(results))
	}
	if truncated {
		t.Errorf("truncated should be false when all credits fit under the cap")
	}
	if !results[0].Complete || results[0].Hex == "" {
		t.Errorf("results[0] should be a successful redemption, got %+v", results[0])
	}
	if results[1].Complete {
		t.Errorf("results[1] should be incomplete (failed), got Complete=true")
	}
	if len(results[1].Errors) != 1 {
		t.Fatalf("results[1] should carry one Errors entry, got %d", len(results[1].Errors))
	}
	if results[1].Errors[0].TxID != hashB.String() {
		t.Errorf("results[1] error TxID: want %s, got %s", hashB.String(), results[1].Errors[0].TxID)
	}
	if results[1].Errors[0].Vout != 1 {
		t.Errorf("results[1] error Vout: want 1, got %d", results[1].Errors[0].Vout)
	}
	if results[1].Errors[0].Error != "synthetic failure" {
		t.Errorf("results[1] error message: want %q, got %q", "synthetic failure", results[1].Errors[0].Error)
	}
	if !results[2].Complete {
		t.Errorf("results[2] should be a successful redemption (later outputs still attempted), got %+v", results[2])
	}
}

// TestRedeemMultiSigOutsCollectRespectsCap verifies that the per-call cap is
// honored even when later items would also fail.
func TestRedeemMultiSigOutsCollectRespectsCap(t *testing.T) {
	hashFromByte := func(b byte) chainhash.Hash {
		var h chainhash.Hash
		h[0] = b
		return h
	}
	mso := func(b byte, idx uint32) *udb.MultisigCredit {
		return &udb.MultisigCredit{OutPoint: &wire.OutPoint{Hash: hashFromByte(b), Index: idx}}
	}
	msos := []*udb.MultisigCredit{mso(1, 0), mso(2, 0), mso(3, 0), mso(4, 0)}
	calls := 0
	redeem := func(_ context.Context, req *types.RedeemMultiSigOutCmd) (types.RedeemMultiSigOutResult, error) {
		calls++
		return types.RedeemMultiSigOutResult{Hex: req.Hash, Complete: true}, nil
	}
	results, truncated := redeemMultiSigOutsCollect(context.Background(), msos, 2, nil, nil, redeem)
	if len(results) != 2 {
		t.Fatalf("want 2 results capped, got %d", len(results))
	}
	if calls != 2 {
		t.Errorf("redeemer should be invoked exactly cap=2 times, got %d", calls)
	}
	if !truncated {
		t.Errorf("truncated should be true when cap=2 is hit with 4 credits available")
	}
}

// TestRedeemMultiSigOutsCollectTruncationFilterAware pins the HIGH fix from
// the 2026-05-02 review: truncated must reflect post-filter emission count,
// not the pre-filter total. Previously a 200-SKA + 200-VAR address with
// cap=256 and a SKA-only filter reported truncated=true even though every
// SKA credit was emitted, causing pagination clients to loop forever.
func TestRedeemMultiSigOutsCollectTruncationFilterAware(t *testing.T) {
	hashFromByte := func(b byte) chainhash.Hash {
		var h chainhash.Hash
		h[0] = b
		return h
	}
	mkCredit := func(hashByte byte, ct cointype.CoinType) *udb.MultisigCredit {
		return &udb.MultisigCredit{
			OutPoint: &wire.OutPoint{Hash: hashFromByte(hashByte), Index: 0},
			CoinType: ct,
		}
	}
	noopRedeem := func(_ context.Context, req *types.RedeemMultiSigOutCmd) (types.RedeemMultiSigOutResult, error) {
		return types.RedeemMultiSigOutResult{Complete: true, Hex: req.Hash}, nil
	}

	t.Run("MixedCoinTypeUnderCapNotTruncated", func(t *testing.T) {
		// 3 VAR + 3 SKA1, cap=4, filter=SKA1: 3 SKA1 emitted, no
		// matching credits remain. Pre-filter len=6 > cap=4 would have
		// said truncated=true, but post-filter we emit all 3 with no more
		// matching credits, so truncated=false.
		msos := []*udb.MultisigCredit{
			mkCredit(0x10, cointype.CoinTypeVAR),
			mkCredit(0x11, cointype.CoinType(1)),
			mkCredit(0x20, cointype.CoinTypeVAR),
			mkCredit(0x12, cointype.CoinType(1)),
			mkCredit(0x30, cointype.CoinTypeVAR),
			mkCredit(0x13, cointype.CoinType(1)),
		}
		filterSKA1 := uint8(1)
		results, truncated := redeemMultiSigOutsCollect(
			context.Background(), msos, 4, nil, &filterSKA1, noopRedeem)
		if len(results) != 3 {
			t.Fatalf("want 3 SKA1 results, got %d", len(results))
		}
		if truncated {
			t.Errorf("truncated must be false: all matching credits emitted")
		}
	})

	t.Run("MatchingCreditsExceedCapTruncated", func(t *testing.T) {
		// 5 SKA1 credits, cap=2, filter=SKA1: cap is hit and 3 matching
		// credits remain. truncated=true.
		msos := []*udb.MultisigCredit{
			mkCredit(0x11, cointype.CoinType(1)),
			mkCredit(0x12, cointype.CoinType(1)),
			mkCredit(0x13, cointype.CoinType(1)),
			mkCredit(0x14, cointype.CoinType(1)),
			mkCredit(0x15, cointype.CoinType(1)),
		}
		filterSKA1 := uint8(1)
		results, truncated := redeemMultiSigOutsCollect(
			context.Background(), msos, 2, nil, &filterSKA1, noopRedeem)
		if len(results) != 2 {
			t.Fatalf("want 2 results capped, got %d", len(results))
		}
		if !truncated {
			t.Errorf("truncated must be true when more matching credits remain past the cap")
		}
	})

	t.Run("CapHitWithOnlyNonMatchingRemaining", func(t *testing.T) {
		// 2 SKA1 then a tail of VAR-only credits, cap=2, filter=SKA1:
		// cap is hit at the second SKA1 but the remaining credits are
		// all filtered out, so truncated=false.
		msos := []*udb.MultisigCredit{
			mkCredit(0x11, cointype.CoinType(1)),
			mkCredit(0x12, cointype.CoinType(1)),
			mkCredit(0x20, cointype.CoinTypeVAR),
			mkCredit(0x21, cointype.CoinTypeVAR),
		}
		filterSKA1 := uint8(1)
		results, truncated := redeemMultiSigOutsCollect(
			context.Background(), msos, 2, nil, &filterSKA1, noopRedeem)
		if len(results) != 2 {
			t.Fatalf("want 2 results, got %d", len(results))
		}
		if truncated {
			t.Errorf("truncated must be false: remaining credits do not match the filter")
		}
	})
}

// TestRedeemMultiSigOutsBatchCoinTypePropagation pins the HIGH-2 fix from the
// 2026-05-02 review: each per-output redeem request must carry the credit's
// own CoinType (mso.CoinType), not the outer cmd.CoinType. Routing every
// credit through the caller-supplied hint dropped SKA credits onto the VAR
// fee/output path when cmd.CoinType was nil, producing malformed
// transactions.
//
// Two scenarios are covered:
//  1. outer coinType=nil ⇒ every credit is processed and each request
//     carries its own per-credit CoinType.
//  2. outer coinType=&skaOne ⇒ only credits matching SKA1 are processed
//     (acts as a coin-type filter).
func TestRedeemMultiSigOutsBatchCoinTypePropagation(t *testing.T) {
	hashFromByte := func(b byte) chainhash.Hash {
		var h chainhash.Hash
		h[0] = b
		return h
	}
	mkCredit := func(hashByte byte, ct cointype.CoinType) *udb.MultisigCredit {
		return &udb.MultisigCredit{
			OutPoint: &wire.OutPoint{Hash: hashFromByte(hashByte), Index: 0},
			CoinType: ct,
		}
	}
	msos := []*udb.MultisigCredit{
		mkCredit(0xA1, cointype.CoinTypeVAR),
		mkCredit(0xB2, cointype.CoinType(1)), // SKA1
		mkCredit(0xC3, cointype.CoinType(2)), // SKA2
	}

	type call struct {
		hash     string
		coinType *uint8
	}

	t.Run("PerCreditCoinTypeWhenOuterNil", func(t *testing.T) {
		var calls []call
		redeem := func(_ context.Context, req *types.RedeemMultiSigOutCmd) (types.RedeemMultiSigOutResult, error) {
			calls = append(calls, call{hash: req.Hash, coinType: req.CoinType})
			return types.RedeemMultiSigOutResult{Complete: true, Hex: req.Hash}, nil
		}

		results, _ := redeemMultiSigOutsCollect(context.Background(), msos, uint32(len(msos)), nil, nil, redeem)
		if len(results) != 3 {
			t.Fatalf("want 3 results, got %d", len(results))
		}
		if len(calls) != 3 {
			t.Fatalf("redeemer should be invoked once per credit, got %d", len(calls))
		}

		want := []cointype.CoinType{cointype.CoinTypeVAR, cointype.CoinType(1), cointype.CoinType(2)}
		for i, c := range calls {
			if c.coinType == nil {
				t.Errorf("call %d: req.CoinType is nil; want per-credit %d", i, want[i])
				continue
			}
			if cointype.CoinType(*c.coinType) != want[i] {
				t.Errorf("call %d: req.CoinType = %d, want %d (mso.CoinType)",
					i, *c.coinType, want[i])
			}
		}

		// Per-iteration pointer binding: every call must observe a distinct
		// *uint8 (otherwise a single shared address would alias to whatever
		// value was assigned last).
		if calls[0].coinType == calls[1].coinType {
			t.Error("CoinType pointers aliased across iterations; each call must own its pointer")
		}
	})

	t.Run("FilterToSKA1WhenOuterSet", func(t *testing.T) {
		var calls []call
		redeem := func(_ context.Context, req *types.RedeemMultiSigOutCmd) (types.RedeemMultiSigOutResult, error) {
			calls = append(calls, call{hash: req.Hash, coinType: req.CoinType})
			return types.RedeemMultiSigOutResult{Complete: true, Hex: req.Hash}, nil
		}

		filterSKA1 := uint8(1)
		results, _ := redeemMultiSigOutsCollect(context.Background(), msos, uint32(len(msos)), nil, &filterSKA1, redeem)
		if len(results) != 1 {
			t.Fatalf("want 1 filtered result (SKA1 only), got %d", len(results))
		}
		if len(calls) != 1 {
			t.Fatalf("redeemer should be invoked exactly once for the SKA1 credit, got %d", len(calls))
		}
		if calls[0].coinType == nil || cointype.CoinType(*calls[0].coinType) != cointype.CoinType(1) {
			got := "<nil>"
			if calls[0].coinType != nil {
				got = string(rune('0' + *calls[0].coinType))
			}
			t.Errorf("filtered call CoinType: want SKA1, got %s", got)
		}
	})
}

// TestFilterLiveMultisigCreditsPhantomSKA mirrors Michael's reported scenario:
// 2 live SKA + 2 live VAR + 1 phantom SKA. The phantom is the orphan left by a
// failed-publish multisig tx (see RemoveUnconfirmed fix in txunmined.go). The
// filter must drop the phantom from `live` and surface it in `skipped` with the
// stored CoinType and the canonical reason string.
func TestFilterLiveMultisigCreditsPhantomSKA(t *testing.T) {
	hashFromByte := func(b byte) chainhash.Hash {
		var h chainhash.Hash
		h[0] = b
		return h
	}
	ska := cointype.CoinType(1)
	mk := func(h chainhash.Hash, ct cointype.CoinType) *udb.MultisigCredit {
		return &udb.MultisigCredit{
			OutPoint: &wire.OutPoint{Hash: h, Index: 0},
			CoinType: ct,
		}
	}
	liveSKA1 := mk(hashFromByte(0xA1), ska)
	liveSKA2 := mk(hashFromByte(0xA2), ska)
	liveVAR1 := mk(hashFromByte(0xB1), cointype.CoinTypeVAR)
	liveVAR2 := mk(hashFromByte(0xB2), cointype.CoinTypeVAR)
	phantomSKA := mk(hashFromByte(0xDE), ska)

	msos := []*udb.MultisigCredit{liveSKA1, liveSKA2, liveVAR1, liveVAR2, phantomSKA}

	// Stub: returns nil only for the phantom (gettxout's "spent or never
	// existed" sentinel), non-nil for the rest. The stub treats any per-call
	// error as live-passthrough, matching the production filter's policy.
	query := func(_ context.Context, op *wire.OutPoint) (*mondtypes.GetTxOutResult, error) {
		if op.Hash == phantomSKA.OutPoint.Hash {
			return nil, nil
		}
		return &mondtypes.GetTxOutResult{}, nil
	}

	live, skipped := filterLiveMultisigCredits(context.Background(), msos, query)

	if len(live) != 4 {
		t.Fatalf("want 4 live credits, got %d", len(live))
	}
	for _, mso := range live {
		if mso.OutPoint.Hash == phantomSKA.OutPoint.Hash {
			t.Errorf("phantom %v leaked into live set", mso.OutPoint)
		}
	}
	if len(skipped) != 1 {
		t.Fatalf("want 1 skipped entry, got %d", len(skipped))
	}
	sk := skipped[0]
	if sk.Hash != phantomSKA.OutPoint.Hash.String() {
		t.Errorf("skipped.Hash: want %s, got %s", phantomSKA.OutPoint.Hash, sk.Hash)
	}
	if sk.CoinType != uint8(ska) {
		t.Errorf("skipped.CoinType: want %d, got %d", ska, sk.CoinType)
	}
	if sk.Reason != "output not unspent on chain" {
		t.Errorf("skipped.Reason: want %q, got %q",
			"output not unspent on chain", sk.Reason)
	}
}

// TestFilterLiveMultisigCreditsSPVFallback pins the no-node-connection path:
// query==nil (no chain.Syncer to consult) must pass every credit through
// unchanged rather than block recovery on operators running SPV-only.
func TestFilterLiveMultisigCreditsSPVFallback(t *testing.T) {
	mk := func(b byte) *udb.MultisigCredit {
		var h chainhash.Hash
		h[0] = b
		return &udb.MultisigCredit{OutPoint: &wire.OutPoint{Hash: h}}
	}
	msos := []*udb.MultisigCredit{mk(0x01), mk(0x02), mk(0x03)}

	live, skipped := filterLiveMultisigCredits(context.Background(), msos, nil)

	if len(live) != len(msos) {
		t.Errorf("SPV fallback: want all %d credits passed through, got %d", len(msos), len(live))
	}
	if len(skipped) != 0 {
		t.Errorf("SPV fallback: want skipped empty, got %d entries", len(skipped))
	}
	for i := range msos {
		if live[i] != msos[i] {
			t.Errorf("SPV fallback: live[%d] should pass the input through unmodified", i)
		}
	}
}

// TestFilterLiveMultisigCreditsTransientErrorPassesThrough verifies that a
// per-credit gettxout failure (e.g., transient RPC flake) is treated as live
// rather than dropped. Failing closed on transient errors would deny recovery
// on flaky links, which matters more for a recovery RPC than perfect accuracy.
func TestFilterLiveMultisigCreditsTransientErrorPassesThrough(t *testing.T) {
	hashFromByte := func(b byte) chainhash.Hash {
		var h chainhash.Hash
		h[0] = b
		return h
	}
	flaky := &udb.MultisigCredit{OutPoint: &wire.OutPoint{Hash: hashFromByte(0xEE)}}
	ok := &udb.MultisigCredit{OutPoint: &wire.OutPoint{Hash: hashFromByte(0xFF)}}

	query := func(_ context.Context, op *wire.OutPoint) (*mondtypes.GetTxOutResult, error) {
		if op.Hash == flaky.OutPoint.Hash {
			return nil, errors.New("transient rpc error")
		}
		return &mondtypes.GetTxOutResult{}, nil
	}

	live, skipped := filterLiveMultisigCredits(context.Background(),
		[]*udb.MultisigCredit{flaky, ok}, query)

	if len(live) != 2 {
		t.Fatalf("want both credits in live (transient-error passthrough), got %d", len(live))
	}
	if len(skipped) != 0 {
		t.Errorf("want skipped empty for transient errors, got %d entries", len(skipped))
	}
}
