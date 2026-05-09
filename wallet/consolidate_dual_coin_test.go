// Copyright (c) 2025 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"math/big"
	"testing"

	"github.com/monetarium/monetarium-node/chaincfg"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/wire"
	"github.com/monetarium/monetarium-wallet/errors"
	"github.com/monetarium/monetarium-wallet/wallet/txrules"
)

// TestConsolidateCoinTypeValidation verifies cointype values consolidation
// can accept. Uses the canonical IsValid / IsSKA helpers rather than
// hand-rolling range checks.
func TestConsolidateCoinTypeValidation(t *testing.T) {
	tests := []struct {
		name      string
		coinType  cointype.CoinType
		valid     bool
		isSKA     bool
	}{
		{"VAR", cointype.CoinTypeVAR, true, false},
		{"SKA1", cointype.CoinType(1), true, true},
		{"SKA2", cointype.CoinType(2), true, true},
		{"SKA100", cointype.CoinType(100), true, true},
		{"SKA255 max", cointype.CoinType(255), true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.coinType.IsValid() != tt.valid {
				t.Errorf("IsValid() = %v, want %v", tt.coinType.IsValid(), tt.valid)
			}
			if tt.coinType.IsSKA() != tt.isSKA {
				t.Errorf("IsSKA() = %v, want %v", tt.coinType.IsSKA(), tt.isSKA)
			}
		})
	}
}

// TestConsolidateOutputDustClassification pins the MED-6 fix from the
// 2026-05-04 review: a sub-dust SKA consolidation output must surface as
// errors.Policy (the operator HAS funds, just below the dust threshold),
// not errors.InsufficientBalance ("no funds at all"). compressWalletInternal
// now propagates the Policy classification from txrules.CheckOutput rather
// than re-tagging.
//
// Tests txrules.CheckOutput directly because compressWalletInternal needs a
// loaded wallet harness. CheckOutput is the single source of truth that
// produces the Policy error consolidate now propagates verbatim.
func TestConsolidateOutputDustClassification(t *testing.T) {
	chainParams := chaincfg.SimNetParams()
	// Plausible per-kB relay fee for testing dust thresholds; the exact
	// value is not load-bearing — CheckOutput compares the SKA output
	// against MinSKADustAmount (constant) and the VAR output against
	// the per-kB-derived dust amount.
	relayFee := cointype.SKAAmountFromInt64(100000)

	t.Run("sub-dust SKA output yields Policy", func(t *testing.T) {
		// MinSKADustAmount is 30 atoms; 1 atom is well below it.
		out := &wire.TxOut{
			Value:    0,
			SKAValue: big.NewInt(1),
			CoinType: cointype.CoinType(1),
			PkScript: simnetP2PKHScript(t, chainParams),
		}
		err := txrules.CheckOutput(out, relayFee)
		if err == nil {
			t.Fatal("expected dust error for 1-atom SKA output, got nil")
		}
		if !errors.Is(err, errors.Policy) {
			t.Errorf("error kind = %v, want errors.Policy (operator has funds, "+
				"just below dust threshold; not InsufficientBalance)", err)
		}
	})

	t.Run("above-dust SKA output passes", func(t *testing.T) {
		// 1000 atoms is comfortably above the 30-atom MinSKADustAmount.
		out := &wire.TxOut{
			Value:    0,
			SKAValue: big.NewInt(1000),
			CoinType: cointype.CoinType(1),
			PkScript: simnetP2PKHScript(t, chainParams),
		}
		if err := txrules.CheckOutput(out, relayFee); err != nil {
			t.Errorf("unexpected error for 1000-atom SKA output: %v", err)
		}
	})

	t.Run("VAR dust output yields Policy", func(t *testing.T) {
		// At simnet relay fee 1e5 atoms/KB the per-output dust threshold is
		// well above 1 atom; pick a value clearly below it.
		out := &wire.TxOut{
			Value:    1,
			CoinType: cointype.CoinTypeVAR,
			PkScript: simnetP2PKHScript(t, chainParams),
		}
		err := txrules.CheckOutput(out, relayFee)
		if err == nil {
			t.Fatal("expected dust error for 1-atom VAR output, got nil")
		}
		if !errors.Is(err, errors.Policy) {
			t.Errorf("error kind = %v, want errors.Policy", err)
		}
	})
}

// simnetP2PKHScript returns a minimal valid P2PKH script for testing
// CheckOutput against a real script type (the function rejects non-standard
// scripts as dust regardless of value).
func simnetP2PKHScript(t *testing.T, params *chaincfg.Params) []byte {
	t.Helper()
	// OP_DUP OP_HASH160 <20-byte pubkey hash> OP_EQUALVERIFY OP_CHECKSIG
	return []byte{
		0x76, 0xa9, 0x14,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x88, 0xac,
	}
}
