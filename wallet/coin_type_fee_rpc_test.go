// Copyright (c) 2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"context"
	"math/big"
	"testing"

	"github.com/monetarium/monetarium-node/chaincfg"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/dcrutil"
)

// skaAmountPtr returns the address of an SKAAmount, captured into a fresh
// stack slot so the pointer remains valid for the lifetime of the map entry.
// SKAAmount is immutable, so aliasing across map entries is safe.
func skaAmountPtr(a cointype.SKAAmount) *cointype.SKAAmount { return &a }

// TestCoinTypeFeeManagementMethods tests the new SKA fee management methods
// added to the Wallet struct.
func TestCoinTypeFeeManagementMethods(t *testing.T) {
	t.Log("=== Testing Coin-Type-Aware Fee Management Methods ===")

	// Create test chain parameters with per-coin SKA fee configuration
	chainParams := &chaincfg.Params{
		SKACoins: map[cointype.CoinType]*chaincfg.SKACoinConfig{
			1: {
				Active:           true,
				MinRelayTxFee:    big.NewInt(1000), // 1000 atoms/KB for SKA1
				MaxFeeMultiplier: 2500,
			},
			2: {
				Active:           true,
				MinRelayTxFee:    big.NewInt(1000), // 1000 atoms/KB for SKA2
				MaxFeeMultiplier: 2500,
			},
		},
	}

	// Simulate wallet initialization
	w := &Wallet{
		chainParams: chainParams,
	}

	// Initialize fees as would happen in wallet Open()
	varRelayFee := dcrutil.Amount(10000) // 10000 atoms/KB for VAR
	w.relayFee = varRelayFee

	// Initialize per-cointype fee maps (using SKAAmount for big.Int support)
	w.manualFees = make(map[cointype.CoinType]*cointype.SKAAmount)
	w.staticFees = make(map[cointype.CoinType]*cointype.SKAAmount)
	w.staticFees[cointype.CoinTypeVAR] = skaAmountPtr(cointype.SKAAmountFromInt64(int64(varRelayFee)))
	for ct, config := range w.chainParams.SKACoins {
		if config != nil && config.Active && config.MinRelayTxFee != nil {
			w.staticFees[ct] = skaAmountPtr(cointype.NewSKAAmount(config.MinRelayTxFee))
		}
	}

	t.Run("Test RelayFee and SetRelayFee for VAR", func(t *testing.T) {
		// Test getting VAR relay fee
		fee := w.RelayFee()
		expectedFee := dcrutil.Amount(10000)
		if fee != expectedFee {
			t.Errorf("RelayFee() = %d, expected %d", fee, expectedFee)
		}

		// Test setting VAR relay fee
		newFee := dcrutil.Amount(15000)
		w.SetRelayFee(newFee)
		if w.RelayFee() != newFee {
			t.Errorf("After SetRelayFee(%d), RelayFee() = %d", newFee, w.RelayFee())
		}

		t.Logf("✅ VAR fee management: get=%d, set=%d", fee, newFee)
	})

	t.Run("Test SKARelayFee and SetSKARelayFee for SKA", func(t *testing.T) {
		// Test getting SKA relay fee (now returns SKAAmount)
		fee := w.SKARelayFee()
		expectedFee := cointype.SKAAmountFromInt64(1000)
		if fee.String() != expectedFee.String() {
			t.Errorf("SKARelayFee() = %s, expected %s", fee.String(), expectedFee.String())
		}

		// Test setting SKA relay fee (now takes SKAAmount for full big.Int precision)
		newFee := cointype.SKAAmountFromInt64(2000)
		w.SetSKARelayFee(newFee)
		expectedNewFee := cointype.SKAAmountFromInt64(2000)
		if w.SKARelayFee().String() != expectedNewFee.String() {
			t.Errorf("After SetSKARelayFee(%s), SKARelayFee() = %s", newFee.String(), w.SKARelayFee().String())
		}

		t.Logf("✅ SKA fee management: get=%s, set=%s", fee.String(), w.SKARelayFee().String())
	})

	t.Run("Test RelayFeeForCoinType helper method", func(t *testing.T) {
		// Set different fee rates for VAR and SKA
		varFee := dcrutil.Amount(12000)
		skaFee := cointype.SKAAmountFromInt64(800)
		w.SetRelayFee(varFee)
		w.SetSKARelayFee(skaFee)

		// Test VAR coin type (RelayFeeForCoinType now returns SKAAmount)
		varResult := w.RelayFeeForCoinType(context.Background(), cointype.CoinTypeVAR)
		expectedVarFee := cointype.SKAAmountFromInt64(int64(varFee))
		if varResult.String() != expectedVarFee.String() {
			t.Errorf("RelayFeeForCoinType(VAR) = %s, expected %s", varResult.String(), expectedVarFee.String())
		}

		// Test SKA coin type (SKA1)
		skaResult := w.RelayFeeForCoinType(context.Background(), cointype.CoinType(1))
		if skaResult.String() != skaFee.String() {
			t.Errorf("RelayFeeForCoinType(SKA1) = %s, expected %s", skaResult.String(), skaFee.String())
		}

		// Test another SKA coin type (SKA2)
		ska2Result := w.RelayFeeForCoinType(context.Background(), cointype.CoinType(2))
		if ska2Result.String() != skaFee.String() {
			t.Errorf("RelayFeeForCoinType(SKA2) = %s, expected %s", ska2Result.String(), skaFee.String())
		}

		t.Logf("✅ RelayFeeForCoinType: VAR=%s, SKA1=%s, SKA2=%s",
			varResult.String(), skaResult.String(), ska2Result.String())
	})

	t.Run("Test Fee Independence", func(t *testing.T) {
		// Set different fees for VAR and SKA
		varFee := dcrutil.Amount(20000)
		skaFee := cointype.SKAAmountFromInt64(500)
		w.SetRelayFee(varFee)
		w.SetSKARelayFee(skaFee)

		// Verify they are independent (compare using int64 conversion)
		skaFeeInt64, _ := w.SKARelayFee().Int64()
		if int64(w.RelayFee()) == skaFeeInt64 {
			t.Error("VAR and SKA fees should be independent")
		}

		// Change VAR fee, verify SKA fee unchanged
		w.SetRelayFee(dcrutil.Amount(25000))
		if w.SKARelayFee().String() != skaFee.String() {
			t.Error("SKA fee should not change when VAR fee changes")
		}

		// Change SKA fee, verify VAR fee unchanged
		w.SetSKARelayFee(cointype.SKAAmountFromInt64(600))
		if w.RelayFee() != dcrutil.Amount(25000) {
			t.Error("VAR fee should not change when SKA fee changes")
		}

		t.Logf("✅ Fee independence verified: VAR=%d, SKA=%s",
			w.RelayFee(), w.SKARelayFee().String())
	})
}

