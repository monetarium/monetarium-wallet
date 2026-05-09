// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"math/big"
	"strings"
	"testing"

	"github.com/monetarium/monetarium-node/chaincfg"
	"github.com/monetarium/monetarium-node/cointype"

	"github.com/monetarium/monetarium-wallet/errors"
)

// TestValidateSKAChainParamsAcceptsNil documents the nil-safety contract:
// a wallet built without any chain params or with no SKA coins configured
// is valid (the dual-coin features are simply unused).
func TestValidateSKAChainParamsAcceptsNil(t *testing.T) {
	if err := validateSKAChainParams(nil); err != nil {
		t.Fatalf("nil params: got %v, want nil", err)
	}

	emptyParams := &chaincfg.Params{}
	if err := validateSKAChainParams(emptyParams); err != nil {
		t.Fatalf("params with no SKACoins: got %v, want nil", err)
	}
}

// TestValidateSKAChainParamsAcceptsMainnet asserts the validator does not
// reject any production SKA coin config. Failure here means a release would
// no longer be openable.
func TestValidateSKAChainParamsAcceptsMainnet(t *testing.T) {
	if err := validateSKAChainParams(chaincfg.MainNetParams()); err != nil {
		t.Fatalf("mainnet params rejected: %v", err)
	}
}

// TestValidateSKAChainParamsAcceptsAtomsPerCoinOne locks in the boundary case:
// AtomsPerCoin=1 means 0 decimal places (10^0). The decimal-place inference
// in coinsToAtomsBig and DecimalStringToAtoms handles this correctly because
// len("1") - 1 == 0, so the validator must accept it.
func TestValidateSKAChainParamsAcceptsAtomsPerCoinOne(t *testing.T) {
	params := &chaincfg.Params{
		SKACoins: map[cointype.CoinType]*chaincfg.SKACoinConfig{
			cointype.CoinType(1): {AtomsPerCoin: big.NewInt(1)},
		},
	}
	if err := validateSKAChainParams(params); err != nil {
		t.Fatalf("AtomsPerCoin=1 (10^0) must be accepted; got %v", err)
	}
}

// TestValidateSKAChainParamsRejectsBrokenAtomsPerCoin covers the negative
// space the prior tests left uncovered: every input shape that should make
// wallet Open fail loudly rather than silently miscompute amounts at the
// coinsToAtomsBig precondition.
func TestValidateSKAChainParamsRejectsBrokenAtomsPerCoin(t *testing.T) {
	tests := []struct {
		name            string
		atomsPerCoin    *big.Int
		wantMsgContains string
	}{
		{"nil", nil, "must be positive"},
		{"zero", big.NewInt(0), "must be positive"},
		{"negative", big.NewInt(-1), "must be positive"},
		{"non-pow10 mantissa", big.NewInt(99999999), "not a power of 10"},
		{"close-to-pow10", big.NewInt(1000000001), "not a power of 10"},
		{"two-digit non-pow10", big.NewInt(50), "not a power of 10"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			params := &chaincfg.Params{
				SKACoins: map[cointype.CoinType]*chaincfg.SKACoinConfig{
					cointype.CoinType(1): {AtomsPerCoin: tc.atomsPerCoin},
				},
			}
			err := validateSKAChainParams(params)
			if err == nil {
				t.Fatalf("expected error, got nil for AtomsPerCoin=%v", tc.atomsPerCoin)
			}
			if !errors.Is(err, errors.Invalid) {
				t.Fatalf("expected errors.Invalid kind, got %v", err)
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.wantMsgContains) {
				t.Errorf("error message must contain %q; got %q",
					tc.wantMsgContains, msg)
			}
			// MED-3: every error message must point operators at the
			// chain-params invariant source so they don't waste time
			// looking for a wallet-side override.
			if !strings.Contains(msg, "SKACoinConfig") {
				t.Errorf("error message must reference chaincfg SKACoinConfig; got %q", msg)
			}
		})
	}
}

// TestValidateSKAChainParamsSkipsNilEntry asserts the validator treats a nil
// per-coin config as "not present" rather than as a broken config. The
// wallet-side iteration over SKACoins is best-effort: chain-params builders
// may legitimately leave gaps in the map.
func TestValidateSKAChainParamsSkipsNilEntry(t *testing.T) {
	params := &chaincfg.Params{
		SKACoins: map[cointype.CoinType]*chaincfg.SKACoinConfig{
			cointype.CoinType(1): nil,
			cointype.CoinType(2): {AtomsPerCoin: big.NewInt(100)},
		},
	}
	if err := validateSKAChainParams(params); err != nil {
		t.Fatalf("nil per-coin config must be skipped, not rejected; got %v", err)
	}
}

// TestValidateSKAChainParamsRejectsRandomNonPow10 generates a wide spread of
// inputs deterministically and asserts the validator's accept/reject decision
// matches a reference power-of-10 detector. Pin from the 2026-05-04 review:
// the existing hardcoded cases (50, 99999999, 1000000001) leave the negative
// space lightly probed; this test fans out across the 64-bit range so a
// future rewrite that, for example, accepts negative powers or off-by-one
// ranges fails loudly.
//
// The reference is intentionally trivial — repeated /10 until 1 (accept)
// or non-zero remainder (reject). Any divergence between this and the
// production validator is a bug in the validator.
func TestValidateSKAChainParamsRejectsRandomNonPow10(t *testing.T) {
	// Fixed seed so failures reproduce. We avoid math/rand so the numbers
	// are picked deterministically: a Linear Congruential Generator over
	// the [1, 2^62) range giving 200 distinct, well-distributed values
	// across both the pow10 and non-pow10 cases.
	const numIters = 200
	const a, c uint64 = 6364136223846793005, 1442695040888963407
	state := uint64(0xC0DECAFE_12345678)

	classify := func(x *big.Int) (isPow10 bool) {
		// Reference: positive AND repeatedly /10 reaches 1 with no remainder.
		if x == nil || x.Sign() <= 0 {
			return false
		}
		ten := big.NewInt(10)
		one := big.NewInt(1)
		v := new(big.Int).Set(x)
		zero := big.NewInt(0)
		mod := new(big.Int)
		for v.Cmp(one) != 0 {
			new(big.Int).DivMod(v, ten, mod)
			if mod.Cmp(zero) != 0 {
				return false
			}
			v.Quo(v, ten)
		}
		return true
	}

	for i := 0; i < numIters; i++ {
		state = state*a + c
		// Mix a bit so we hit both small and large magnitudes.
		shift := uint(state % 60)
		val := big.NewInt(int64(state >> shift))
		// Throw in a power-of-10 case every 7th iteration to keep the
		// accept side covered.
		if i%7 == 0 {
			exp := int64(state % 25) // up to 10^24
			val = new(big.Int).Exp(big.NewInt(10), big.NewInt(exp), nil)
		}
		params := &chaincfg.Params{
			SKACoins: map[cointype.CoinType]*chaincfg.SKACoinConfig{
				cointype.CoinType(1): {AtomsPerCoin: val},
			},
		}
		err := validateSKAChainParams(params)
		want := classify(val)
		gotAccept := err == nil
		if want != gotAccept {
			t.Errorf("iter %d AtomsPerCoin=%s: validator %s but reference says %s",
				i, val.String(),
				map[bool]string{true: "accepted", false: "rejected"}[gotAccept],
				map[bool]string{true: "accept (pow10)", false: "reject (non-pow10)"}[want])
		}
	}
}
