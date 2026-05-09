// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txrules_test

import (
	"math/big"
	"strings"
	"testing"

	"github.com/monetarium/monetarium-node/chaincfg"
	"github.com/monetarium/monetarium-node/chaincfg/chainhash"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/wire"

	"github.com/monetarium/monetarium-wallet/errors"
	"github.com/monetarium/monetarium-wallet/wallet/txrules"
)

// buildSKATxWithOverpaidFee constructs a one-input one-output SKA tx whose
// SKA input value dwarfs the output value by more than MaxFeeMultiplier ×
// MinRelayTxFee × txSize / 1000 — i.e. clearly over-paying fees. It uses the
// live mainnet chaincfg so the per-coin fee config matches production.
func buildSKATxWithOverpaidFee(t *testing.T, coinType cointype.CoinType) (*wire.MsgTx, *chaincfg.Params) {
	t.Helper()
	params := chaincfg.MainNetParams()
	cfg, ok := params.SKACoins[coinType]
	if !ok {
		t.Fatalf("no SKACoins config for coin type %d in mainnet params", coinType)
	}
	if cfg.MinRelayTxFee == nil {
		t.Fatalf("MinRelayTxFee not configured for coin type %d", coinType)
	}

	// Mainnet SKA1 has MinRelayTxFee=4e18 atoms/KB and
	// MaxFeeMultiplier=2500, so the ceiling is ~1e22 atoms per KB.
	// Overpay by 100× that ceiling to make the assertion unambiguous
	// regardless of the exact tx size.
	tx := wire.NewMsgTx()
	in := wire.NewTxIn(&wire.OutPoint{Index: 0}, 0, nil)
	overpay := new(big.Int).Mul(cfg.MinRelayTxFee, big.NewInt(cfg.MaxFeeMultiplier))
	overpay.Mul(overpay, big.NewInt(100))
	in.SKAValueIn = overpay
	tx.AddTxIn(in)

	out := &wire.TxOut{
		Value:    0,
		SKAValue: big.NewInt(1),
		CoinType: coinType,
		Version:  wire.DefaultPkScriptVersion,
		PkScript: make([]byte, 25), // P2PKH script placeholder
	}
	tx.AddTxOut(out)
	return tx, params
}

// TestTxPaysHighFeesVARRejectsSKATx asserts the runtime guard added to
// TxPaysHighFees: when the tx contains any SKA output, the function must
// return an Invalid error rather than silently sum the int64 ValueIn fields
// (which are ~always 0 for SKA inputs) and produce a meaningless answer.
// Callers must dispatch to TxPaysHighFeesSKA for SKA transactions instead.
func TestTxPaysHighFeesVARRejectsSKATx(t *testing.T) {
	tx, _ := buildSKATxWithOverpaidFee(t, cointype.CoinType(1))

	_, err := txrules.TxPaysHighFees(tx)
	if err == nil {
		t.Fatal("TxPaysHighFees must return an error for SKA outputs (VAR-only guard)")
	}
}

// TestTxPaysHighFeesSKAFlagsOverpaid exercises the correct dispatch: the
// SKA-aware check catches the overpaying tx.
func TestTxPaysHighFeesSKAFlagsOverpaid(t *testing.T) {
	tx, params := buildSKATxWithOverpaidFee(t, cointype.CoinType(1))

	highFees, err := txrules.TxPaysHighFeesSKA(tx, params)
	if err != nil {
		t.Fatalf("TxPaysHighFeesSKA: %v", err)
	}
	if !highFees {
		t.Fatalf("TxPaysHighFeesSKA failed to flag a tx overpaying by 1e12 atoms")
	}
}

// TestTxPaysHighFeesSKAPassesReasonableFee confirms the SKA check does not
// false-positive on a sanely-fee'd tx.
func TestTxPaysHighFeesSKAPassesReasonableFee(t *testing.T) {
	params := chaincfg.MainNetParams()
	coinType := cointype.CoinType(1)
	cfg, ok := params.SKACoins[coinType]
	if !ok {
		t.Fatalf("no SKACoins config for coin type %d", coinType)
	}

	tx := wire.NewMsgTx()
	in := wire.NewTxIn(&wire.OutPoint{Index: 0}, 0, nil)
	// Input = output + (MinRelayTxFee × 1.5) — modest fee, well under the
	// MaxFeeMultiplier threshold (default 2500×).
	outputAtoms := big.NewInt(1_000_000_000)
	modestFee := new(big.Int).Mul(cfg.MinRelayTxFee, big.NewInt(3))
	modestFee.Div(modestFee, big.NewInt(2))
	in.SKAValueIn = new(big.Int).Add(outputAtoms, modestFee)
	tx.AddTxIn(in)

	out := &wire.TxOut{
		Value:    0,
		SKAValue: new(big.Int).Set(outputAtoms),
		CoinType: coinType,
		Version:  wire.DefaultPkScriptVersion,
		PkScript: make([]byte, 25),
	}
	tx.AddTxOut(out)

	highFees, err := txrules.TxPaysHighFeesSKA(tx, params)
	if err != nil {
		t.Fatalf("TxPaysHighFeesSKA: %v", err)
	}
	if highFees {
		t.Fatalf("TxPaysHighFeesSKA false-positive on a modestly-fee'd tx")
	}
}