// TestWalletFeeInitialization tests that wallet fees are properly initialized
// from chain parameters during wallet creation.
func TestWalletFeeInitialization(t *testing.T) {
	t.Log("=== Testing Wallet Fee Initialization from Chain Parameters ===")

	t.Run("Test initialization with SKA fee configured", func(t *testing.T) {
		chainParams := &chaincfg.Params{
			SKACoins: map[cointype.CoinType]*chaincfg.SKACoinConfig{
				1: {
					Active:           true,
					MinRelayTxFee:    big.NewInt(1500), // 1500 atoms/KB for SKA
					MaxFeeMultiplier: 2500,
				},
			},
		}

		w := &Wallet{
			chainParams: chainParams,
		}

		// Simulate Open() initialization
		configRelayFee := dcrutil.Amount(8000)
		w.relayFee = configRelayFee
		w.manualFees = make(map[cointype.CoinType]*cointype.SKAAmount)
		w.staticFees = make(map[cointype.CoinType]*cointype.SKAAmount)
		w.staticFees[cointype.CoinTypeVAR] = skaAmountPtr(cointype.SKAAmountFromInt64(int64(configRelayFee)))
		for ct, config := range w.chainParams.SKACoins {
			if config != nil && config.Active && config.MinRelayTxFee != nil {
				w.staticFees[ct] = skaAmountPtr(cointype.NewSKAAmount(config.MinRelayTxFee))
			}
		}

		if w.RelayFee() != configRelayFee {
			t.Errorf("VAR fee should be initialized to config value %d, got %d",
				configRelayFee, w.RelayFee())
		}

		expectedSKAFee := cointype.SKAAmountFromInt64(1500)
		if w.SKARelayFee().String() != expectedSKAFee.String() {
			t.Errorf("SKA fee should be initialized to chain param value %s, got %s",
				expectedSKAFee.String(), w.SKARelayFee().String())
		}

		t.Logf("✅ Initialized with SKA param: VAR=%d, SKA=%s",
			w.RelayFee(), w.SKARelayFee().String())
	})

	t.Run("Test initialization without SKA fee configured", func(t *testing.T) {
		chainParams := &chaincfg.Params{
			SKACoins: map[cointype.CoinType]*chaincfg.SKACoinConfig{
				1: {
					Active:        true,
					MinRelayTxFee: nil, // No SKA fee configured
				},
			},
		}

		w := &Wallet{
			chainParams: chainParams,
		}

		// Simulate Open() initialization
		configRelayFee := dcrutil.Amount(9000)
		w.relayFee = configRelayFee
		w.manualFees = make(map[cointype.CoinType]*cointype.SKAAmount)
		w.staticFees = make(map[cointype.CoinType]*cointype.SKAAmount)
		w.staticFees[cointype.CoinTypeVAR] = skaAmountPtr(cointype.SKAAmountFromInt64(int64(configRelayFee)))
		// No SKA fee configured, so fallback to VAR fee for SKA coins
		w.staticFees[cointype.CoinType(1)] = skaAmountPtr(cointype.SKAAmountFromInt64(int64(configRelayFee)))

		if w.RelayFee() != configRelayFee {
			t.Errorf("VAR fee should be initialized to config value %d, got %d",
				configRelayFee, w.RelayFee())
		}

		expectedSKAFee := cointype.SKAAmountFromInt64(int64(configRelayFee))
		if w.SKARelayFee().String() != expectedSKAFee.String() {
			t.Errorf("SKA fee should fallback to config value %s, got %s",
				expectedSKAFee.String(), w.SKARelayFee().String())
		}

		t.Logf("✅ Initialized without SKA param (fallback): VAR=%d, SKA=%s",
			w.RelayFee(), w.SKARelayFee().String())
	})
}

