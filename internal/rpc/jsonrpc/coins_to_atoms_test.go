// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package jsonrpc

import (
	"math/big"
	"testing"

	"github.com/monetarium/monetarium-node/cointype"
)

// TestCoinsToAtomsBigRejectsNegativeAndOverflow guards two regressions:
//
//  1. Negative amounts must be rejected by the parser, so callers that
//     forget to pre-validate (or whose pre-validation has gaps, e.g. leading
//     whitespace) cannot drive negative big.Int values into transaction
//     construction.
//  2. Fractional parts longer than the coin's atom precision must be
//     rejected, not silently truncated. The previous behavior rounded toward
//     zero by up to one atom for over-precise SKA amounts.
func TestCoinsToAtomsBigRejectsNegativeAndOverflow(t *testing.T) {
	skaPrecision := cointype.GetAtomsPerSKACoin() // 1e18 → 18 decimals
	varPrecision := big.NewInt(cointype.AtomsPerVAR)

	tests := []struct {
		name        string
		amount      string
		atomsPer    *big.Int
		wantErr     bool
		wantAtoms   string // decimal string; only checked when wantErr=false
	}{
		{"valid positive SKA", "1.5", skaPrecision, false, "1500000000000000000"},
		{"valid zero SKA", "0", skaPrecision, false, "0"},
		{"valid SKA at full precision", "1.123456789012345678", skaPrecision, false, "1123456789012345678"},
		{"valid VAR", "1.23", varPrecision, false, "123000000"},

		{"negative integer rejected", "-1", skaPrecision, true, ""},
		{"negative fractional rejected", "-0.5", skaPrecision, true, ""},
		{"negative VAR rejected", "-1.23", varPrecision, true, ""},

		{"SKA fractional one digit too many rejected", "1.1234567890123456789", skaPrecision, true, ""},
		{"VAR fractional one digit too many rejected", "1.123456789", varPrecision, true, ""},

		{"empty rejected", "", skaPrecision, true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := coinsToAtomsBig(tt.amount, tt.atomsPer)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %s", got.String())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.String() != tt.wantAtoms {
				t.Fatalf("atoms: got %s, want %s", got.String(), tt.wantAtoms)
			}
		})
	}
}
