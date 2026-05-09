// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txauthor_test

import (
	"math/big"
	"strings"
	"testing"

	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/dcrutil"
	"github.com/monetarium/monetarium-node/wire"
	"github.com/monetarium/monetarium-wallet/errors"
	"github.com/monetarium/monetarium-wallet/wallet/txauthor"
	"github.com/monetarium/monetarium-wallet/wallet/txrules"
	"github.com/monetarium/monetarium-wallet/wallet/txsizes"
)

// TestSubtractFeeFromAmountVAR exercises the subtractfeefromamount path on a
// VAR send: the recipient output should be reduced by the fee, change should
// equal inputs minus the requested amount (NOT minus the fee), and inputs +
// outputs + fee must conserve.
func TestSubtractFeeFromAmountVAR(t *testing.T) {
	relayFee := cointype.SKAAmountFromInt64(1e3)
	requested := dcrutil.Amount(1e6)
	unspents := p2pkhOutputs(1e8) // single 1 VAR UTXO

	outputs := p2pkhOutputs(requested)
	inputSource := makeInputSource(unspents)

	tx, err := txauthor.NewUnsignedTransaction(outputs, relayFee, inputSource,
		AuthorTestChangeSource{}, 100000, 0)
	if err != nil {
		t.Fatalf("NewUnsignedTransaction: %v", err)
	}

	// Recompute the fee from the resulting tx size and assert the recipient
	// output was reduced by exactly that fee.
	relayFeeInt64, _ := relayFee.Int64()
	expectedFee := txrules.FeeForSerializeSize(dcrutil.Amount(relayFeeInt64),
		tx.EstimatedSignedSerializeSize)

	// Identify the non-change output.
	var recipientIdx int
	if tx.ChangeIndex == 0 {
		recipientIdx = 1
	}
	gotRecipient := dcrutil.Amount(tx.Tx.TxOut[recipientIdx].Value)
	wantRecipient := requested - expectedFee
	if gotRecipient != wantRecipient {
		t.Errorf("recipient amount: got %v, want %v (requested %v − fee %v)",
			gotRecipient, wantRecipient, requested, expectedFee)
	}

	// Change should be inputs − requested (NOT − fee), since the fee was
	// absorbed by reducing the recipient output.
	if tx.ChangeIndex < 0 {
		t.Fatalf("expected change output for VAR subtractfee; got none")
	}
	gotChange := dcrutil.Amount(tx.Tx.TxOut[tx.ChangeIndex].Value)
	wantChange := dcrutil.Amount(1e8) - requested
	if gotChange != wantChange {
		t.Errorf("change amount: got %v, want %v (inputs %v − requested %v)",
			gotChange, wantChange, dcrutil.Amount(1e8), requested)
	}

	// Conservation: inputs == sum(outputs) + fee.
	var outSum dcrutil.Amount
	for _, o := range tx.Tx.TxOut {
		outSum += dcrutil.Amount(o.Value)
	}
	if outSum+expectedFee != tx.TotalInput {
		t.Errorf("conservation broken: inputs=%v outputs=%v fee=%v",
			tx.TotalInput, outSum, expectedFee)
	}
}

// TestSubtractFeeFromAmountVARUnderflow verifies that a recipient amount that
// fails to cover the fee plus dust is rejected with an error rather than
// silently broadcast.
func TestSubtractFeeFromAmountVARUnderflow(t *testing.T) {
	relayFee := cointype.SKAAmountFromInt64(1e3)
	// A 100-atom recipient cannot cover even the smallest VAR tx fee.
	tinyAmount := dcrutil.Amount(100)
	unspents := p2pkhOutputs(1e8)
	outputs := p2pkhOutputs(tinyAmount)
	inputSource := makeInputSource(unspents)

	_, err := txauthor.NewUnsignedTransaction(outputs, relayFee, inputSource,
		AuthorTestChangeSource{}, 100000, 0)
	if err == nil {
		t.Fatalf("expected error for amount below fee+dust; got nil")
	}
	if !strings.Contains(err.Error(), "subtractfeefromamount") {
		t.Errorf("expected subtractfeefromamount error; got %v", err)
	}
}

// TestSubtractFeeFromAmountIdxOutOfRange confirms the index validation in the
// public entry point.
func TestSubtractFeeFromAmountIdxOutOfRange(t *testing.T) {
	relayFee := cointype.SKAAmountFromInt64(1e3)
	outputs := p2pkhOutputs(1e6)
	unspents := p2pkhOutputs(1e8)
	inputSource := makeInputSource(unspents)

	_, err := txauthor.NewUnsignedTransaction(outputs, relayFee, inputSource,
		AuthorTestChangeSource{}, 100000, 5) // out of range
	if err == nil {
		t.Fatalf("expected error for out-of-range index; got nil")
	}
	if !errors.Is(err, errors.Invalid) {
		t.Errorf("expected Invalid error; got %v", err)
	}
}

