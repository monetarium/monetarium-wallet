// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"context"
	"math/big"
	"testing"

	"github.com/monetarium/monetarium-node/chaincfg"
	"github.com/monetarium/monetarium-node/cointype"
)

// TestGetEffectiveFeeTreatsZeroAsUnset pins the post-2026-05-04 contract:
// GetEffectiveFee treats a zero-valued fee at any layer (manual / RPC /
// static) as UNSET and consults the next fallback. This aligns with
// consensus reality — only SKA emission transactions are allowed to be
// zero-fee, and they construct fee=0 directly rather than going through
// this path. Every other tx must pay a positive fee or the node rejects it.
func TestGetEffectiveFeeTreatsZeroAsUnset(t *testing.T) {
	const ct = cointype.CoinType(1)
	chainParams := &chaincfg.Params{
		SKACoins: map[cointype.CoinType]*chaincfg.SKACoinConfig{
			1: {Active: true, MinRelayTxFee: big.NewInt(1000), MaxFeeMultiplier: 2500},
		},
	}

	t.Run("zero static fee falls through to no-positive-fee error", func(t *testing.T) {
		w := &Wallet{chainParams: chainParams}
		w.manualFees = map[cointype.CoinType]*cointype.SKAAmount{}
		w.staticFees = map[cointype.CoinType]*cointype.SKAAmount{
			ct: skaAmountPtr(cointype.Zero()), // explicit zero -> treated as unset
		}

		fee, src, err := w.GetEffectiveFee(context.Background(), ct)
		if err == nil {
			t.Fatalf("want error 'no positive fee configured', got fee=%s src=%s", fee, src)
		}
		if !fee.IsZero() {
			t.Errorf("error path returned non-zero fee %s", fee)
		}
	})

	t.Run("positive static fee is honored", func(t *testing.T) {
		w := &Wallet{chainParams: chainParams}
		w.manualFees = map[cointype.CoinType]*cointype.SKAAmount{}
		w.staticFees = map[cointype.CoinType]*cointype.SKAAmount{
			ct: skaAmountPtr(cointype.SKAAmountFromInt64(2500)),
		}

		fee, src, err := w.GetEffectiveFee(context.Background(), ct)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if src != "static" {
			t.Errorf("source = %q, want %q", src, "static")
		}
		if v, _ := fee.Int64(); v != 2500 {
			t.Errorf("fee = %s, want 2500", fee)
		}
	})

	t.Run("zero manual override falls through to positive static", func(t *testing.T) {
		w := &Wallet{chainParams: chainParams}
		w.manualFees = map[cointype.CoinType]*cointype.SKAAmount{
			ct: skaAmountPtr(cointype.Zero()), // zero manual -> ignored
		}
		w.staticFees = map[cointype.CoinType]*cointype.SKAAmount{
			ct: skaAmountPtr(cointype.SKAAmountFromInt64(1500)),
		}

		fee, src, err := w.GetEffectiveFee(context.Background(), ct)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if src != "static" {
			t.Errorf("source = %q, want %q (zero manual must not be honored)", src, "static")
		}
		if v, _ := fee.Int64(); v != 1500 {
			t.Errorf("fee = %s, want 1500", fee)
		}
	})

	t.Run("positive manual override takes priority", func(t *testing.T) {
		w := &Wallet{chainParams: chainParams}
		w.manualFees = map[cointype.CoinType]*cointype.SKAAmount{
			ct: skaAmountPtr(cointype.SKAAmountFromInt64(9999)),
		}
		w.staticFees = map[cointype.CoinType]*cointype.SKAAmount{
			ct: skaAmountPtr(cointype.SKAAmountFromInt64(1500)),
		}

		fee, src, err := w.GetEffectiveFee(context.Background(), ct)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if src != "manual" {
			t.Errorf("source = %q, want %q", src, "manual")
		}
		if v, _ := fee.Int64(); v != 9999 {
			t.Errorf("fee = %s, want 9999", fee)
		}
	})
}

// TestRelayFeeForCoinTypeFallsThroughToChainparams pins the late-activation
// invariant: when GetEffectiveFee returns its 'no positive fee configured'
// error AND staticFees has nothing usable, RelayFeeForCoinType reads the SKA
// chainparams MinRelayTxFee directly. Without this fallback, a SKA coin
// whose emission window opens mid-session would author txs at zero fee.
func TestRelayFeeForCoinTypeFallsThroughToChainparams(t *testing.T) {
	const ct = cointype.CoinType(1)
	chainParams := &chaincfg.Params{
		SKACoins: map[cointype.CoinType]*chaincfg.SKACoinConfig{
			1: {Active: true, MinRelayTxFee: big.NewInt(7000), MaxFeeMultiplier: 2500},
		},
	}
	w := &Wallet{chainParams: chainParams}
	w.manualFees = map[cointype.CoinType]*cointype.SKAAmount{}
	w.staticFees = map[cointype.CoinType]*cointype.SKAAmount{} // nothing seeded

	got := w.RelayFeeForCoinType(context.Background(), ct)
	if got.IsZero() {
		t.Fatalf("late-activation chainparams fallback failed: got 0, want 7000")
	}
	if v, _ := got.Int64(); v != 7000 {
		t.Errorf("got %s, want 7000 (chainparams.SKACoins[1].MinRelayTxFee)", got)
	}
}
