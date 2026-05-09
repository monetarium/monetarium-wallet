// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package jsonrpc

import (
	"testing"

	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/dcrutil"
)

// TestFundRawTxFeeRateOverride pins the contract that drives fee-rate
// selection in fundRawTransaction. The historical bug (fixed alongside this
// test) was that the per-kB VAR relay fee was wrapped as if it were SKA atoms
// and passed as an override for SKA-output transactions, underpaying by the
// AtomsPerCoin ratio. The override must be zero for SKA outputs so the wallet
// falls through to RelayFeeForCoinType, and must carry the VAR rate verbatim
// for VAR outputs.
func TestFundRawTxFeeRateOverride(t *testing.T) {
	const varFee = dcrutil.Amount(100000) // 1e5 atoms / kB

	tests := []struct {
		name           string
		coinType       cointype.CoinType
		fee            dcrutil.Amount
		wantZero       bool
		wantBigIntVal  int64
	}{
		{
			name:          "VAR output uses VAR fee verbatim",
			coinType:      cointype.CoinTypeVAR,
			fee:           varFee,
			wantZero:      false,
			wantBigIntVal: int64(varFee),
		},
		{
			name:     "SKA1 output yields zero override",
			coinType: cointype.CoinType(1),
			fee:      varFee,
			wantZero: true,
		},
		{
			name:     "SKA255 output yields zero override",
			coinType: cointype.CoinType(255),
			fee:      varFee,
			wantZero: true,
		},
		{
			name:          "VAR output with zero fee still passes override",
			coinType:      cointype.CoinTypeVAR,
			fee:           0,
			wantZero:      true,
			wantBigIntVal: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fundRawTxFeeRateOverride(tt.coinType, tt.fee)

			if tt.wantZero {
				if !got.IsZero() {
					t.Errorf("override = %s, want zero", got.String())
				}
				return
			}

			if got.IsZero() {
				t.Fatalf("override unexpectedly zero for VAR fee %d", tt.fee)
			}
			if v, err := got.Int64(); err != nil {
				t.Fatalf("override.Int64(): %v", err)
			} else if v != tt.wantBigIntVal {
				t.Errorf("override.Int64() = %d, want %d", v, tt.wantBigIntVal)
			}
		})
	}
}