// TestCoinTypeFeeIntegrationScenarios tests real-world scenarios of
// coin-type-aware fee management.
func TestCoinTypeFeeIntegrationScenarios(t *testing.T) {
	t.Log("=== Testing Real-World Fee Management Scenarios ===")

	chainParams := &chaincfg.Params{
		SKACoins: map[cointype.CoinType]*chaincfg.SKACoinConfig{
			1: {
				Active:           true,
				MinRelayTxFee:    big.NewInt(1000), // SKA has lower fees than VAR
				MaxFeeMultiplier: 2500,
			},
		},
	}

	w := &Wallet{
		chainParams: chainParams,
		relayFee:    dcrutil.Amount(10000), // VAR: 10000 atoms/KB
	}

	// Initialize per-cointype fee maps with SKAAmount
	w.manualFees = make(map[cointype.CoinType]*cointype.SKAAmount)
	w.staticFees = make(map[cointype.CoinType]*cointype.SKAAmount)
	w.staticFees[cointype.CoinTypeVAR] = skaAmountPtr(cointype.SKAAmountFromInt64(10000))
	w.staticFees[cointype.CoinType(1)] = skaAmountPtr(cointype.SKAAmountFromInt64(1000)) // SKA: 1000 atoms/KB
	w.staticFees[cointype.CoinType(2)] = skaAmountPtr(cointype.SKAAmountFromInt64(1000))
	w.staticFees[cointype.CoinType(255)] = skaAmountPtr(cointype.SKAAmountFromInt64(1000))

	t.Run("Scenario: User wants to check current fees", func(t *testing.T) {
		// User calls getwalletfee (no coin type = VAR default)
		varFee := w.RelayFeeForCoinType(context.Background(), cointype.CoinTypeVAR)

		// User calls getwalletfee 1 (for SKA1)
		skaFee := w.RelayFeeForCoinType(context.Background(), cointype.CoinType(1))

		t.Logf("Current fees: VAR=%s atoms/KB, SKA=%s atoms/KB", varFee.String(), skaFee.String())

		if varFee.Cmp(skaFee) <= 0 {
			t.Log("⚠️  Note: VAR fee is not higher than SKA fee in this test scenario")
		}

		t.Log("✅ Fee query scenario completed")
	})

	t.Run("Scenario: User adjusts fees for different coin types", func(t *testing.T) {
		// User sets VAR fee higher (settxfee 0.00015)
		newVARFee := dcrutil.Amount(15000)
		w.SetRelayFee(newVARFee)

		// User sets SKA fee lower (settxfee 0.00005 1)
		newSKAFee := cointype.SKAAmountFromInt64(500)
		w.SetSKARelayFee(newSKAFee)

		// Verify changes
		if w.RelayFee() != newVARFee {
			t.Errorf("VAR fee not updated correctly: expected %d, got %d",
				newVARFee, w.RelayFee())
		}

		expectedSKAFee := cointype.SKAAmountFromInt64(500)
		if w.SKARelayFee().String() != expectedSKAFee.String() {
			t.Errorf("SKA fee not updated correctly: expected %s, got %s",
				expectedSKAFee.String(), w.SKARelayFee().String())
		}

		t.Logf("✅ Fee adjustment scenario: VAR raised to %d, SKA lowered to %s",
			newVARFee, w.SKARelayFee().String())
	})

	t.Run("Scenario: Multiple SKA coin types can have different fees", func(t *testing.T) {
		// With per-coin configuration, each SKA coin type can have its own fee
		ska1Fee := w.RelayFeeForCoinType(context.Background(), cointype.CoinType(1))     // SKA1
		ska2Fee := w.RelayFeeForCoinType(context.Background(), cointype.CoinType(2))     // SKA2
		ska255Fee := w.RelayFeeForCoinType(context.Background(), cointype.CoinType(255)) // SKA255 (max)

		// SKA1 was modified by SetSKARelayFee earlier, so it may differ from others
		// This verifies per-coin fees work correctly
		t.Logf("✅ Multiple SKA types scenario: SKA1=%s, SKA2=%s, SKA255=%s",
			ska1Fee.String(), ska2Fee.String(), ska255Fee.String())

		// Verify each coin type returns its configured fee
		if ska1Fee.Cmp(cointype.SKAAmountFromInt64(500)) != 0 {
			t.Errorf("SKA1 should have fee 500 (set by SetSKARelayFee), got %s", ska1Fee.String())
		}
		if ska2Fee.Cmp(cointype.SKAAmountFromInt64(1000)) != 0 {
			t.Errorf("SKA2 should have fee 1000 (from staticFees), got %s", ska2Fee.String())
		}
		if ska255Fee.Cmp(cointype.SKAAmountFromInt64(1000)) != 0 {
			t.Errorf("SKA255 should have fee 1000 (from staticFees), got %s", ska255Fee.String())
		}
	})
}

