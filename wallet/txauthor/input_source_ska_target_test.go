// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txauthor_test

import (
	"math/big"
	"testing"

	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/dcrutil"
	"github.com/monetarium/monetarium-node/wire"

	"github.com/monetarium/monetarium-wallet/wallet/txauthor"
	"github.com/monetarium/monetarium-wallet/wallet/txsizes"
)

// TestSKAInputSourceStopsAtTarget locks in HIGH-severity review fix #2: SKA
// transactions must stop consuming UTXOs once the big.Int target is met,
// rather than dragging in the entire account's SKA bucket. The old code
// passed target=0 to the input source to mean "collect everything", causing
// every SKA send to co-spend all the account's SKA UTXOs.
func TestSKAInputSourceStopsAtTarget(t *testing.T) {
	const coinTypeSKA = cointype.CoinType(1)

	// 100 unspents of 10 atoms each = 1000 atoms total.
	unspents := make([]*wire.TxOut, 100)
	for i := range unspents {
		unspents[i] = &wire.TxOut{
			Value:    0,
			SKAValue: big.NewInt(10),
			CoinType: coinTypeSKA,
			PkScript: make([]byte, txsizes.P2PKHOutputSize),
		}
	}

	// Track how many unspents the source was asked for before it returned.
	var consumedByFetch int
	fetchInputs := func(target dcrutil.Amount, targetSKA cointype.SKAAmount) (*txauthor.InputDetail, error) {
		// Walk unspents in order, accumulating into currentSKATotal until
		// the caller-provided big.Int target is satisfied.
		currentSKATotal := cointype.Zero()
		var inputs []*wire.TxIn
		var redeemScriptSizes []int
		for _, u := range unspents[consumedByFetch:] {
			if !targetSKA.IsZero() && currentSKATotal.Cmp(targetSKA) >= 0 {
				break
			}
			in := wire.NewTxIn(&wire.OutPoint{}, 0, nil)
			in.SKAValueIn = new(big.Int).Set(u.SKAValue)
			inputs = append(inputs, in)
			currentSKATotal = currentSKATotal.Add(cointype.NewSKAAmount(u.SKAValue))
			redeemScriptSizes = append(redeemScriptSizes, txsizes.RedeemP2PKHSigScriptSize)
			consumedByFetch++
		}
		return &txauthor.InputDetail{
			SKAAmount:         currentSKATotal,
			Inputs:            inputs,
			Scripts:           make([][]byte, len(inputs)),
			RedeemScriptSizes: redeemScriptSizes,
		}, nil
	}

	// Target output = 100 atoms. With the default fee, the author should
	// need ~110 atoms of inputs — nowhere near all 1000.
	outputs := []*wire.TxOut{{
		Value:    0,
		SKAValue: big.NewInt(100),
		CoinType: coinTypeSKA,
		PkScript: make([]byte, txsizes.P2PKHOutputSize),
	}}

	// Small relay fee so the overhead stays modest.
	relayFee := cointype.SKAAmountFromInt64(1)

	authored, err := txauthor.NewUnsignedTransaction(outputs, relayFee,
		fetchInputs, testChangeSource{}, 100_000, -1)
	if err != nil {
		t.Fatalf("NewUnsignedTransaction: %v", err)
	}

	// The key assertion: we must NOT have pulled every UTXO. The bug being
	// guarded against would always produce 100 inputs.
	if got := len(authored.Tx.TxIn); got >= len(unspents) {
		t.Fatalf("SKA input source consumed %d of %d UTXOs — regression to "+
			"'target=0 means collect all' semantics", got, len(unspents))
	}

	// Sanity: the tx must still have enough SKA input to cover its outputs.
	outSum := cointype.Zero()
	for _, o := range authored.Tx.TxOut {
		if o.SKAValue != nil {
			outSum = outSum.Add(cointype.NewSKAAmount(o.SKAValue))
		}
	}
	if authored.SKATotalInput.Cmp(outSum) < 0 {
		t.Fatalf("SKA input %v less than output sum %v",
			authored.SKATotalInput, outSum)
	}
}

type testChangeSource struct{}

func (testChangeSource) Script() ([]byte, uint16, error) {
	return make([]byte, 25), 0, nil
}
func (testChangeSource) ScriptSize() int { return 25 }