// TestSubtractFeeFromAmountSKA exercises the SKA path: the SKA recipient
// output's SKAValue must be reduced by the SKA fee, and change in SKA must
// equal inputs − requested.
func TestSubtractFeeFromAmountSKA(t *testing.T) {
	skaCoin := cointype.CoinType(1)
	relayFee := cointype.SKAAmountFromInt64(1e3)

	requested := big.NewInt(100000000) // 1e8 SKA atoms
	inputAmount := big.NewInt(200000000)

	skaUnspents := skaP2PKHOutputs(skaCoin, inputAmount)
	skaOutputs := skaP2PKHOutputs(skaCoin, requested)
	inputSource := makeSKAInputSource(skaUnspents, skaCoin)

	tx, err := txauthor.NewUnsignedTransaction(skaOutputs, relayFee, inputSource,
		AuthorTestChangeSource{}, 100000, 0)
	if err != nil {
		t.Fatalf("NewUnsignedTransaction(SKA): %v", err)
	}

	// Compute expected SKA fee from the actual tx size.
	expectedFeeSKA := txrules.FeeForSerializeSizeSKA(relayFee,
		tx.EstimatedSignedSerializeSize)

	// Find the non-change output (the recipient).
	var recipientIdx int
	if tx.ChangeIndex == 0 {
		recipientIdx = 1
	}
	gotRecipient := tx.Tx.TxOut[recipientIdx].SKAValue
	wantRecipient := new(big.Int).Sub(requested, expectedFeeSKA.BigInt())
	if gotRecipient.Cmp(wantRecipient) != 0 {
		t.Errorf("SKA recipient amount: got %s, want %s (requested %s − fee %s)",
			gotRecipient, wantRecipient, requested, expectedFeeSKA.BigInt())
	}

	// Change in SKA = inputs − requested. The fee was absorbed by the
	// recipient reduction, NOT the change.
	if tx.ChangeIndex < 0 {
		t.Fatalf("expected SKA change output; got none")
	}
	gotChange := tx.Tx.TxOut[tx.ChangeIndex].SKAValue
	wantChange := new(big.Int).Sub(inputAmount, requested)
	if gotChange.Cmp(wantChange) != 0 {
		t.Errorf("SKA change amount: got %s, want %s (inputs %s − requested %s)",
			gotChange, wantChange, inputAmount, requested)
	}

	// Conservation: inputs == sum(SKA outputs) + SKA fee.
	outSum := new(big.Int)
	for _, o := range tx.Tx.TxOut {
		if o.SKAValue != nil {
			outSum.Add(outSum, o.SKAValue)
		}
	}
	totalIn := tx.SKATotalInput.BigInt()
	withFee := new(big.Int).Add(outSum, expectedFeeSKA.BigInt())
	if withFee.Cmp(totalIn) != 0 {
		t.Errorf("SKA conservation broken: inputs=%s outputs=%s fee=%s",
			totalIn, outSum, expectedFeeSKA.BigInt())
	}
}

// TestSubtractFeeFromAmountSKAUnderflow confirms that an SKA recipient amount
// at or below the SKA fee returns an error.
func TestSubtractFeeFromAmountSKAUnderflow(t *testing.T) {
	skaCoin := cointype.CoinType(1)
	relayFee := cointype.SKAAmountFromInt64(1e3)

	// Recipient of 5 atoms cannot cover any positive SKA fee.
	requested := big.NewInt(5)
	inputAmount := big.NewInt(1e8)

	skaUnspents := skaP2PKHOutputs(skaCoin, inputAmount)
	skaOutputs := skaP2PKHOutputs(skaCoin, requested)
	inputSource := makeSKAInputSource(skaUnspents, skaCoin)

	_, err := txauthor.NewUnsignedTransaction(skaOutputs, relayFee, inputSource,
		AuthorTestChangeSource{}, 100000, 0)
	if err == nil {
		t.Fatalf("expected error for SKA amount below fee; got nil")
	}
	if !strings.Contains(err.Error(), "subtractfeefromamount") {
		t.Errorf("expected subtractfeefromamount error; got %v", err)
	}
}

