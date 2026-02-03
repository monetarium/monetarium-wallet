// Copyright (c) 2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txrules

import (
	"math/big"
	"testing"

	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/dcrutil"
	"github.com/monetarium/monetarium-node/wire"
)

// TestFeeForSerializeSizeSKA tests SKA fee calculation with big.Int.
func TestFeeForSerializeSizeSKA(t *testing.T) {
	txSize := 250 // 250 byte transaction

	tests := []struct {
		name        string
		feePerKb    *big.Int
		expectedFee *big.Int
	}{
		{
			name:        "Small SKA fee rate",
			feePerKb:    big.NewInt(1000), // 1000 atoms/KB
			expectedFee: big.NewInt(250),  // 1000 * 250 / 1000 = 250
		},
		{
			name:        "Large SKA fee rate (4 SKA/KB)",
			feePerKb:    big.NewInt(4000000000000000000), // 4e18 atoms/KB
			expectedFee: big.NewInt(1000000000000000000), // 4e18 * 250 / 1000 = 1e18
		},
		{
			name:        "Zero size returns min fee",
			feePerKb:    big.NewInt(1000),
			expectedFee: big.NewInt(1000), // Returns feePerKb for tiny transactions
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			skaFeePerKb := cointype.NewSKAAmount(test.feePerKb)

			// For zero size test, use 0
			size := txSize
			if test.name == "Zero size returns min fee" {
				size = 0
			}

			actualFee := FeeForSerializeSizeSKA(skaFeePerKb, size)

			if actualFee.BigInt().Cmp(test.expectedFee) != 0 {
				t.Errorf("Expected fee %s, got %s", test.expectedFee.String(), actualFee.BigInt().String())
			}

			t.Logf("%s: fee = %s atoms", test.name, actualFee.BigInt().String())
		})
	}
}

// TestFeeForSerializeSizeVAR tests VAR fee calculation (int64-based).
func TestFeeForSerializeSizeVAR(t *testing.T) {
	varRelayFee := dcrutil.Amount(10000) // 10000 atoms/KB
	txSize := 250                        // 250 byte transaction

	actualFee := FeeForSerializeSize(varRelayFee, txSize)
	expectedFee := dcrutil.Amount(2500) // 10000 * 250 / 1000 = 2500

	if actualFee != expectedFee {
		t.Errorf("VAR fee: expected %d, got %d", expectedFee, actualFee)
	}
}

// TestGetCoinTypeFromOutputs tests coin type detection from transaction outputs.
func TestGetCoinTypeFromOutputs(t *testing.T) {
	tests := []struct {
		name     string
		outputs  []*wire.TxOut
		expected cointype.CoinType
	}{
		{
			name: "All VAR outputs",
			outputs: []*wire.TxOut{
				{CoinType: cointype.CoinTypeVAR, Value: 1000},
				{CoinType: cointype.CoinTypeVAR, Value: 2000},
			},
			expected: cointype.CoinTypeVAR,
		},
		{
			name: "All SKA-1 outputs",
			outputs: []*wire.TxOut{
				{CoinType: cointype.CoinType(1), Value: 1000},
				{CoinType: cointype.CoinType(1), Value: 2000},
			},
			expected: cointype.CoinType(1),
		},
		{
			name: "All SKA-2 outputs",
			outputs: []*wire.TxOut{
				{CoinType: cointype.CoinType(2), Value: 1000},
				{CoinType: cointype.CoinType(2), Value: 2000},
			},
			expected: cointype.CoinType(2),
		},
		{
			name: "Single output",
			outputs: []*wire.TxOut{
				{CoinType: cointype.CoinType(1), Value: 1000},
			},
			expected: cointype.CoinType(1),
		},
		{
			name:     "Empty outputs",
			outputs:  []*wire.TxOut{},
			expected: cointype.CoinTypeVAR, // Default to VAR when no outputs
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := GetCoinTypeFromOutputs(test.outputs)
			if actual != test.expected {
				t.Errorf("Expected coin type %d, got %d", test.expected, actual)
			}
		})
	}
}

// TestSKAFeeDesign verifies that SKA transactions pay fees in their own coin type.
func TestSKAFeeDesign(t *testing.T) {
	relayFee := dcrutil.Amount(10000)
	txSize := 250

	// Test that SKA transactions pay fees in their own coin type
	skaFee := FeeForSerializeSizeDualCoin(relayFee, txSize, cointype.CoinType(1))

	// Should be same calculation as VAR (fee paid in SKA coins)
	expectedFee := relayFee * dcrutil.Amount(txSize) / 1000
	if expectedFee == 0 && relayFee > 0 {
		expectedFee = relayFee
	}

	if skaFee != expectedFee {
		t.Errorf("SKA transactions should pay fees in SKA coins using same calculation as VAR, expected %d, got %d", expectedFee, skaFee)
	}

	// VAR should have same calculation
	varFee := FeeForSerializeSizeDualCoin(relayFee, txSize, cointype.CoinTypeVAR)
	if varFee != expectedFee {
		t.Errorf("VAR and SKA should use same fee calculation, VAR=%d, SKA=%d",
			varFee, skaFee)
	}

	t.Logf("Fixed fee calculation: VAR=%d atoms, SKA=%d atoms (no longer zero)", varFee, skaFee)
}
