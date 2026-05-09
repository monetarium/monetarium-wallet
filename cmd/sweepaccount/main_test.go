// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"math/big"
	"testing"

	"github.com/monetarium/monetarium-wallet/rpc/jsonrpc/types"
	"github.com/monetarium/monetarium-wallet/wallet/txsizes"
	"github.com/monetarium/monetarium-node/cointype"
)

// TestMakeInputSourceVARPopulatesRedeemScriptSizes is the regression test for
// a bug where the per-input redeem-script size was never appended, causing
// txauthor.NewUnsignedTransaction to under-estimate the signed tx size by
// ~108 bytes per input — producing transactions that the relay rejects for
// low fees once the input count grows past a few UTXOs.
func TestMakeInputSourceVARPopulatesRedeemScriptSizes(t *testing.T) {
	const validTxID = "0000000000000000000000000000000000000000000000000000000000000003"
	utxos := []types.ListUnspentResult{
		{TxID: validTxID, Vout: 0, Tree: 0, Amount: "0.5", CoinType: 0},
		{TxID: validTxID, Vout: 1, Tree: 0, Amount: "1.0", CoinType: 0},
		{TxID: validTxID, Vout: 2, Tree: 0, Amount: "2.0", CoinType: 0},
	}

	atomsPerCoin := big.NewInt(int64(cointype.AtomsPerVAR))
	src, _ := makeInputSource(utxos, cointype.CoinTypeVAR, atomsPerCoin)

	detail, err := src(0, cointype.Zero())
	if err != nil {
		t.Fatalf("input source returned error: %v", err)
	}
	if got, want := len(detail.Inputs), len(utxos); got != want {
		t.Fatalf("inputs: got %d, want %d", got, want)
	}
	if got, want := len(detail.RedeemScriptSizes), len(utxos); got != want {
		t.Fatalf("RedeemScriptSizes count: got %d, want %d (one per input)", got, want)
	}
	for i, sz := range detail.RedeemScriptSizes {
		if sz != txsizes.RedeemP2PKHSigScriptSize {
			t.Errorf("RedeemScriptSizes[%d] = %d, want %d", i, sz, txsizes.RedeemP2PKHSigScriptSize)
		}
	}
}

// TestMakeInputSourceSKAPopulatesRedeemScriptSizes is the SKA twin of the
// above — same bug pattern, different code branch.
func TestMakeInputSourceSKAPopulatesRedeemScriptSizes(t *testing.T) {
	const validTxID = "0000000000000000000000000000000000000000000000000000000000000004"
	utxos := []types.ListUnspentResult{
		{TxID: validTxID, Vout: 0, Tree: 0, Amount: "1.0", CoinType: 1},
		{TxID: validTxID, Vout: 1, Tree: 0, Amount: "2.5", CoinType: 1},
	}

	atomsPerCoin := new(big.Int)
	atomsPerCoin.Exp(big.NewInt(10), big.NewInt(18), nil)

	src, _ := makeInputSource(utxos, cointype.CoinType(1), atomsPerCoin)

	detail, err := src(0, cointype.Zero())
	if err != nil {
		t.Fatalf("input source returned error: %v", err)
	}
	if got, want := len(detail.RedeemScriptSizes), len(utxos); got != want {
		t.Fatalf("RedeemScriptSizes count: got %d, want %d (one per input)", got, want)
	}
	for i, sz := range detail.RedeemScriptSizes {
		if sz != txsizes.RedeemP2PKHSigScriptSize {
			t.Errorf("RedeemScriptSizes[%d] = %d, want %d", i, sz, txsizes.RedeemP2PKHSigScriptSize)
		}
	}
}
