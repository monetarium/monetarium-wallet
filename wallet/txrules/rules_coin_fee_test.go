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
			name: "All SKA1 outputs",
			outputs: []*wire.TxOut{
				{CoinType: cointype.CoinType(1), Value: 1000},
				{CoinType: cointype.CoinType(1), Value: 2000},
			},
			expected: cointype.CoinType(1),
		},
		{
			name: "All SKA2 outputs",
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

// TestSKAFeeDesign verifies that SKA transactions pay fees in their own coin
// type via the big.Int FeeForSerializeSizeSKA path.
func TestSKAFeeDesign(t *testing.T) {
	relayFee := cointype.SKAAmountFromInt64(10000)
	txSize := 250

	// SKA fee uses big.Int math; should match the same arithmetic as VAR
	// when relayFee fits in int64.
	skaFee := FeeForSerializeSizeSKA(relayFee, txSize)
	expectedAtoms := int64(10000) * int64(txSize) / 1000
	got, _ := skaFee.Int64()
	if got != expectedAtoms {
		t.Errorf("FeeForSerializeSizeSKA: expected %d atoms, got %d", expectedAtoms, got)
	}

	// VAR fee uses int64 math.
	varFee := FeeForSerializeSize(dcrutil.Amount(10000), txSize)
	if int64(varFee) != expectedAtoms {
		t.Errorf("FeeForSerializeSize VAR: expected %d atoms, got %d", expectedAtoms, varFee)
	}

	t.Logf("Fee calculation: VAR=%d atoms, SKA=%d atoms", varFee, got)
}