// TestSubtractFeeFromAmountVARMultiRecipient verifies that when the
// subtractFeeFromAmountIdx points at a non-zero output in a multi-recipient
// VAR transaction, only the targeted recipient is reduced — the other
// recipients (and the original *wire.TxOut objects passed in by the caller)
// are left untouched. Also covers the M1 caller-slice non-mutation contract:
// after the call, the caller's outputs slice still references the same
// *wire.TxOut pointers it started with, with their fields unchanged.
func TestSubtractFeeFromAmountVARMultiRecipient(t *testing.T) {
	relayFee := cointype.SKAAmountFromInt64(1e3)
	const a0, a1, a2 = dcrutil.Amount(2e6), dcrutil.Amount(3e6), dcrutil.Amount(4e6)
	unspents := p2pkhOutputs(1e8)

	outputs := p2pkhOutputs(a0, a1, a2)
	// Snapshot: original pointers and Value/Script identity, asserted unchanged after.
	origPtrs := []*wire.TxOut{outputs[0], outputs[1], outputs[2]}
	origValues := []int64{outputs[0].Value, outputs[1].Value, outputs[2].Value}

	inputSource := makeInputSource(unspents)

	tx, err := txauthor.NewUnsignedTransaction(outputs, relayFee, inputSource,
		AuthorTestChangeSource{}, 100000, 1)
	if err != nil {
		t.Fatalf("NewUnsignedTransaction: %v", err)
	}

	// Caller's slice must be untouched: same pointers, same field values.
	for i := 0; i < 3; i++ {
		if outputs[i] != origPtrs[i] {
			t.Errorf("caller outputs[%d] pointer was replaced (M1 contract violated)", i)
		}
		if outputs[i].Value != origValues[i] {
			t.Errorf("caller outputs[%d].Value mutated: got %d, want %d (M1 contract violated)",
				i, outputs[i].Value, origValues[i])
		}
	}

	// Inside the resulting tx: change output is appended at the end (index 3,
	// since NewUnsignedTransaction does not randomize change position).
	if tx.ChangeIndex != 3 {
		t.Fatalf("expected change at index 3, got %d", tx.ChangeIndex)
	}

	relayFeeInt64, _ := relayFee.Int64()
	expectedFee := txrules.FeeForSerializeSize(dcrutil.Amount(relayFeeInt64),
		tx.EstimatedSignedSerializeSize)

	// Recipients 0 and 2 unchanged; recipient 1 reduced by exactly the fee.
	if got := dcrutil.Amount(tx.Tx.TxOut[0].Value); got != a0 {
		t.Errorf("recipient[0]: got %v, want %v (must be untouched)", got, a0)
	}
	if got := dcrutil.Amount(tx.Tx.TxOut[1].Value); got != a1-expectedFee {
		t.Errorf("recipient[1]: got %v, want %v (a1 − fee)", got, a1-expectedFee)
	}
	if got := dcrutil.Amount(tx.Tx.TxOut[2].Value); got != a2 {
		t.Errorf("recipient[2]: got %v, want %v (must be untouched)", got, a2)
	}

	// Conservation: inputs == sum(outputs) + fee.
	var outSum dcrutil.Amount
	for _, o := range tx.Tx.TxOut {
		outSum += dcrutil.Amount(o.Value)
	}
	if outSum+expectedFee != tx.TotalInput {
		t.Errorf("conservation broken: inputs=%v outputs=%v fee=%v",
			tx.TotalInput, outSum, expectedFee)
	}
}

