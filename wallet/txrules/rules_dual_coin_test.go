// Copyright (c) 2024 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txrules_test

import (
	"math/big"
	"testing"

	"github.com/monetarium/monetarium-wallet/wallet/txrules"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/wire"
)

// TestCheckOutputSKAMinimum tests the 30-atom minimum for SKA outputs
func TestCheckOutputSKAMinimum(t *testing.T) {
	relayFeePerKb := cointype.SKAAmountFromInt64(1e4)
	pkScript := make([]byte, 25) // P2PKH script

	testCases := []struct {
		name        string
		output      *wire.TxOut
		expectError bool
		description string
	}{
		{
			name: "SKA output with 30 atoms (minimum)",
			output: &wire.TxOut{
				Value:    0,
				CoinType: cointype.CoinType(1),
				PkScript: pkScript,
				SKAValue: big.NewInt(30),
			},
			expectError: false,
			description: "SKA output at minimum should be valid",
		},
		{
			name: "SKA output with 31 atoms",
			output: &wire.TxOut{
				Value:    0,
				CoinType: cointype.CoinType(1),
				PkScript: pkScript,
				SKAValue: big.NewInt(31),
			},
			expectError: false,
			description: "SKA output above minimum should be valid",
		},
		{
			name: "SKA output with 29 atoms (below minimum)",
			output: &wire.TxOut{
				Value:    0,
				CoinType: cointype.CoinType(1),
				PkScript: pkScript,
				SKAValue: big.NewInt(29),
			},
			expectError: true,
			description: "SKA output below minimum should fail",
		},
		{
			name: "SKA output with zero SKAValue",
			output: &wire.TxOut{
				Value:    0,
				CoinType: cointype.CoinType(1),
				PkScript: pkScript,
				SKAValue: big.NewInt(0),
			},
			expectError: true,
			description: "SKA output with zero should fail",
		},
		{
			name: "SKA output with nil SKAValue",
			output: &wire.TxOut{
				Value:    0,
				CoinType: cointype.CoinType(1),
				PkScript: pkScript,
				SKAValue: nil,
			},
			expectError: true,
			description: "SKA output with nil should fail",
		},
		{
			name: "SKA output with negative SKAValue",
			output: &wire.TxOut{
				Value:    0,
				CoinType: cointype.CoinType(1),
				PkScript: pkScript,
				SKAValue: big.NewInt(-1),
			},
			expectError: true,
			description: "SKA output with negative should fail",
		},
		{
			name: "SKA output with large value",
			output: &wire.TxOut{
				Value:    0,
				CoinType: cointype.CoinType(1),
				PkScript: pkScript,
				SKAValue: big.NewInt(1000000),
			},
			expectError: false,
			description: "SKA output with large value should be valid",
		},
		{
			name: "VAR output with normal amount",
			output: &wire.TxOut{
				Value:    1e6,
				CoinType: cointype.CoinTypeVAR,
				PkScript: pkScript,
			},
			expectError: false,
			description: "VAR output with normal amount should be valid",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := txrules.CheckOutput(tc.output, relayFeePerKb)
			if tc.expectError && err == nil {
				t.Errorf("CheckOutput should have failed: %s", tc.description)
			}
			if !tc.expectError && err != nil {
				t.Errorf("CheckOutput should have succeeded: %s, got error: %v", tc.description, err)
			}
		})
	}
}
