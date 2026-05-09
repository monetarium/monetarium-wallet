// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txauthor_test

import (
	"strings"
	"testing"

	"github.com/monetarium/monetarium-wallet/errors"
	"github.com/monetarium/monetarium-wallet/wallet/txauthor"
	"github.com/monetarium/monetarium-wallet/wallet/txsizes"
	"github.com/monetarium/monetarium-node/chaincfg"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/dcrutil"
	"github.com/monetarium/monetarium-node/wire"
)

// TestNewUnsignedTransactionFeeGrowLoopBounded pins the L2 fix from the
// 2026-05-05 review: the iterative fee-grow loop in NewUnsignedTransaction
// must terminate even when the InputSource is non-monotonic in a way that
// causes maxRequiredFee to grow indefinitely. Without the iteration cap the
// loop spins forever; with it, the wallet returns errors.Invalid with an
// actionable message.
//
// The adversarial source below adds one input per call while nudging
// inputDetail.Amount forward by only 1 atom past the requested target. The
// fee cost of each additional input exceeds that 1-atom headroom, so the
// remainingAmount<maxRequiredFee branch fires every iteration and targetFee
// ratchets up without the loop ever satisfying the headroom check.
func TestNewUnsignedTransactionFeeGrowLoopBounded(t *testing.T) {
	// Adversarial input source: ignores the requested target's relationship
	// to a real UTXO set. On iteration N it returns 10*N inputs whose summed
	// Amount equals the requested target — passing the InsufficientBalance
	// check (>=) with zero headroom for the fee guard. The wallet's size
	// estimate at iteration N+1 sees 10 more inputs than at iteration N, so
	// the recomputed maxRequiredFee strictly exceeds the targetFee carried
	// over from the previous iteration. The loop cannot converge until the
	// iteration cap fires.
	const inputsPerIter = 10
	iter := 0
	source := txauthor.InputSource(func(target dcrutil.Amount, _ cointype.SKAAmount) (*txauthor.InputDetail, error) {
		iter++
		count := iter * inputsPerIter
		inputs := make([]*wire.TxIn, count)
		scripts := make([][]byte, count)
		sizes := make([]int, count)
		for i := 0; i < count; i++ {
			inputs[i] = wire.NewTxIn(&wire.OutPoint{}, 0, nil)
			sizes[i] = txsizes.RedeemP2PKHSigScriptSize
		}
		return &txauthor.InputDetail{
			Amount:            target,
			Inputs:            inputs,
			Scripts:           scripts,
			RedeemScriptSizes: sizes,
		}, nil
	})

	// Output: 1 VAR. Relay fee per kB: 10000 atoms (the wallet default,
	// txrules.DefaultRelayFeePerKb). Each new input costs ~1.08 atoms of
	// fee at this rate — but we add 108 bytes per iteration and only 1
	// atom of value, so the fee headroom never closes.
	outScript := make([]byte, txsizes.P2PKHOutputSize)
	outputs := []*wire.TxOut{wire.NewTxOut(int64(dcrutil.Amount(1e8)), outScript)}
	relayFee := cointype.SKAAmountFromInt64(10000)
	changeSource := AuthorTestChangeSource{}

	_, err := txauthor.NewUnsignedTransaction(outputs, relayFee, source, changeSource,
		chaincfg.MainNetParams().MaxTxSize, -1)
	if err == nil {
		t.Fatalf("non-monotonic input source must terminate with an error; got nil after %d iterations", iter)
	}
	// The iteration cap surfaces as errors.Invalid (errors package), not
	// errors.InsufficientBalance. Pin both the kind and that the message
	// names the loop, so future readers can correlate the failure with the
	// L2 fix.
	if !errors.Is(err, errors.Invalid) {
		t.Errorf("expected errors.Invalid; got %v", err)
	}
	if !strings.Contains(err.Error(), "fee-grow loop") {
		t.Errorf("error must reference fee-grow loop; got %q", err.Error())
	}
	// Sanity: the cap is small (constant maxFeeGrowIterations=32 in
	// author.go). 200 iterations would mean the cap is broken or absent.
	if iter > 200 {
		t.Errorf("loop ran %d iterations — cap is too generous or absent", iter)
	}
}