// TestSubtractFeeFromAmountSKAMultiRecipient mirrors the VAR multi-recipient
// test for SKA: non-zero subtractFeeFromAmountIdx in a 3-output SKA tx must
// reduce only outputs[1] and leave the caller's slice untouched.
func TestSubtractFeeFromAmountSKAMultiRecipient(t *testing.T) {
	skaCoin := cointype.CoinType(1)
	relayFee := cointype.SKAAmountFromInt64(1e3)

	a0 := big.NewInt(20000000)
	a1 := big.NewInt(30000000)
	a2 := big.NewInt(40000000)
	inputAmount := big.NewInt(200000000)

	skaUnspents := skaP2PKHOutputs(skaCoin, inputAmount)
	skaOutputs := append(skaP2PKHOutputs(skaCoin, a0),
		append(skaP2PKHOutputs(skaCoin, a1), skaP2PKHOutputs(skaCoin, a2)...)...)
	origPtrs := []*wire.TxOut{skaOutputs[0], skaOutputs[1], skaOutputs[2]}
	origVals := []*big.Int{
		new(big.Int).Set(skaOutputs[0].SKAValue),
		new(big.Int).Set(skaOutputs[1].SKAValue),
		new(big.Int).Set(skaOutputs[2].SKAValue),
	}

	inputSource := makeSKAInputSource(skaUnspents, skaCoin)

	tx, err := txauthor.NewUnsignedTransaction(skaOutputs, relayFee, inputSource,
		AuthorTestChangeSource{}, 100000, 1)
	if err != nil {
		t.Fatalf("NewUnsignedTransaction(SKA multi): %v", err)
	}

	// Caller's slice must be untouched.
	for i := 0; i < 3; i++ {
		if skaOutputs[i] != origPtrs[i] {
			t.Errorf("caller skaOutputs[%d] pointer was replaced (M1 contract violated)", i)
		}
		if skaOutputs[i].SKAValue.Cmp(origVals[i]) != 0 {
			t.Errorf("caller skaOutputs[%d].SKAValue mutated: got %s, want %s",
				i, skaOutputs[i].SKAValue, origVals[i])
		}
	}

	if tx.ChangeIndex != 3 {
		t.Fatalf("expected SKA change at index 3, got %d", tx.ChangeIndex)
	}

	expectedFeeSKA := txrules.FeeForSerializeSizeSKA(relayFee,
		tx.EstimatedSignedSerializeSize)

	if tx.Tx.TxOut[0].SKAValue.Cmp(a0) != 0 {
		t.Errorf("SKA recipient[0]: got %s, want %s (untouched)",
			tx.Tx.TxOut[0].SKAValue, a0)
	}
	wantR1 := new(big.Int).Sub(a1, expectedFeeSKA.BigInt())
	if tx.Tx.TxOut[1].SKAValue.Cmp(wantR1) != 0 {
		t.Errorf("SKA recipient[1]: got %s, want %s (a1 − fee)",
			tx.Tx.TxOut[1].SKAValue, wantR1)
	}
	if tx.Tx.TxOut[2].SKAValue.Cmp(a2) != 0 {
		t.Errorf("SKA recipient[2]: got %s, want %s (untouched)",
			tx.Tx.TxOut[2].SKAValue, a2)
	}

	// Conservation: SKA inputs == sum(SKA outputs) + SKA fee.
	outSum := new(big.Int)
	for _, o := range tx.Tx.TxOut {
		if o.SKAValue != nil {
			outSum.Add(outSum, o.SKAValue)
		}
	}
	withFee := new(big.Int).Add(outSum, expectedFeeSKA.BigInt())
	if withFee.Cmp(tx.SKATotalInput.BigInt()) != 0 {
		t.Errorf("SKA conservation broken: inputs=%s outputs=%s fee=%s",
			tx.SKATotalInput.BigInt(), outSum, expectedFeeSKA.BigInt())
	}
}

// skaP2PKHOutputs returns a single SKA P2PKH-shaped output with the given
// coin type and amount.
func skaP2PKHOutputs(coin cointype.CoinType, amount *big.Int) []*wire.TxOut {
	outScript := make([]byte, txsizes.P2PKHOutputSize)
	return []*wire.TxOut{{
		Value:    0,
		SKAValue: new(big.Int).Set(amount),
		PkScript: outScript,
		CoinType: coin,
	}}
}

// makeSKAInputSource produces an InputSource that returns SKA UTXOs of the
// requested coin type, accumulating until the SKA target is met. The returned
// InputDetail carries SKAAmount; the int64 Amount field is left zero (matches
// production wallet behavior for SKA-only inputs).
func makeSKAInputSource(unspents []*wire.TxOut, coin cointype.CoinType) txauthor.InputSource {
	currentSKA := cointype.Zero()
	currentInputs := make([]*wire.TxIn, 0, len(unspents))
	redeemScriptSizes := make([]int, 0, len(unspents))
	f := func(_ dcrutil.Amount, targetSKA cointype.SKAAmount) (*txauthor.InputDetail, error) {
		for currentSKA.Cmp(targetSKA) < 0 && len(unspents) != 0 {
			u := unspents[0]
			unspents = unspents[1:]
			nextInput := wire.NewTxIn(&wire.OutPoint{}, 0, nil)
			nextInput.SKAValueIn = u.SKAValue
			currentSKA = currentSKA.Add(cointype.NewSKAAmount(u.SKAValue))
			currentInputs = append(currentInputs, nextInput)
			redeemScriptSizes = append(redeemScriptSizes, txsizes.RedeemP2PKHSigScriptSize)
		}
		return &txauthor.InputDetail{
			Amount:            0,
			SKAAmount:         currentSKA,
			Inputs:            currentInputs,
			Scripts:           make([][]byte, len(currentInputs)),
			RedeemScriptSizes: redeemScriptSizes,
		}, nil
	}
	_ = coin
	return txauthor.InputSource(f)
}
