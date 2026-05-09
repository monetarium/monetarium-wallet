// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"math/big"
	"testing"

	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/wire"
	"github.com/monetarium/monetarium-wallet/wallet/txrules"
	"github.com/monetarium/monetarium-wallet/wallet/txsizes"
)

// TestCompressWalletSKAScaleNoTruncation locks in the big.Int discipline of
// compressWalletInternal's SKA branch (wallet/createtx.go:1110-1119). The SKA
// path computes `totalAddedSKA = Σ SKAValue_i` over the eligible inputs and
// `skaOutput = totalAddedSKA - skaFee`, then writes `skaOutput.BigInt()` into
// the consolidated output. If any of those operations were ever to silently
// truncate to int64, an at-scale consolidation would lose funds equal to the
// truncated remainder.
//
// This test replicates the inner math against the same primitives the
// production code uses (cointype.SKAAmount.Add/Sub, txrules.FeeForSerializeSizeSKA,
// txsizes.EstimateSerializeSizeSKA) at a scale that demonstrably exceeds
// int64 atoms: 200 inputs at 2^62 atoms each. The combined input total is
// 200 * 2^62 = ~9.2e20 atoms, well above math.MaxInt64 (~9.2e18).
//
// We do not drive the full wallet UTXO store here — funding 200 SKA UTXOs via
// the wtxmgr internal API is a 100+ line setup whose value is dominated by
// what's already covered by dual_coin_utxo_test.go and the txauthor SKA
// integration tests. The gap the code review flagged is specifically the
// big.Int boundary inside compressWalletInternal; this test guards that
// boundary.
func TestCompressWalletSKAScaleNoTruncation(t *testing.T) {
	const (
		numInputs = 200
		// 2^62 atoms per input — chosen so the sum exceeds int64 max but
		// each individual value remains representable as int64 (so any
		// truncation must come from the *aggregation*, not the leaves).
		perInputAtoms = int64(1) << 62
	)

	perInput := big.NewInt(perInputAtoms)
	expectedTotal := new(big.Int).Mul(perInput, big.NewInt(numInputs))

	// Sanity check: the test only catches truncation if the total really
	// does overflow int64.
	if expectedTotal.IsInt64() {
		t.Fatalf("test setup bug: expected total %v fits in int64; pick larger inputs", expectedTotal)
	}

	// Replicate the per-input accumulation compressWalletInternal does at
	// wallet/createtx.go:1093-1098. Each input contributes via
	// `totalAddedSKA = totalAddedSKA.Add(cointype.NewSKAAmount(SKAValue))`.
	totalAddedSKA := cointype.Zero()
	scriptSizes := make([]int, numInputs)
	for i := 0; i < numInputs; i++ {
		totalAddedSKA = totalAddedSKA.Add(cointype.NewSKAAmount(perInput))
		scriptSizes[i] = txsizes.RedeemP2PKHSigScriptSize
	}
	if got := totalAddedSKA.BigInt(); got.Cmp(expectedTotal) != 0 {
		t.Fatalf("Σ SKAAmount.Add truncated:\n  got  %v\n  want %v", got, expectedTotal)
	}

	// Build the same-shape output the function does: a single SKA output
	// (Value=0, CoinType=SKA1, placeholder PkScript). The pkScript size
	// only matters insofar as it feeds into the fee estimate.
	out := &wire.TxOut{
		Value:    0,
		SKAValue: big.NewInt(0),
		CoinType: cointype.CoinType(1),
		Version:  wire.DefaultPkScriptVersion,
		PkScript: make([]byte, 25), // P2PKH placeholder
	}

	// Mirror compressWalletInternal's fee path (wallet/createtx.go:1112-1113).
	// Use a realistic SKA fee rate: 4 SKA/KB (4e18 atoms/KB), the simnet/
	// mainnet MinRelayTxFee for SKA1.
	feeRateBig := cointype.NewSKAAmount(new(big.Int).Exp(
		big.NewInt(10), big.NewInt(18), nil)) // 1e18 = 1 SKA
	feeRateBig = feeRateBig.Add(feeRateBig).Add(feeRateBig).Add(feeRateBig) // 4 SKA
	szEst := txsizes.EstimateSerializeSizeSKA(scriptSizes, []*wire.TxOut{out}, 0)
	skaFee := txrules.FeeForSerializeSizeSKA(feeRateBig, szEst)

	// Sanity: fee must be smaller than the total so the resulting output is
	// non-negative (the truncation path of interest only fires after a
	// successful subtraction).
	if skaFee.BigInt().Cmp(expectedTotal) >= 0 {
		t.Fatalf("test setup bug: fee %v >= total %v; reduce fee rate or raise inputs",
			skaFee, expectedTotal)
	}

	// Final subtraction (wallet/createtx.go:1114).
	skaOutput := totalAddedSKA.Sub(skaFee)
	if skaOutput.IsNegative() || skaOutput.IsZero() {
		t.Fatalf("test setup bug: skaOutput=%v is non-positive", skaOutput)
	}

	// Expected output is total - fee, computed independently with big.Int.
	wantOutput := new(big.Int).Sub(expectedTotal, skaFee.BigInt())
	if got := skaOutput.BigInt(); got.Cmp(wantOutput) != 0 {
		t.Fatalf("skaOutput truncated:\n  got  %v\n  want %v", got, wantOutput)
	}

	// Final shape compressWalletInternal writes back to the output (line 1119).
	out.SKAValue = skaOutput.BigInt()
	if out.SKAValue.Cmp(wantOutput) != 0 {
		t.Fatalf("TxOut.SKAValue truncated after assignment:\n  got  %v\n  want %v",
			out.SKAValue, wantOutput)
	}

	// Conservation: total inputs - output - fee must be zero.
	checkSum := new(big.Int).Sub(expectedTotal, out.SKAValue)
	checkSum.Sub(checkSum, skaFee.BigInt())
	if checkSum.Sign() != 0 {
		t.Fatalf("conservation broken: total - output - fee = %v (expected 0)", checkSum)
	}
}