// TestRelayFeeForCoinTypeExplicitZero verifies the post-2026-05-04 contract:
// a configured zero fee is treated as UNSET at every layer (manual, RPC,
// static) and the next fallback is consulted. This aligns with consensus
// reality — only SKA emission transactions are allowed to be zero-fee, and
// they construct fee=0 directly without consulting RelayFeeForCoinType.
//
// Earlier the test pinned the inverse "zero is legitimate" contract; that
// was wrong because a non-emission tx with zero fee is rejected by the node.
func TestRelayFeeForCoinTypeExplicitZero(t *testing.T) {
	chainParams := &chaincfg.Params{
		SKACoins: map[cointype.CoinType]*chaincfg.SKACoinConfig{
			1: {
				Active:           true,
				MinRelayTxFee:    big.NewInt(4000), // chainparams fallback the test EXPECTS to pick up
				MaxFeeMultiplier: 2500,
			},
		},
	}
	w := &Wallet{chainParams: chainParams}
	w.manualFees = make(map[cointype.CoinType]*cointype.SKAAmount)
	w.staticFees = make(map[cointype.CoinType]*cointype.SKAAmount)
	w.staticFees[cointype.CoinTypeVAR] = skaAmountPtr(cointype.Zero()) // explicitly zero (treated as unset)
	w.staticFees[cointype.CoinType(1)] = skaAmountPtr(cointype.Zero()) // explicitly zero (treated as unset)
	// SKA2 is intentionally absent from both staticFees and SKACoins, so
	// the late-activation chainparams path can't kick in either — the
	// "no fee configured" final-zero+log path is exercised.

	// queryDynamicFee with nil NetworkBackend fails; manual is unset.
	ctx := context.Background()

	// VAR: explicit-zero static fee falls through. No chainparams VAR config
	// either, so the final return is zero (with a log.Errorf).
	if got := w.RelayFeeForCoinType(ctx, cointype.CoinTypeVAR); !got.IsZero() {
		t.Errorf("VAR explicit-zero with no fallbacks: got %s, want 0", got.String())
	}

	// SKA1: explicit-zero static fee falls through to the chainparams
	// MinRelayTxFee=4000 — that's the late-activation path's only job.
	got := w.RelayFeeForCoinType(ctx, cointype.CoinType(1))
	wantBig := big.NewInt(4000)
	if got.IsZero() {
		t.Fatalf("SKA1 explicit-zero static fee: got 0, want chainparams MinRelayTxFee=%s", wantBig)
	}
	if v, err := got.Int64(); err != nil || v != 4000 {
		t.Errorf("SKA1 fallback: got %s, want 4000 (from chainparams.SKACoins[1].MinRelayTxFee)",
			got.String())
	}
}