// TestGetCoinTypeFromOutputsSelectsSKA locks the dispatch predicate used by
// sendrawtransaction: the first output's coin type drives the choice between
// TxPaysHighFees and TxPaysHighFeesSKA. (The consensus rule that all outputs
// must share a coin type is enforced elsewhere.)
func TestGetCoinTypeFromOutputsSelectsSKA(t *testing.T) {
	outs := []*wire.TxOut{{
		CoinType: cointype.CoinType(1),
		SKAValue: big.NewInt(1),
	}}
	if !txrules.GetCoinTypeFromOutputs(outs).IsSKA() {
		t.Fatal("first-SKA output must yield IsSKA()==true")
	}

	outsVar := []*wire.TxOut{{
		CoinType: cointype.CoinTypeVAR,
		Value:    1,
	}}
	if txrules.GetCoinTypeFromOutputs(outsVar).IsSKA() {
		t.Fatal("VAR-only outputs must yield IsSKA()==false")
	}
}

// buildEmissionTx constructs a minimal SKA emission transaction with the same
// structure that createAuthorizedSKAEmissionTransaction produces: one null
// input, [SKA] marker in the signature script, and one SKA output. The lone
// input has SKAValueIn = nil because no SKA exists yet to consume.
func buildEmissionTx(coinType cointype.CoinType) *wire.MsgTx {
	return &wire.MsgTx{
		SerType: wire.TxSerializeFull,
		Version: 1,
		TxIn: []*wire.TxIn{{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{},
				Index: 0xffffffff,
				Tree:  wire.TxTreeRegular,
			},
			SignatureScript: []byte{0x01, 0x53, 0x4b, 0x41},
			Sequence:        0xffffffff,
			BlockHeight:     wire.NullBlockHeight,
			BlockIndex:      wire.NullBlockIndex,
			ValueIn:         wire.NullValueIn,
			// SKAValueIn deliberately left nil — emission inputs cannot
			// reference any pre-existing SKA value.
		}},
		TxOut: []*wire.TxOut{{
			Value:    0,
			SKAValue: big.NewInt(1_000_000_000),
			CoinType: coinType,
			Version:  wire.DefaultPkScriptVersion,
		}},
	}
}

// TestTxPaysHighFeesSKAExemptsEmissionTx is the regression test for the bug
// where sendrawtransaction → TxPaysHighFeesSKA rejected emission transactions
// with "SKA transaction input 0 does not specify the SKA input value" because
// the lone null input has SKAValueIn=nil. Emission txs are zero-fee by
// protocol (no SKA exists yet to pay a fee with) and must be exempted from
// the high-fee check entirely.
func TestTxPaysHighFeesSKAExemptsEmissionTx(t *testing.T) {
	tx := buildEmissionTx(cointype.CoinType(1))
	// Sanity check our tx builder.
	if !wire.IsSKAEmissionTransaction(tx) {
		t.Fatal("test setup bug: constructed tx is not detected as emission")
	}

	highFees, err := txrules.TxPaysHighFeesSKA(tx, chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("TxPaysHighFeesSKA on emission tx returned error: %v", err)
	}
	if highFees {
		t.Fatal("TxPaysHighFeesSKA must report false for emission txs (zero-fee by protocol)")
	}
}

// TestPaysHighFeesReturnsBugOnSKA locks in the runtime guard on PaysHighFees.
// The VAR-only function previously returned a silent `false` when handed an
// SKA output — a value that, in any caller reaching the function with the
// wrong dispatch, would silently bypass the high-fee gate. The fix replaces
// that silent default with an errors.Bug return so any future caller that
// forgets to dispatch on coin type fails loudly without crashing the
// long-running daemon. All current call sites (createtx.go, methods.go,
// rpcserver/server.go) already branch on coin type before calling, so this
// error path is unreachable in practice.
func TestPaysHighFeesReturnsBugOnSKA(t *testing.T) {
	tx, _ := buildSKATxWithOverpaidFee(t, cointype.CoinType(1))

	highFees, err := txrules.PaysHighFees(0, tx)
	if err == nil {
		t.Fatal("PaysHighFees must return an error for SKA outputs (VAR-only guard)")
	}
	if highFees {
		t.Fatal("PaysHighFees must return false alongside the error")
	}
	if !errors.Is(err, errors.Bug) {
		t.Fatalf("expected errors.Bug kind, got %v", err)
	}
	// Sanity-check the message points users at the right function; avoid
	// asserting the exact phrasing.
	msg := err.Error()
	if !strings.Contains(msg, "SKA") || !strings.Contains(msg, "PaysHighFeesSKA") {
		t.Fatalf("error message must point at PaysHighFeesSKA; got %q", msg)
	}
}
