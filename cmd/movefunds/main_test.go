// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/monetarium/monetarium-wallet/rpc/jsonrpc/types"
	"github.com/monetarium/monetarium-wallet/wallet/txsizes"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/wire"
)

// TestMakeInputSourceVARPopulatesRedeemScriptSizes is the regression test for
// a bug where the per-input redeem-script size was never appended, causing
// txauthor.NewUnsignedTransaction to under-estimate the signed tx size by
// ~108 bytes per input — producing transactions that the relay rejects for
// low fees once the input count grows past a few UTXOs.
func TestMakeInputSourceVARPopulatesRedeemScriptSizes(t *testing.T) {
	const validTxID = "0000000000000000000000000000000000000000000000000000000000000001"
	utxos := []types.ListUnspentResult{
		{TxID: validTxID, Vout: 0, Tree: 0, Amount: "0.5", CoinType: 0},
		{TxID: validTxID, Vout: 1, Tree: 0, Amount: "1.0", CoinType: 0},
		{TxID: validTxID, Vout: 2, Tree: 0, Amount: "2.0", CoinType: 0},
	}

	atomsPerCoin := big.NewInt(cointype.AtomsPerVAR)
	_, src, _ := makeInputSource(utxos, cointype.CoinTypeVAR, atomsPerCoin)

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
	const validTxID = "0000000000000000000000000000000000000000000000000000000000000002"
	utxos := []types.ListUnspentResult{
		{TxID: validTxID, Vout: 0, Tree: 0, Amount: "1.0", CoinType: 1},
		{TxID: validTxID, Vout: 1, Tree: 0, Amount: "2.5", CoinType: 1},
	}

	// SKA uses 1e18 atoms per coin in the live mainnet config, but the
	// makeInputSource function is scale-agnostic — pass an explicit value
	// so the test does not depend on chain params.
	atomsPerCoin := new(big.Int)
	atomsPerCoin.Exp(big.NewInt(10), big.NewInt(18), nil)

	_, src, _ := makeInputSource(utxos, cointype.CoinType(1), atomsPerCoin)

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

// TestMakeInputSourcePrevScriptsCarriesPkScript locks in MED-4: the helper
// that generates the signrawtransaction-inputs JSON must emit the canonical
// scriptPubKey of the previous output, not the (empty) SignatureScript of
// the unsigned tx. The script values flow from each ListUnspentResult into
// the prevScripts map keyed by OutPoint.
func TestMakeInputSourcePrevScriptsCarriesPkScript(t *testing.T) {
	const (
		validTxID    = "0000000000000000000000000000000000000000000000000000000000000003"
		varPkScript  = "76a914aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa88ac"
		skaPkScript  = "76a914bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb88ac"
		skaRedeemSc  = "522102deadbeef02103abad1deaf52ae"
	)

	t.Run("VAR", func(t *testing.T) {
		utxos := []types.ListUnspentResult{
			{TxID: validTxID, Vout: 7, Tree: 0, Amount: "0.5", CoinType: 0,
				ScriptPubKey: varPkScript},
		}
		_, _, prev := makeInputSource(utxos, cointype.CoinTypeVAR,
			big.NewInt(cointype.AtomsPerVAR))
		if got, want := len(prev), 1; got != want {
			t.Fatalf("prevScripts size: got %d, want %d", got, want)
		}
		for op, ps := range prev {
			if ps.PkScript != varPkScript {
				t.Errorf("prevScripts[%v].PkScript = %q, want %q",
					op, ps.PkScript, varPkScript)
			}
			if ps.RedeemScript != "" {
				t.Errorf("prevScripts[%v].RedeemScript = %q, want \"\"",
					op, ps.RedeemScript)
			}
		}
	})

	t.Run("SKA-with-redeem-script", func(t *testing.T) {
		atomsPerCoin := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
		utxos := []types.ListUnspentResult{
			{TxID: validTxID, Vout: 9, Tree: 0, Amount: "1.0", CoinType: 1,
				ScriptPubKey: skaPkScript, RedeemScript: skaRedeemSc},
		}
		_, _, prev := makeInputSource(utxos, cointype.CoinType(1), atomsPerCoin)
		if got, want := len(prev), 1; got != want {
			t.Fatalf("prevScripts size: got %d, want %d", got, want)
		}
		for op, ps := range prev {
			if ps.PkScript != skaPkScript {
				t.Errorf("prevScripts[%v].PkScript = %q, want %q",
					op, ps.PkScript, skaPkScript)
			}
			if ps.RedeemScript != skaRedeemSc {
				t.Errorf("prevScripts[%v].RedeemScript = %q, want %q",
					op, ps.RedeemScript, skaRedeemSc)
			}
		}
	})
}

// TestSignInputJSONUsesCanonicalFieldNames asserts that the on-disk JSON
// emitted into the helper sign.sh script uses the canonical
// signrawtransaction RawTxInput field names — scriptPubKey/redeemScript with
// camelCase — and that the values match the previous output's scripts (not
// the empty SignatureScript of the unsigned tx). This is a structural test
// against the signInput struct definition rather than the full main()
// pipeline so it stays hermetic.
func TestSignInputJSONUsesCanonicalFieldNames(t *testing.T) {
	type signInput struct {
		Txid         string `json:"txid"`
		Vout         uint32 `json:"vout"`
		Tree         int8   `json:"tree"`
		ScriptPubKey string `json:"scriptPubKey"`
		RedeemScript string `json:"redeemScript"`
		SKAValueIn   string `json:"skaValueIn,omitempty"`
	}

	t.Run("VAR-omits-skaValueIn", func(t *testing.T) {
		in := signInput{
			Txid:         "deadbeef",
			Vout:         3,
			Tree:         0,
			ScriptPubKey: "76a914cccc88ac",
			RedeemScript: "522102deadbeef52ae",
			// SKAValueIn left empty — VAR inputs.
		}
		b, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		js := string(b)
		if !strings.Contains(js, `"scriptPubKey":`) {
			t.Errorf("expected canonical scriptPubKey field name; got %s", js)
		}
		if !strings.Contains(js, `"redeemScript":`) {
			t.Errorf("expected canonical redeemScript field name; got %s", js)
		}
		// Lowercase legacy field names must NOT appear — a typo regression
		// here silently broke air-gapped flows previously.
		if strings.Contains(js, `"scriptpubkey":`) {
			t.Errorf("legacy scriptpubkey field must not appear; got %s", js)
		}
		if strings.Contains(js, `"redeemscript":`) {
			t.Errorf("legacy redeemscript field must not appear; got %s", js)
		}
		// VAR flow: skaValueIn is empty → omitempty must drop it.
		if strings.Contains(js, `"skaValueIn"`) {
			t.Errorf("skaValueIn must be omitted for VAR inputs; got %s", js)
		}
	})

	t.Run("SKA-emits-skaValueIn", func(t *testing.T) {
		in := signInput{
			Txid:         "deadbeef",
			Vout:         3,
			Tree:         0,
			ScriptPubKey: "76a914cccc88ac",
			RedeemScript: "",
			SKAValueIn:   "1234.000000000000000000",
		}
		b, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		js := string(b)
		if !strings.Contains(js, `"skaValueIn":"1234.000000000000000000"`) {
			t.Errorf("expected canonical skaValueIn field name with value; got %s", js)
		}
		// Snake_case and lowercased aliases must NOT appear.
		if strings.Contains(js, `"ska_value_in"`) {
			t.Errorf("legacy ska_value_in field must not appear; got %s", js)
		}
		if strings.Contains(js, `"skavaluein"`) {
			t.Errorf("legacy skavaluein field must not appear; got %s", js)
		}
	})
}

// TestFormatSKAValueInRoundTrip locks in the wire-format contract for the
// air-gapped SKA signing helper: skaValueIn must be a decimal coin string
// (e.g. "1.234567890123456789"), not raw atoms. The wallet-side parser
// (populateSKAValueIn → coinsToAtomsBig) multiplies the value by
// atomsPerCoin, so emitting "1000000000000000000" (atoms) for a 1-SKA input
// would yield 1e36 atoms downstream — a 1e18× over-assertion that fails the
// node's SKAValueIn fraud-proof check.
//
// This test was added in response to the HIGH finding in the 2026-05-05
// monetarium-wallet review: cmd/movefunds previously emitted in.SKAValueIn
// .String() (atoms) instead of converting via cointype.AtomsToDecimalString.
func TestFormatSKAValueInRoundTrip(t *testing.T) {
	atomsPerCoin := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil) // 1e18

	t.Run("VAR-input-returns-empty", func(t *testing.T) {
		in := wire.NewTxIn(&wire.OutPoint{}, 100000000, nil)
		got := formatSKAValueIn(in, false /*isSKA*/, big.NewInt(cointype.AtomsPerVAR))
		if got != "" {
			t.Errorf("VAR input must produce empty skaValueIn; got %q", got)
		}
	})

	t.Run("SKA-input-with-nil-SKAValueIn-returns-empty", func(t *testing.T) {
		in := wire.NewTxIn(&wire.OutPoint{}, 0, nil)
		// SKAValueIn left nil.
		got := formatSKAValueIn(in, true /*isSKA*/, atomsPerCoin)
		if got != "" {
			t.Errorf("nil SKAValueIn must produce empty string; got %q", got)
		}
	})

	t.Run("SKA1-coin-emits-decimal-coin-string-not-atoms", func(t *testing.T) {
		in := wire.NewTxIn(&wire.OutPoint{}, 0, nil)
		in.SKAValueIn = new(big.Int).Set(atomsPerCoin) // 1 SKA = 1e18 atoms
		got := formatSKAValueIn(in, true, atomsPerCoin)

		// The bug produced "1000000000000000000" (atoms-as-string). The
		// fix produces a decimal coin string the wallet can parse with
		// coinsToAtomsBig.
		if got == in.SKAValueIn.String() {
			t.Fatalf("regression: skaValueIn must be a decimal coin "+
				"string, not raw atoms; got %q (which equals "+
				"the raw atom value — wire-format mismatch)", got)
		}

		// Round-trip via the wallet-side parser.
		roundTripped, err := cointype.DecimalStringToAtoms(got, atomsPerCoin)
		if err != nil {
			t.Fatalf("DecimalStringToAtoms(%q) failed: %v", got, err)
		}
		if roundTripped.Cmp(in.SKAValueIn) != 0 {
			t.Fatalf("round-trip mismatch: emitted %q, parsed back to %v atoms, "+
				"want %v atoms", got, roundTripped, in.SKAValueIn)
		}
	})

	t.Run("SKA-fractional-atoms-round-trip", func(t *testing.T) {
		// 1.234567890123456789 SKA — exercises full 1e18 precision.
		atomsStr := "1234567890123456789"
		atoms, _ := new(big.Int).SetString(atomsStr, 10)

		in := wire.NewTxIn(&wire.OutPoint{}, 0, nil)
		in.SKAValueIn = atoms
		got := formatSKAValueIn(in, true, atomsPerCoin)

		roundTripped, err := cointype.DecimalStringToAtoms(got, atomsPerCoin)
		if err != nil {
			t.Fatalf("DecimalStringToAtoms(%q) failed: %v", got, err)
		}
		if roundTripped.Cmp(atoms) != 0 {
			t.Fatalf("round-trip mismatch: emitted %q, parsed back to %v atoms, "+
				"want %v atoms", got, roundTripped, atoms)
		}
	})

	t.Run("SKA-non-1e18-AtomsPerCoin-scale", func(t *testing.T) {
		// Defends against a future SKA coin with a non-default
		// AtomsPerCoin (the chain-params validator permits any
		// power-of-10). 1 coin in a 1e10-scale system is 1e10 atoms.
		altAtomsPerCoin := big.NewInt(int64(1e10))
		in := wire.NewTxIn(&wire.OutPoint{}, 0, nil)
		in.SKAValueIn = new(big.Int).Set(altAtomsPerCoin)
		got := formatSKAValueIn(in, true, altAtomsPerCoin)

		roundTripped, err := cointype.DecimalStringToAtoms(got, altAtomsPerCoin)
		if err != nil {
			t.Fatalf("DecimalStringToAtoms(%q) failed: %v", got, err)
		}
		if roundTripped.Cmp(in.SKAValueIn) != 0 {
			t.Fatalf("round-trip mismatch under alt atomsPerCoin: emitted %q, "+
				"parsed back to %v, want %v", got, roundTripped, in.SKAValueIn)
		}
	})
}