// TestSetManualFeeVAROverflowRejected pins the M1 fix from the 2026-05-05
// review: SetManualFee(VAR, fee) must surface an error when fee exceeds int64
// rather than silently truncating into the legacy dcrutil.Amount-shaped
// relayFee field. Previously the conversion error was discarded with `_` and
// callers received nil with a corrupted relayFee.
func TestSetManualFeeVAROverflowRejected(t *testing.T) {
	chainParams := &chaincfg.Params{
		SKACoins: map[cointype.CoinType]*chaincfg.SKACoinConfig{
			1: {Active: true, MinRelayTxFee: big.NewInt(1000), MaxFeeMultiplier: 2500},
		},
	}
	w := &Wallet{chainParams: chainParams}
	w.manualFees = make(map[cointype.CoinType]*cointype.SKAAmount)
	w.staticFees = make(map[cointype.CoinType]*cointype.SKAAmount)
	w.staticFees[cointype.CoinTypeVAR] = skaAmountPtr(cointype.SKAAmountFromInt64(10000))
	originalRelayFee := dcrutil.Amount(10000)
	w.relayFee = originalRelayFee

	// 2^63 — does not fit in int64 (max int64 is 2^63 - 1).
	overflow := new(big.Int).Lsh(big.NewInt(1), 63)
	overflowAmt := cointype.NewSKAAmount(overflow)

	if err := w.SetManualFee(cointype.CoinTypeVAR, overflowAmt); err == nil {
		t.Fatalf("SetManualFee(VAR, 2^63) must error; got nil")
	}

	// State must be untouched on failure: neither manualFees nor relayFee
	// should be mutated by a rejected call.
	if _, ok := w.manualFees[cointype.CoinTypeVAR]; ok {
		t.Errorf("rejected SetManualFee must not persist VAR manual override")
	}
	if w.relayFee != originalRelayFee {
		t.Errorf("rejected SetManualFee must not mutate w.relayFee; got %d, want %d",
			w.relayFee, originalRelayFee)
	}

	// Sanity: a representable VAR fee succeeds and persists.
	ok := cointype.SKAAmountFromInt64(15000)
	if err := w.SetManualFee(cointype.CoinTypeVAR, ok); err != nil {
		t.Fatalf("SetManualFee(VAR, 15000) must succeed; got %v", err)
	}
	if got, present := w.manualFees[cointype.CoinTypeVAR]; !present || got == nil ||
		got.String() != "15000" {
		t.Errorf("manual VAR override not persisted; got %v", got)
	}
	if w.relayFee != dcrutil.Amount(15000) {
		t.Errorf("relayFee not updated; got %d, want 15000", w.relayFee)
	}

	// SKA path is big.Int-precision and accepts arbitrarily-large fees. The
	// same overflow value that VAR rejects must be accepted on SKA1 — the
	// int64 ceiling does not apply.
	if err := w.SetManualFee(cointype.CoinType(1), overflowAmt); err != nil {
		t.Fatalf("SetManualFee(SKA1, 2^63) must succeed; got %v", err)
	}
	if got, present := w.manualFees[cointype.CoinType(1)]; !present || got == nil ||
		got.BigInt().Cmp(overflow) != 0 {
		t.Errorf("manual SKA1 override not persisted at full precision; got %v", got)
	}
}
