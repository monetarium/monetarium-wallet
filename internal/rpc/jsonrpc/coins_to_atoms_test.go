// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package jsonrpc

import (
	"math/big"
	"strings"
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

		// Malformed-input regressions added 2026-05-05. Before the M3 fix,
		// "." parsed silently as 0 (intPart="" + fracPart="" padded to "000…0"
		// which big.Int.SetString accepts), and "+1.5" parsed because
		// big.Int.SetString permits a leading '+'. Both violate the
		// decimal-coin-string contract — pin the rejection.
		{"bare decimal point rejected", ".", skaPrecision, true, ""},
		{"bare decimal point VAR rejected", ".", varPrecision, true, ""},
		{"missing integer part rejected", ".5", skaPrecision, true, ""},
		{"missing integer part VAR rejected", ".5", varPrecision, true, ""},
		{"leading plus rejected", "+1", skaPrecision, true, ""},
		{"leading plus with fraction rejected", "+1.5", skaPrecision, true, ""},
		{"leading plus VAR rejected", "+1.23", varPrecision, true, ""},

		// Trailing decimal point with present integer part remains valid
		// — "1." parses as 1 coin under the same contract as "1" or "1.0".
		{"trailing decimal point accepted", "1.", skaPrecision, false, "1000000000000000000"},
		{"trailing decimal point VAR accepted", "1.", varPrecision, false, "100000000"},
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

// TestCoinsToAtomsBigPow10Invariant pins the L5 fix from the 2026-05-05
// review: validateSKAChainParams enforces that all SKA AtomsPerCoin values
// are exact powers of 10 at wallet open, but coinsToAtomsBig is reachable
// from call sites that may bypass that path (or be added in the future).
// The function uses len(atomsPerCoin.String())-1 as a shortcut for
// log10(atomsPerCoin); that shortcut silently corrupts when atomsPerCoin is
// not a power of 10. Reject up front with a clear error.
func TestCoinsToAtomsBigPow10Invariant(t *testing.T) {
	tests := []struct {
		name         string
		atomsPerCoin *big.Int
		wantErr      bool
		wantSubstr   string
	}{
		// Valid powers of 10.
		{"1 (10^0)", big.NewInt(1), false, ""},
		{"10 (10^1)", big.NewInt(10), false, ""},
		{"1e8 (VAR)", big.NewInt(1e8), false, ""},
		{"1e18 (SKA)", new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil), false, ""},

		// Non-powers of 10.
		{"5e17 non-pow10", new(big.Int).Mul(big.NewInt(5), new(big.Int).Exp(big.NewInt(10), big.NewInt(17), nil)), true, "power of 10"},
		{"15 non-pow10", big.NewInt(15), true, "power of 10"},
		{"99 non-pow10", big.NewInt(99), true, "power of 10"},

		// Negative — explicit reject (the nil/zero path falls back to AtomsPerVAR
		// at the function head and is exercised in the existing tests).
		{"negative", big.NewInt(-1000000), true, "must be positive"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			_, err := coinsToAtomsBig("1", tt.atomsPerCoin)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q; got nil", tt.wantSubstr)
				}
				if tt.wantSubstr != "" && !strings.Contains(err.Error(), tt.wantSubstr) {
					t.Fatalf("expected error containing %q; got %q", tt.wantSubstr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for valid power-of-10 atomsPerCoin: %v", err)
			}
		})
	}
}

// TestCoinsToAtomsBigLengthCap pins the DoS-bound on caller-supplied decimal
// strings: an authenticated RPC caller must not be able to coerce the wallet
// into parsing a multi-megabyte string into a multi-megabyte big.Int.
func TestCoinsToAtomsBigLengthCap(t *testing.T) {
	skaPrecision := cointype.GetAtomsPerSKACoin()

	// 200 zeros after a leading "1" — well over the 100-char cap. Must error.
	tooLong := "1" + strings.Repeat("0", 200)
	if _, err := coinsToAtomsBig(tooLong, skaPrecision); err == nil {
		t.Fatalf("expected error for %d-char amount, got none", len(tooLong))
	} else if !strings.Contains(err.Error(), "too long") {
		t.Fatalf("expected length-cap error, got: %v", err)
	}

	// Strings near the boundary: 100-char string accepted (any valid value
	// of that length); 101-char string rejected.
	atBound := "1." + strings.Repeat("0", 17) + "1" // exactly 20 chars, well under cap, valid SKA precision
	if _, err := coinsToAtomsBig(atBound, skaPrecision); err != nil {
		t.Fatalf("unexpected error for valid short amount: %v", err)
	}

	// Construct a 101-char string. Use "0." + 99 zeros (101 chars total).
	// big.Int parses to 0, but the length-cap rejection must fire before
	// the Sign() check in callers — so the error here must be the length
	// cap, not "must be positive".
	overBound := "0." + strings.Repeat("0", 99)
	if _, err := coinsToAtomsBig(overBound, skaPrecision); err == nil {
		t.Fatalf("expected error for %d-char amount, got none", len(overBound))
	} else if !strings.Contains(err.Error(), "too long") {
		t.Fatalf("expected length-cap error for 101-char input, got: %v", err)
	}
}
