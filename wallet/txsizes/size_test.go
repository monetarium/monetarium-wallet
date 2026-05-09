package txsizes_test

import (
	"bytes"
	"testing"

	. "github.com/monetarium/monetarium-wallet/wallet/txsizes"
	"github.com/monetarium/monetarium-node/chaincfg/chainhash"
	"github.com/monetarium/monetarium-node/wire"
)

const (
	p2pkhScriptSize = P2PKHPkScriptSize
	p2shScriptSize  = 23
)

func makeScriptSizes(count int, size int) *[]int {
	scriptSizes := make([]int, count)
	for idx := 0; idx < count; idx++ {
		scriptSizes[idx] = size
	}
	return &scriptSizes
}

func makeInts(value int, n int) []int {
	v := make([]int, n)
	for i := range v {
		v[i] = value
	}
	return v
}

func TestEstimateSerializeSize(t *testing.T) {
	tests := []struct {
		InputScriptSizes     []int
		OutputScriptLengths  []int
		ChangeScriptSize     int
		ExpectedSizeEstimate int
	}{
		// Updated expected values to account for:
		// - 1-byte CoinType field per output (V12)
		// - 1-byte SKAValueInLen field per input witness (V13)
		0: {[]int{RedeemP2PKHSigScriptSize}, []int{}, 0, 182},                              // +1 for SKAValueInLen
		1: {[]int{RedeemP2PKHSigScriptSize}, []int{p2pkhScriptSize}, 0, 219},               // +1 CoinType, +1 SKAValueInLen
		2: {[]int{RedeemP2PKHSigScriptSize}, []int{}, p2pkhScriptSize, 219},                // +1 CoinType in change, +1 SKAValueInLen
		3: {[]int{RedeemP2PKHSigScriptSize}, []int{p2pkhScriptSize}, p2pkhScriptSize, 256}, // +2 CoinType, +1 SKAValueInLen
		4: {[]int{RedeemP2PKHSigScriptSize}, []int{p2shScriptSize}, 0, 217},                // +1 CoinType, +1 SKAValueInLen
		5: {[]int{RedeemP2PKHSigScriptSize}, []int{p2shScriptSize}, p2pkhScriptSize, 254},  // +2 CoinType, +1 SKAValueInLen

		6:  {[]int{RedeemP2PKHSigScriptSize, RedeemP2PKHSigScriptSize}, []int{}, 0, 349},                              // +2 SKAValueInLen
		7:  {[]int{RedeemP2PKHSigScriptSize, RedeemP2PKHSigScriptSize}, []int{p2pkhScriptSize}, 0, 386},               // +1 CoinType, +2 SKAValueInLen
		8:  {[]int{RedeemP2PKHSigScriptSize, RedeemP2PKHSigScriptSize}, []int{}, p2pkhScriptSize, 386},                // +1 CoinType in change, +2 SKAValueInLen
		9:  {[]int{RedeemP2PKHSigScriptSize, RedeemP2PKHSigScriptSize}, []int{p2pkhScriptSize}, p2pkhScriptSize, 423}, // +2 CoinType, +2 SKAValueInLen
		10: {[]int{RedeemP2PKHSigScriptSize, RedeemP2PKHSigScriptSize}, []int{p2shScriptSize}, 0, 384},                // +1 CoinType, +2 SKAValueInLen
		11: {[]int{RedeemP2PKHSigScriptSize, RedeemP2PKHSigScriptSize}, []int{p2shScriptSize}, p2pkhScriptSize, 421},  // +2 CoinType, +2 SKAValueInLen

		// 0xfd is discriminant for 16-bit compact ints, compact int
		// total size increases from 1 byte to 3.
		12: {[]int{RedeemP2PKHSigScriptSize}, makeInts(p2pkhScriptSize, 0xfc), 0, 9506},               // +252 CoinType, +1 SKAValueInLen
		13: {[]int{RedeemP2PKHSigScriptSize}, makeInts(p2pkhScriptSize, 0xfd), 0, 9545},               // +253 CoinType, +1 SKAValueInLen
		14: {[]int{RedeemP2PKHSigScriptSize}, makeInts(p2pkhScriptSize, 0xfc), p2pkhScriptSize, 9545}, // +253 CoinType, +1 SKAValueInLen
		15: {*makeScriptSizes(0xfc, RedeemP2PKHSigScriptSize), []int{}, 0, 42099},                     // +252 SKAValueInLen
		16: {*makeScriptSizes(0xfd, RedeemP2PKHSigScriptSize), []int{}, 0, 42270},                     // +253 SKAValueInLen (0xfd inputs * 1 byte each)
	}
	for i, test := range tests {
		outputs := make([]*wire.TxOut, 0, len(test.OutputScriptLengths))
		for _, l := range test.OutputScriptLengths {
			outputs = append(outputs, &wire.TxOut{PkScript: make([]byte, l)})
		}
		actualEstimate := EstimateSerializeSize(test.InputScriptSizes, outputs, test.ChangeScriptSize)
		if actualEstimate != test.ExpectedSizeEstimate {
			t.Errorf("Test %d: Got %v: Expected %v", i, actualEstimate, test.ExpectedSizeEstimate)
		}
	}
}

// TestEstimateInputSizeRoundTrip asserts that EstimateInputSize matches the
// actual serialized size of a TxIn in the V13 wire format, including the
// SKAValueInLen byte that is always present (0 for VAR inputs). Catches the
// off-by-one regression where the legacy estimator forgot the SKAValueInLen
// byte and consequently underestimated fees by 1 byte per input.
func TestEstimateInputSizeRoundTrip(t *testing.T) {
	scriptSizes := []int{
		0,
		1,
		RedeemP2PKHSigScriptSize,
		RedeemP2SHSigScriptSize,
		0xfc,
		0xfd, // varint discriminant boundary
		0x100,
	}
	for _, sz := range scriptSizes {
		sigScript := bytes.Repeat([]byte{0x01}, sz)
		txIn := wire.NewTxIn(&wire.OutPoint{Hash: chainhash.Hash{}, Index: 0, Tree: 0}, 0, sigScript)
		// Full-input wire size = prefix + witness (V13).
		actual := txIn.SerializeSizePrefix() + txIn.SerializeSizeWitness()
		estimated := EstimateInputSize(sz)
		if estimated != actual {
			t.Errorf("scriptSize=%d: EstimateInputSize=%d, actual prefix+witness=%d (delta %d)",
				sz, estimated, actual, estimated-actual)
		}
	}
}
