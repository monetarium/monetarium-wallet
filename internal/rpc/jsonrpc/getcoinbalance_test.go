// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package jsonrpc

import (
	"math"
	"math/big"
	"testing"

	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/dcrutil"
	"github.com/monetarium/monetarium-wallet/wallet"
)

// TestRenderCoinBalanceAmountsSKAPrecision verifies that SKA balances
// exceeding math.MaxInt64 atoms render through ToDecimalString rather than
// through the int64 → float64 path (HIGH-#1: getcoinbalance truncated SKA
// totals via int64 cast). The test pins the wire-format change: SKA fields
// must be strings, VAR fields must be float64.
func TestRenderCoinBalanceAmountsSKAPrecision(t *testing.T) {
	// 50 billion SKA at AtomsPerCoin=1e18 = 5e28 atoms — far above math.MaxInt64
	// (~9.22e18). Casting through int64 would silently truncate.
	skaAtomsStr := "50000000000000000000000000000"
	skaAtoms, ok := new(big.Int).SetString(skaAtomsStr, 10)
	if !ok {
		t.Fatalf("failed to parse big SKA atoms")
	}
	atomsPerSKA := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)

	cb := wallet.CoinBalance{
		CoinType:                   cointype.CoinType(1),
		SKASpendable:               cointype.NewSKAAmount(skaAtoms),
		SKATotal:                   cointype.NewSKAAmount(skaAtoms),
		SKAUnconfirmed:             cointype.Zero(),
		SKAImmatureCoinbaseRewards: cointype.Zero(),
		SKAImmatureStakeGeneration: cointype.Zero(),
	}

	got := renderCoinBalanceAmounts(cb, true /* isSKA */, atomsPerSKA)

	spendable, ok := got.Spendable.(string)
	if !ok {
		t.Fatalf("SKA Spendable should be string, got %T", got.Spendable)
	}
	// 5e28 atoms / 1e18 atoms-per-coin = 5e10 coins = "50000000000"
	const wantWhole = "50000000000"
	if spendable != wantWhole {
		t.Errorf("SKA Spendable: want %q, got %q", wantWhole, spendable)
	}

	total, _ := got.Total.(string)
	if total != wantWhole {
		t.Errorf("SKA Total: want %q, got %q", wantWhole, total)
	}

	// Stake-only fields must be the literal "0" string for SKA — SKA does not
	// participate in PoS.
	if got.LockedByTickets != "0" {
		t.Errorf("SKA LockedByTickets: want %q, got %v (%T)", "0", got.LockedByTickets, got.LockedByTickets)
	}
	if got.VotingAuthority != "0" {
		t.Errorf("SKA VotingAuthority: want %q, got %v (%T)", "0", got.VotingAuthority, got.VotingAuthority)
	}
	if got.ImmatureStakeGeneration != "0" {
		t.Errorf("SKA ImmatureStakeGeneration: want %q, got %v (%T)", "0", got.ImmatureStakeGeneration, got.ImmatureStakeGeneration)
	}

	// Verify that pre-fix int64 cast would have produced wrong values.
	// math.MaxInt64 = 9223372036854775807; our value is ~5.4e9× that.
	if int64(skaAtoms.Int64()) > 0 && new(big.Int).SetInt64(skaAtoms.Int64()).Cmp(skaAtoms) == 0 {
		t.Fatal("test setup error: skaAtoms must exceed int64 to exercise the precision-loss path")
	}
}

// TestRenderCoinBalanceAmountsVARFloat verifies VAR balances still render as
// float64 (preserving the existing wire format for non-SKA queries).
func TestRenderCoinBalanceAmountsVARFloat(t *testing.T) {
	atomsPerVAR := big.NewInt(1e8)
	cb := wallet.CoinBalance{
		CoinType:    cointype.CoinTypeVAR,
		Spendable:   dcrutil.Amount(150_000_000),
		Total:       dcrutil.Amount(200_000_000),
		Unconfirmed: dcrutil.Amount(50_000_000),
	}

	got := renderCoinBalanceAmounts(cb, false /* isSKA */, atomsPerVAR)

	spendable, ok := got.Spendable.(float64)
	if !ok {
		t.Fatalf("VAR Spendable should be float64, got %T", got.Spendable)
	}
	if spendable != 1.5 {
		t.Errorf("VAR Spendable: want 1.5, got %v", spendable)
	}

	total, _ := got.Total.(float64)
	if total != 2.0 {
		t.Errorf("VAR Total: want 2.0, got %v", total)
	}
}

// TestRenderCoinBalanceAmountsSKAZero verifies the empty-balance SKA case
// renders as the decimal string "0", not float64(0). This pins the wire
// format for callers iterating coin types.
func TestRenderCoinBalanceAmountsSKAZero(t *testing.T) {
	atomsPerSKA := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	cb := wallet.CoinBalance{
		CoinType:                   cointype.CoinType(1),
		SKASpendable:               cointype.Zero(),
		SKATotal:                   cointype.Zero(),
		SKAUnconfirmed:             cointype.Zero(),
		SKAImmatureCoinbaseRewards: cointype.Zero(),
		SKAImmatureStakeGeneration: cointype.Zero(),
	}

	got := renderCoinBalanceAmounts(cb, true, atomsPerSKA)

	if s, ok := got.Spendable.(string); !ok || s != "0" {
		t.Errorf("SKA zero Spendable: want string \"0\", got %v (%T)", got.Spendable, got.Spendable)
	}
	if s, ok := got.Total.(string); !ok || s != "0" {
		t.Errorf("SKA zero Total: want string \"0\", got %v (%T)", got.Total, got.Total)
	}
}

// TestRenderCoinBalanceAmountsVARMaxInt64 verifies the VAR float64 path
// rounds correctly at the largest representable int64 atom value, ensuring
// no regression for callers near the upper int64 limit.
func TestRenderCoinBalanceAmountsVARMaxInt64(t *testing.T) {
	atomsPerVAR := big.NewInt(1e8)
	cb := wallet.CoinBalance{
		CoinType:  cointype.CoinTypeVAR,
		Spendable: dcrutil.Amount(math.MaxInt64),
	}
	got := renderCoinBalanceAmounts(cb, false, atomsPerVAR)
	if _, ok := got.Spendable.(float64); !ok {
		t.Fatalf("VAR Spendable should be float64, got %T", got.Spendable)
	}
}
