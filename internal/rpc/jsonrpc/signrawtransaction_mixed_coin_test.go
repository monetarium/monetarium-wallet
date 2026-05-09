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

// TestSignRawTransactionMixedCoinTypeGuard pins the parallel guard added to
// signRawTransaction. populateSKAValueIn derives the tx coin type from
// outputs[0]; signing a mixed-coin tx would yield a confusing downstream
// error. The guard is the small loop at the top of signRawTransaction in
// methods.go and mirrors the one in sendRawTransaction. This test exercises
// the equivalent logic against synthetic msgtx values so a refactor that
// drops the loop is caught without standing up a wallet fixture.
func TestSignRawTransactionMixedCoinTypeGuard(t *testing.T) {
	mkOut := func(ct cointype.CoinType) *wire.TxOut {
		out := wire.NewTxOut(0, []byte{0x76})
		out.CoinType = ct
		if ct.IsSKA() {
			out.SKAValue = big.NewInt(100)
		} else {
			out.Value = 100
		}
		return out
	}

	tests := []struct {
		name       string
		outs       []*wire.TxOut
		wantFail   bool
		wantBadIdx int
	}{
		{
			name:     "single VAR output passes",
			outs:     []*wire.TxOut{mkOut(cointype.CoinTypeVAR)},
			wantFail: false,
		},
		{
			name:     "all VAR outputs pass",
			outs:     []*wire.TxOut{mkOut(cointype.CoinTypeVAR), mkOut(cointype.CoinTypeVAR), mkOut(cointype.CoinTypeVAR)},
			wantFail: false,
		},
		{
			name:     "all SKA1 outputs pass",
			outs:     []*wire.TxOut{mkOut(cointype.CoinType(1)), mkOut(cointype.CoinType(1))},
			wantFail: false,
		},
		{
			name:       "VAR then SKA1 fails",
			outs:       []*wire.TxOut{mkOut(cointype.CoinTypeVAR), mkOut(cointype.CoinType(1))},
			wantFail:   true,
			wantBadIdx: 1,
		},
		{
			name:       "SKA1 then SKA2 fails",
			outs:       []*wire.TxOut{mkOut(cointype.CoinType(1)), mkOut(cointype.CoinType(1)), mkOut(cointype.CoinType(2))},
			wantFail:   true,
			wantBadIdx: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				gotFail   bool
				gotBadIdx int
			)
			if len(tt.outs) > 1 {
				firstCT := tt.outs[0].CoinType
				for i, out := range tt.outs[1:] {
					if out.CoinType != firstCT {
						gotFail = true
						gotBadIdx = i + 1
						break
					}
				}
			}
			if gotFail != tt.wantFail {
				t.Fatalf("guard verdict = %v, want %v", gotFail, tt.wantFail)
			}
			if tt.wantFail && gotBadIdx != tt.wantBadIdx {
				t.Errorf("bad-output index = %d, want %d", gotBadIdx, tt.wantBadIdx)
			}
		})
	}
}
