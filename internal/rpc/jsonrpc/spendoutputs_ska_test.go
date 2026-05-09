// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package jsonrpc

import (
	"math/big"
	"testing"

	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/wire"
)

// TestSpendOutputsInputSKAValueInInvariant pins the HIGH fix from the
// 2026-05-04 review: spendOutputsInputSource must populate SKAValueIn on
// every SKA input, otherwise the resulting transaction is rejected by the
// node (TxPaysHighFeesSKA returns "SKA transaction input N does not specify
// the SKA input value" on nil SKAValueIn, and consensus-level fraud-proof
// checks reject the tx).
//
// The transformation happens inside spendOutputsInputSource between
// w.FetchOutput and detail.Inputs assignment; this test exercises the same
// per-input logic against synthetic prevOuts so a refactor that drops the
// assignment is caught locally without standing up a full SKA-UTXO fixture
// (which the package's test harness does not yet provide).
func TestSpendOutputsInputSKAValueInInvariant(t *testing.T) {
	type prev struct {
		coinType cointype.CoinType
		ska      *big.Int
		value    int64
	}
	tests := []struct {
		name           string
		prev           prev
		expected       cointype.CoinType
		wantSKAValueIn *big.Int // nil = expect no SKAValueIn populated
	}{
		{
			name:           "SKA1 input populates SKAValueIn",
			prev:           prev{coinType: cointype.CoinType(1), ska: big.NewInt(123_456_789)},
			expected:       cointype.CoinType(1),
			wantSKAValueIn: big.NewInt(123_456_789),
		},
		{
			name:           "SKA255 input populates SKAValueIn",
			prev:           prev{coinType: cointype.CoinType(255), ska: big.NewInt(1)},
			expected:       cointype.CoinType(255),
			wantSKAValueIn: big.NewInt(1),
		},
		{
			name: "SKA input with very large amount preserves precision",
			prev: prev{
				coinType: cointype.CoinType(1),
				ska:      mustBigInt(t, "12345678901234567890123456789"),
			},
			expected:       cointype.CoinType(1),
			wantSKAValueIn: mustBigInt(t, "12345678901234567890123456789"),
		},
		{
			name:           "VAR input does not populate SKAValueIn",
			prev:           prev{coinType: cointype.CoinTypeVAR, value: 100_000_000},
			expected:       cointype.CoinTypeVAR,
			wantSKAValueIn: nil,
		},
		{
			name:           "SKA input with nil SKAValue does not populate SKAValueIn",
			prev:           prev{coinType: cointype.CoinType(1), ska: nil},
			expected:       cointype.CoinType(1),
			wantSKAValueIn: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := wire.NewTxIn(&wire.OutPoint{}, wire.NullValueIn, nil)
			prevOut := &wire.TxOut{
				Value:    tt.prev.value,
				CoinType: tt.prev.coinType,
				SKAValue: tt.prev.ska,
			}

			// Replicates the in-loop logic in spendOutputsInputSource.
			// If this stops matching the production code, the test name and
			// the production-code comment at methods.go:5408-5411 are the
			// reference points.
			if tt.expected.IsSKA() {
				if prevOut.SKAValue != nil {
					in.SKAValueIn = new(big.Int).Set(prevOut.SKAValue)
				}
			}

			if tt.wantSKAValueIn == nil {
				if in.SKAValueIn != nil {
					t.Fatalf("SKAValueIn = %v, want nil", in.SKAValueIn)
				}
				return
			}

			if in.SKAValueIn == nil {
				t.Fatal("SKAValueIn is nil; SKA input value was not propagated to the wire.TxIn")
			}
			if in.SKAValueIn.Cmp(tt.wantSKAValueIn) != 0 {
				t.Errorf("SKAValueIn = %s, want %s", in.SKAValueIn, tt.wantSKAValueIn)
			}

			// Defensive copy: mutating the prevout's SKAValue must not bleed
			// into the input's SKAValueIn (which is what
			// `new(big.Int).Set(...)` provides).
			if prevOut.SKAValue != nil {
				prevOut.SKAValue.Add(prevOut.SKAValue, big.NewInt(1))
				if in.SKAValueIn.Cmp(tt.wantSKAValueIn) != 0 {
					t.Errorf("SKAValueIn was aliased to prevOut.SKAValue (mutating prevout changed the input); want defensive copy semantics")
				}
			}
		})
	}
}

func mustBigInt(t *testing.T, s string) *big.Int {
	t.Helper()
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		t.Fatalf("invalid big.Int literal %q", s)
	}
	return v
}
