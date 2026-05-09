// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txauthor_test

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/dcrutil"
	"github.com/monetarium/monetarium-node/wire"
	"github.com/monetarium/monetarium-wallet/wallet/txauthor"
	"github.com/monetarium/monetarium-wallet/wallet/txsizes"
)

// makeBigIntInputSource builds an InputSource that does NOT truncate via
// Int64(). Use this fixture in tests that exercise SKA atom values exceeding
// math.MaxInt64; the existing makeInputSourceWithCoinType in
// author_dual_coin_test.go truncates and must not be used here.
func makeBigIntInputSource(unspents []*wire.TxOut) txauthor.InputSource {
	currentSKATotal := cointype.Zero()
	currentInputs := make([]*wire.TxIn, 0, len(unspents))
	redeemScriptSizes := make([]int, 0, len(unspents))
	return func(_ dcrutil.Amount, targetSKA cointype.SKAAmount) (*txauthor.InputDetail, error) {
		for len(unspents) != 0 {
			if !targetSKA.IsZero() && currentSKATotal.Cmp(targetSKA) >= 0 {
				break
			}
			u := unspents[0]
			unspents = unspents[1:]
			if !u.CoinType.IsSKA() || u.SKAValue == nil {
				continue
			}
			// Defensive-copy so the caller cannot mutate our totals via the
			// wire.TxIn's SKAValueIn pointer.
			ska := cointype.NewSKAAmount(new(big.Int).Set(u.SKAValue))
			currentSKATotal = currentSKATotal.Add(ska)
			txIn := wire.NewTxIn(&wire.OutPoint{}, 0, nil)
			txIn.SKAValueIn = new(big.Int).Set(u.SKAValue)
			currentInputs = append(currentInputs, txIn)
			redeemScriptSizes = append(redeemScriptSizes, txsizes.RedeemP2PKHSigScriptSize)
		}
		return &txauthor.InputDetail{
			SKAAmount:         currentSKATotal,
			Inputs:            currentInputs,
			RedeemScriptSizes: redeemScriptSizes,
		}, nil
	}
}

// TestSKAValueGreaterThanInt64MaxRoundTrip asserts that an SKA atom value
// above math.MaxInt64 (~9.2e18) survives the full author → wire-serialise →
// wire-deserialise pipeline without truncation. Mainnet SKA1 is configured
// with 900 trillion coins × 1e18 atoms/coin = 9e29 atoms total supply, well
// above int64; a regression that re-introduces an Int64() conversion path on
// the canonical authoring code path would only be caught at first mainnet
// broadcast — this test pins the lossless contract at the unit layer.
func TestSKAValueGreaterThanInt64MaxRoundTrip(t *testing.T) {
	skaCoinType := cointype.CoinType(1)

	// 1 << 100 atoms ≈ 1.27e30, well above math.MaxInt64 (~9.22e18).
	largeAtoms := new(big.Int).Lsh(big.NewInt(1), 100)
	// Need slightly more on the input side to cover dust + the change output;
	// shift by 101 to give the input source 2× headroom over the output.
	inputAtoms := new(big.Int).Lsh(big.NewInt(1), 101)

	outScript := make([]byte, txsizes.P2PKHOutputSize)
	skaOutput := &wire.TxOut{
		PkScript: outScript,
		CoinType: skaCoinType,
		Value:    0,
		SKAValue: new(big.Int).Set(largeAtoms),
	}
	skaInput := &wire.TxOut{
		PkScript: outScript,
		CoinType: skaCoinType,
		Value:    0,
		SKAValue: new(big.Int).Set(inputAtoms),
	}

	relayFee := cointype.SKAAmountFromInt64(1e3)
	inputSource := makeBigIntInputSource([]*wire.TxOut{skaInput})
	changeSource := AuthorTestChangeSource{}

	atx, err := txauthor.NewUnsignedTransaction(
		[]*wire.TxOut{skaOutput}, relayFee, inputSource, changeSource, 100000, -1)
	if err != nil {
		t.Fatalf("NewUnsignedTransaction: %v", err)
	}

	// Locate the non-change output (the canonical SKA payment) and assert
	// SKAValue equals the original big.Int.
	if atx.ChangeIndex < 0 || atx.ChangeIndex >= len(atx.Tx.TxOut) {
		t.Fatalf("expected a change output, got ChangeIndex=%d", atx.ChangeIndex)
	}
	var paymentOut *wire.TxOut
	for i, out := range atx.Tx.TxOut {
		if i == atx.ChangeIndex {
			continue
		}
		paymentOut = out
		break
	}
	if paymentOut == nil {
		t.Fatalf("authored tx has no payment output")
	}
	if paymentOut.SKAValue == nil {
		t.Fatalf("payment output SKAValue is nil")
	}
	if paymentOut.SKAValue.Cmp(largeAtoms) != 0 {
		t.Fatalf("authored payment SKAValue mismatch: got %s, want %s",
			paymentOut.SKAValue, largeAtoms)
	}

	// Verify the input ValueIn / SKAValueIn semantics:
	// Value should be 0 (SKA inputs carry value in SKAValueIn, not Value).
	if len(atx.Tx.TxIn) != 1 {
		t.Fatalf("expected 1 input, got %d", len(atx.Tx.TxIn))
	}
	in := atx.Tx.TxIn[0]
	if in.ValueIn != 0 {
		t.Fatalf("SKA input ValueIn should be 0, got %d", in.ValueIn)
	}
	if in.SKAValueIn == nil {
		t.Fatalf("SKA input SKAValueIn is nil")
	}
	if in.SKAValueIn.Cmp(inputAtoms) != 0 {
		t.Fatalf("SKA input SKAValueIn mismatch: got %s, want %s",
			in.SKAValueIn, inputAtoms)
	}

	// Round-trip the authored tx through wire serialise/deserialise to lock
	// down the V13 wire format's SKAValue handling for >int64 values.
	var buf bytes.Buffer
	if err := atx.Tx.Serialize(&buf); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	roundTrip := wire.NewMsgTx()
	if err := roundTrip.Deserialize(&buf); err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	if len(roundTrip.TxOut) != len(atx.Tx.TxOut) {
		t.Fatalf("round-trip TxOut count mismatch: got %d, want %d",
			len(roundTrip.TxOut), len(atx.Tx.TxOut))
	}
	// Compare the non-change output across the round-trip.
	var rtPayment *wire.TxOut
	for i, out := range roundTrip.TxOut {
		if i == atx.ChangeIndex {
			continue
		}
		rtPayment = out
		break
	}
	if rtPayment == nil || rtPayment.SKAValue == nil {
		t.Fatalf("round-trip payment output missing or has nil SKAValue")
	}
	if rtPayment.SKAValue.Cmp(largeAtoms) != 0 {
		t.Fatalf("round-trip SKAValue mismatch: got %s, want %s",
			rtPayment.SKAValue, largeAtoms)
	}
	if rtPayment.CoinType != skaCoinType {
		t.Fatalf("round-trip CoinType mismatch: got %d, want %d",
			rtPayment.CoinType, skaCoinType)
	}
	if rtPayment.Value != 0 {
		t.Fatalf("round-trip Value should be 0 for SKA output, got %d", rtPayment.Value)
	}
}
