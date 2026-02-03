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

// TestCoinTypeFeeManagementMethods tests the new SKA fee management methods
// added to the Wallet struct.
func TestCoinTypeFeeManagementMethods(t *testing.T) {
	t.Log("=== Testing Coin-Type-Aware Fee Management Methods ===")

	// Create test chain parameters with per-coin SKA fee configuration
	chainParams := &chaincfg.Params{
		SKACoins: map[cointype.CoinType]*chaincfg.SKACoinConfig{
			1: {
				Active:           true,
				MinRelayTxFee:    big.NewInt(1000), // 1000 atoms/KB for SKA-1
				MaxFeeMultiplier: 2500,
			},
			2: {
				Active:           true,
				MinRelayTxFee:    big.NewInt(1000), // 1000 atoms/KB for SKA-2
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
	w.staticFees = make(map[cointype.CoinType]cointype.SKAAmount)
	w.staticFees[cointype.CoinTypeVAR] = cointype.SKAAmountFromInt64(int64(varRelayFee))
	for ct, config := range w.chainParams.SKACoins {
		if config != nil && config.Active && config.MinRelayTxFee != nil {
			w.staticFees[ct] = cointype.NewSKAAmount(config.MinRelayTxFee)
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

		// Test SKA coin type (SKA-1)
		skaResult := w.RelayFeeForCoinType(context.Background(), cointype.CoinType(1))
		if skaResult.String() != skaFee.String() {
			t.Errorf("RelayFeeForCoinType(SKA-1) = %s, expected %s", skaResult.String(), skaFee.String())
		}

		// Test another SKA coin type (SKA-2)
		ska2Result := w.RelayFeeForCoinType(context.Background(), cointype.CoinType(2))
		if ska2Result.String() != skaFee.String() {
			t.Errorf("RelayFeeForCoinType(SKA-2) = %s, expected %s", ska2Result.String(), skaFee.String())
		}

		t.Logf("✅ RelayFeeForCoinType: VAR=%s, SKA-1=%s, SKA-2=%s",
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
		w.staticFees = make(map[cointype.CoinType]cointype.SKAAmount)
		w.staticFees[cointype.CoinTypeVAR] = cointype.SKAAmountFromInt64(int64(configRelayFee))
		for ct, config := range w.chainParams.SKACoins {
			if config != nil && config.Active && config.MinRelayTxFee != nil {
				w.staticFees[ct] = cointype.NewSKAAmount(config.MinRelayTxFee)
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
		w.staticFees = make(map[cointype.CoinType]cointype.SKAAmount)
		w.staticFees[cointype.CoinTypeVAR] = cointype.SKAAmountFromInt64(int64(configRelayFee))
		// No SKA fee configured, so fallback to VAR fee for SKA coins
		w.staticFees[cointype.CoinType(1)] = cointype.SKAAmountFromInt64(int64(configRelayFee))

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
	w.staticFees = make(map[cointype.CoinType]cointype.SKAAmount)
	w.staticFees[cointype.CoinTypeVAR] = cointype.SKAAmountFromInt64(10000)
	w.staticFees[cointype.CoinType(1)] = cointype.SKAAmountFromInt64(1000) // SKA: 1000 atoms/KB
	w.staticFees[cointype.CoinType(2)] = cointype.SKAAmountFromInt64(1000)
	w.staticFees[cointype.CoinType(255)] = cointype.SKAAmountFromInt64(1000)

	t.Run("Scenario: User wants to check current fees", func(t *testing.T) {
		// User calls getwalletfee (no coin type = VAR default)
		varFee := w.RelayFeeForCoinType(context.Background(), cointype.CoinTypeVAR)

		// User calls getwalletfee 1 (for SKA-1)
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
		ska1Fee := w.RelayFeeForCoinType(context.Background(), cointype.CoinType(1))     // SKA-1
		ska2Fee := w.RelayFeeForCoinType(context.Background(), cointype.CoinType(2))     // SKA-2
		ska255Fee := w.RelayFeeForCoinType(context.Background(), cointype.CoinType(255)) // SKA-255 (max)

		// SKA-1 was modified by SetSKARelayFee earlier, so it may differ from others
		// This verifies per-coin fees work correctly
		t.Logf("✅ Multiple SKA types scenario: SKA-1=%s, SKA-2=%s, SKA-255=%s",
			ska1Fee.String(), ska2Fee.String(), ska255Fee.String())

		// Verify each coin type returns its configured fee
		if ska1Fee.Cmp(cointype.SKAAmountFromInt64(500)) != 0 {
			t.Errorf("SKA-1 should have fee 500 (set by SetSKARelayFee), got %s", ska1Fee.String())
		}
		if ska2Fee.Cmp(cointype.SKAAmountFromInt64(1000)) != 0 {
			t.Errorf("SKA-2 should have fee 1000 (from staticFees), got %s", ska2Fee.String())
		}
		if ska255Fee.Cmp(cointype.SKAAmountFromInt64(1000)) != 0 {
			t.Errorf("SKA-255 should have fee 1000 (from staticFees), got %s", ska255Fee.String())
		}
	})
}
