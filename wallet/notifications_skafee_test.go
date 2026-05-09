// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"math/big"
	"testing"
)

// TestTransactionSummaryHasSKAFields verifies the dual-coin extension fields
// exist on TransactionSummary, TransactionSummaryInput, and
// TransactionSummaryOutput. These fields close the silent-zero gap for SKA
// transactions where the legacy int64 Fee / Amount / PreviousAmount fields
// always serialized to zero.
func TestTransactionSummaryHasSKAFields(t *testing.T) {
	skaFee := big.NewInt(42_000)
	skaOut := big.NewInt(1_000_000_000_000_000_000) // 1 SKA at 1e18
	skaIn := big.NewInt(1_000_000_000_000_042_000)

	ts := TransactionSummary{
		Fee:    0,
		SKAFee: skaFee,
	}
	if ts.SKAFee == nil || ts.SKAFee.Cmp(skaFee) != 0 {
		t.Errorf("SKAFee not set or wrong value: %v", ts.SKAFee)
	}

	out := TransactionSummaryOutput{
		Amount:    0,
		SKAAmount: skaOut,
	}
	if out.SKAAmount == nil || out.SKAAmount.Cmp(skaOut) != 0 {
		t.Errorf("SKAAmount not set or wrong value: %v", out.SKAAmount)
	}

	in := TransactionSummaryInput{
		PreviousAmount:    0,
		PreviousSKAAmount: skaIn,
	}
	if in.PreviousSKAAmount == nil || in.PreviousSKAAmount.Cmp(skaIn) != 0 {
		t.Errorf("PreviousSKAAmount not set or wrong value: %v", in.PreviousSKAAmount)
	}
}

// TestTransactionSummaryVARLeavesSKANil asserts that constructing a VAR-only
// summary leaves the new SKA fields nil so consumers can branch on
// "is this an SKA tx?" by checking SKAFee != nil.
func TestTransactionSummaryVARLeavesSKANil(t *testing.T) {
	ts := TransactionSummary{Fee: 12345}
	if ts.SKAFee != nil {
		t.Errorf("expected SKAFee=nil for VAR tx; got %v", ts.SKAFee)
	}
	out := TransactionSummaryOutput{Amount: 1000}
	if out.SKAAmount != nil {
		t.Errorf("expected SKAAmount=nil for VAR output; got %v", out.SKAAmount)
	}
	in := TransactionSummaryInput{PreviousAmount: 2000}
	if in.PreviousSKAAmount != nil {
		t.Errorf("expected PreviousSKAAmount=nil for VAR input; got %v", in.PreviousSKAAmount)
	}
}
