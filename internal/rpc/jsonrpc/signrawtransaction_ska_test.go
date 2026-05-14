// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package jsonrpc

import (
	"bytes"
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/decred/slog"
	"github.com/monetarium/monetarium-node/chaincfg"
	"github.com/monetarium/monetarium-node/chaincfg/chainhash"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/wire"
	"github.com/monetarium/monetarium-wallet/wallet/udb"
)

// TestPopulateSKAValueIn pins the HIGH-2 fix from the 2026-05-04 review:
// when the wallet is asked to sign an SKA tx whose inputs arrived without
// SKAValueIn (e.g. a third-party tool built the wire.MsgTx from primitives
// without the V13 wire-format extension fields), the handler must populate
// SKAValueIn from the caller-supplied skaValueIn RawTxInput field, then from
// the wallet's UTXO set, and refuse with an actionable error if neither
// source is available — rather than silently producing a tx the node will
// reject with a fraud-proof error.
func TestPopulateSKAValueIn(t *testing.T) {
	// Synthetic chain params with one SKA coin (1e18 atoms per coin).
	// MaxSupply = 100 SKA so the magnitude-cap arms below have a concrete
	// boundary to test against.
	const skaCT = cointype.CoinType(1)
	atomsPerCoin := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	maxSupply := new(big.Int).Mul(big.NewInt(100), atomsPerCoin) // 100 SKA
	params := &chaincfg.Params{
		SKACoins: map[cointype.CoinType]*chaincfg.SKACoinConfig{
			skaCT: {
				AtomsPerCoin: new(big.Int).Set(atomsPerCoin),
				MaxSupply:    new(big.Int).Set(maxSupply),
				Active:       true,
			},
		},
	}

	// mkSKATx builds a minimal SKA tx with one output (so the population
	// pass detects it as SKA via GetCoinTypeFromOutputs) and N inputs whose
	// SKAValueIn is initially nil.
	mkSKATx := func(numInputs int) *wire.MsgTx {
		tx := wire.NewMsgTx()
		// Distinct prevout hashes per input so the lookup map keys do not
		// collide.
		for i := 0; i < numInputs; i++ {
			var h chainhash.Hash
			h[0] = byte(i + 1)
			txIn := wire.NewTxIn(&wire.OutPoint{Hash: h, Index: 0, Tree: 0}, 0, nil)
			txIn.SKAValueIn = nil
			tx.AddTxIn(txIn)
		}
		out := wire.NewTxOut(0, []byte{0x76})
		out.CoinType = skaCT
		out.SKAValue = new(big.Int).Set(atomsPerCoin)
		tx.AddTxOut(out)
		return tx
	}

	// Convenience: run the helper and return tx + error.
	run := func(tx *wire.MsgTx, callerSKA map[wire.OutPoint]string,
		lookup skaCreditLookup) error {
		return populateSKAValueIn(context.Background(), tx, params,
			callerSKA, lookup)
	}

	t.Run("VAR tx is a no-op", func(t *testing.T) {
		tx := wire.NewMsgTx()
		var h chainhash.Hash
		h[0] = 1
		tx.AddTxIn(wire.NewTxIn(&wire.OutPoint{Hash: h}, 0, nil))
		tx.AddTxOut(wire.NewTxOut(100, []byte{0x76}))
		// callerSKA empty, lookup nil — must not error and must not populate.
		if err := run(tx, nil, nil); err != nil {
			t.Fatalf("VAR tx must be a no-op; got %v", err)
		}
		if tx.TxIn[0].SKAValueIn != nil {
			t.Fatalf("VAR tx must not have SKAValueIn populated")
		}
	})

	t.Run("already-set SKAValueIn is preserved", func(t *testing.T) {
		tx := mkSKATx(1)
		preset := big.NewInt(42)
		tx.TxIn[0].SKAValueIn = preset
		if err := run(tx, nil, nil); err != nil {
			t.Fatalf("preset SKAValueIn must satisfy the population pass; got %v", err)
		}
		if tx.TxIn[0].SKAValueIn != preset {
			t.Fatalf("preset SKAValueIn must be preserved (no allocation)")
		}
	})

	t.Run("caller value overrides on-wire SKAValueIn", func(t *testing.T) {
		// Pin the M4 fix from the 2026-05-05 review: when both an on-wire
		// SKAValueIn AND a caller-supplied skaValueIn (RawTxInput field) are
		// present, the caller value wins and the swap is observable. Before
		// the fix the caller value was silently dropped and the on-wire
		// (potentially stale) value passed through unchanged.
		//
		// Also pin the Warn log emission: a future "tighten to chain truth"
		// refactor that drops the override silently would still satisfy the
		// value-swap assertions below if the log line were the only behaviour
		// removed. Capture the package-level logger for the duration of this
		// subtest and assert the override is observable in operator logs.
		var logBuf bytes.Buffer
		backend := slog.NewBackend(&logBuf)
		captured := backend.Logger("TEST")
		captured.SetLevel(slog.LevelTrace)
		oldLog := log
		UseLogger(captured)
		t.Cleanup(func() { UseLogger(oldLog) })

		tx := mkSKATx(1)
		op := tx.TxIn[0].PreviousOutPoint
		// On-wire stale value: 5 SKA; caller's authoritative override: 9 SKA.
		stale := new(big.Int).Mul(big.NewInt(5), atomsPerCoin)
		tx.TxIn[0].SKAValueIn = stale
		tx.TxIn[0].ValueIn = 12345 // sentinel; must be zeroed on override
		callerSKA := map[wire.OutPoint]string{op: "9"}
		if err := run(tx, callerSKA, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := new(big.Int).Mul(big.NewInt(9), atomsPerCoin)
		if got := tx.TxIn[0].SKAValueIn; got == nil || got.Cmp(want) != 0 {
			t.Fatalf("caller value must override on-wire; got %v want %v", got, want)
		}
		if tx.TxIn[0].ValueIn != 0 {
			t.Fatalf("override must zero ValueIn (SKA invariant); got %d", tx.TxIn[0].ValueIn)
		}
		if logged := logBuf.String(); !strings.Contains(logged,
			"populateSKAValueIn: caller-supplied skaValueIn=") {
			t.Fatalf("override must emit a Warn line for operator audit; logger output: %q", logged)
		}
	})

	t.Run("matching caller value preserves on-wire (no spurious override)", func(t *testing.T) {
		// When the caller supplies the same value already on the wire, the
		// override path must not re-allocate or change anything observable.
		// This pins that the override is gated on Cmp != 0, not just presence.
		tx := mkSKATx(1)
		op := tx.TxIn[0].PreviousOutPoint
		preset := new(big.Int).Mul(big.NewInt(3), atomsPerCoin)
		tx.TxIn[0].SKAValueIn = preset
		callerSKA := map[wire.OutPoint]string{op: "3"}
		if err := run(tx, callerSKA, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tx.TxIn[0].SKAValueIn != preset {
			t.Fatalf("matching caller value must not replace the on-wire pointer")
		}
	})

	t.Run("caller-supplied skaValueIn is parsed and assigned", func(t *testing.T) {
		tx := mkSKATx(1)
		op := tx.TxIn[0].PreviousOutPoint
		callerSKA := map[wire.OutPoint]string{op: "1.5"}
		if err := run(tx, callerSKA, nil); err != nil {
			t.Fatalf("caller-supplied skaValueIn must be accepted; got %v", err)
		}
		// 1.5 SKA at 1e18 atoms/coin = 1.5e18 atoms.
		want := new(big.Int).Mul(big.NewInt(15), new(big.Int).Exp(big.NewInt(10), big.NewInt(17), nil))
		if got := tx.TxIn[0].SKAValueIn; got == nil || got.Cmp(want) != 0 {
			t.Fatalf("SKAValueIn = %v, want %v", got, want)
		}
		if tx.TxIn[0].ValueIn != 0 {
			t.Fatalf("SKA input ValueIn must be zeroed; got %d", tx.TxIn[0].ValueIn)
		}
	})

	t.Run("malformed caller skaValueIn is rejected", func(t *testing.T) {
		tx := mkSKATx(1)
		op := tx.TxIn[0].PreviousOutPoint
		callerSKA := map[wire.OutPoint]string{op: "not-a-number"}
		err := run(tx, callerSKA, nil)
		if err == nil {
			t.Fatalf("malformed skaValueIn must be rejected")
		}
		if !strings.Contains(err.Error(), "skaValueIn") {
			t.Errorf("error message must mention skaValueIn; got %q", err.Error())
		}
	})

	t.Run("zero caller skaValueIn is rejected", func(t *testing.T) {
		tx := mkSKATx(1)
		op := tx.TxIn[0].PreviousOutPoint
		callerSKA := map[wire.OutPoint]string{op: "0"}
		err := run(tx, callerSKA, nil)
		if err == nil {
			t.Fatalf("zero skaValueIn must be rejected (nodes require positive)")
		}
		if !strings.Contains(err.Error(), "positive") {
			t.Errorf("error message must mention positivity; got %q", err.Error())
		}
	})

	t.Run("wallet UTXO lookup populates SKAValueIn when no caller value", func(t *testing.T) {
		tx := mkSKATx(1)
		op := tx.TxIn[0].PreviousOutPoint
		want := new(big.Int).Mul(big.NewInt(7), atomsPerCoin) // 7 SKA
		lookup := func(_ context.Context, want_op wire.OutPoint) (*udb.Credit, error) {
			if want_op != op {
				t.Fatalf("lookup called with unexpected outpoint %v", want_op)
			}
			return &udb.Credit{
				CoinType:  skaCT,
				SKAAmount: cointype.NewSKAAmount(want),
			}, nil
		}
		if err := run(tx, nil, lookup); err != nil {
			t.Fatalf("wallet UTXO lookup must succeed; got %v", err)
		}
		if got := tx.TxIn[0].SKAValueIn; got == nil || got.Cmp(want) != 0 {
			t.Fatalf("SKAValueIn = %v, want %v", got, want)
		}
	})

	t.Run("wallet UTXO with mismatched coin type is rejected", func(t *testing.T) {
		tx := mkSKATx(1)
		lookup := func(_ context.Context, _ wire.OutPoint) (*udb.Credit, error) {
			return &udb.Credit{
				CoinType:  cointype.CoinType(2), // SKA2, not SKA1
				SKAAmount: cointype.NewSKAAmount(big.NewInt(1)),
			}, nil
		}
		err := run(tx, nil, lookup)
		if err == nil {
			t.Fatalf("mismatched UTXO coin type must be rejected")
		}
		if !strings.Contains(err.Error(), "coin type") {
			t.Errorf("error must mention coin type mismatch; got %q", err.Error())
		}
	})

	t.Run("wallet UTXO above MaxSupply is rejected", func(t *testing.T) {
		// Defense-in-depth: a corrupted credit record carrying more atoms
		// than the chain's MaxSupply must be rejected before signing. The
		// other two source branches (on-wire, caller-supplied) already
		// enforce the same bound; this arm pins the lookup branch.
		tx := mkSKATx(1)
		// 101 SKA — one whole coin over the 100-SKA MaxSupply.
		oversize := new(big.Int).Mul(big.NewInt(101), atomsPerCoin)
		lookup := func(_ context.Context, _ wire.OutPoint) (*udb.Credit, error) {
			return &udb.Credit{
				CoinType:  skaCT,
				SKAAmount: cointype.NewSKAAmount(oversize),
			}, nil
		}
		err := run(tx, nil, lookup)
		if err == nil {
			t.Fatalf("over-MaxSupply UTXO credit must be rejected")
		}
		if !strings.Contains(err.Error(), "MaxSupply") {
			t.Errorf("error must mention MaxSupply; got %q", err.Error())
		}
		if tx.TxIn[0].SKAValueIn != nil {
			t.Fatalf("rejected input must not be left with a populated SKAValueIn")
		}
	})

	t.Run("no caller value and no wallet UTXO yields actionable error", func(t *testing.T) {
		tx := mkSKATx(1)
		// lookup returns NotExist-style empty credit; populateSKAValueIn must
		// route this branch to the refusal arm.
		lookup := func(_ context.Context, _ wire.OutPoint) (*udb.Credit, error) {
			return nil, errExampleNotInWallet
		}
		err := run(tx, nil, lookup)
		if err == nil {
			t.Fatalf("missing SKAValueIn with no fallback must refuse")
		}
		msg := err.Error()
		if !strings.Contains(msg, "skaValueIn") || !strings.Contains(msg, "RawTxInput") {
			t.Errorf("refusal must point operators at the skaValueIn RawTxInput "+
				"field and explain the alternative; got %q", msg)
		}
	})

	t.Run("caller value beats wallet lookup", func(t *testing.T) {
		tx := mkSKATx(1)
		op := tx.TxIn[0].PreviousOutPoint
		callerSKA := map[wire.OutPoint]string{op: "9"}
		// Lookup returns 7; caller-provided 9 must win to preserve the operator's
		// out-of-band assertion (lookup may be stale).
		lookup := func(_ context.Context, _ wire.OutPoint) (*udb.Credit, error) {
			return &udb.Credit{
				CoinType:  skaCT,
				SKAAmount: cointype.NewSKAAmount(new(big.Int).Mul(big.NewInt(7), atomsPerCoin)),
			}, nil
		}
		if err := run(tx, callerSKA, lookup); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := new(big.Int).Mul(big.NewInt(9), atomsPerCoin)
		if got := tx.TxIn[0].SKAValueIn; got == nil || got.Cmp(want) != 0 {
			t.Fatalf("caller value must take priority; got %v want %v", got, want)
		}
	})

	t.Run("caller value above MaxSupply is rejected (no-on-wire branch)", func(t *testing.T) {
		// Pin the H1 fix from the 2026-05-05 review: an authenticated client
		// must not be able to coerce the wallet to sign with an SKAValueIn
		// that exceeds the configured per-coin MaxSupply. The bound is
		// applied BEFORE any signing work runs.
		tx := mkSKATx(1)
		op := tx.TxIn[0].PreviousOutPoint
		// 101 SKA — one whole coin over the 100-SKA MaxSupply.
		callerSKA := map[wire.OutPoint]string{op: "101"}
		err := run(tx, callerSKA, nil)
		if err == nil {
			t.Fatalf("over-MaxSupply skaValueIn must be rejected")
		}
		if !strings.Contains(err.Error(), "MaxSupply") {
			t.Errorf("error must mention MaxSupply; got %q", err.Error())
		}
		if tx.TxIn[0].SKAValueIn != nil {
			t.Fatalf("rejected input must not be left with a populated SKAValueIn")
		}
	})

	t.Run("caller value at MaxSupply is accepted (boundary)", func(t *testing.T) {
		tx := mkSKATx(1)
		op := tx.TxIn[0].PreviousOutPoint
		callerSKA := map[wire.OutPoint]string{op: "100"} // exactly MaxSupply
		if err := run(tx, callerSKA, nil); err != nil {
			t.Fatalf("at-MaxSupply skaValueIn must be accepted; got %v", err)
		}
		if got := tx.TxIn[0].SKAValueIn; got == nil || got.Cmp(maxSupply) != 0 {
			t.Fatalf("SKAValueIn = %v, want %v", got, maxSupply)
		}
	})

	t.Run("caller value above MaxSupply is rejected (override branch)", func(t *testing.T) {
		// Same cap on the override path: a caller-supplied skaValueIn that
		// disagrees with an on-wire value AND exceeds MaxSupply must error
		// before the override is applied.
		tx := mkSKATx(1)
		op := tx.TxIn[0].PreviousOutPoint
		// On-wire 5 SKA (within bound); override attempt: 200 SKA.
		stale := new(big.Int).Mul(big.NewInt(5), atomsPerCoin)
		tx.TxIn[0].SKAValueIn = stale
		callerSKA := map[wire.OutPoint]string{op: "200"}
		err := run(tx, callerSKA, nil)
		if err == nil {
			t.Fatalf("over-MaxSupply override must be rejected")
		}
		if !strings.Contains(err.Error(), "MaxSupply") {
			t.Errorf("error must mention MaxSupply; got %q", err.Error())
		}
		// On-wire value must not be replaced by the rejected override.
		if got := tx.TxIn[0].SKAValueIn; got == nil || got.Cmp(stale) != 0 {
			t.Fatalf("rejected override must leave on-wire value intact; got %v", got)
		}
	})

	t.Run("mixed: input 0 has SKAValueIn preset, input 1 needs caller value", func(t *testing.T) {
		tx := mkSKATx(2)
		preset := big.NewInt(123)
		tx.TxIn[0].SKAValueIn = preset
		op1 := tx.TxIn[1].PreviousOutPoint
		callerSKA := map[wire.OutPoint]string{op1: "2"}
		if err := run(tx, callerSKA, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tx.TxIn[0].SKAValueIn != preset {
			t.Fatalf("input 0 preset value clobbered")
		}
		want := new(big.Int).Mul(big.NewInt(2), atomsPerCoin)
		if got := tx.TxIn[1].SKAValueIn; got == nil || got.Cmp(want) != 0 {
			t.Fatalf("input 1 SKAValueIn = %v, want %v", got, want)
		}
	})
}

// errExampleNotInWallet is a sentinel used by the "no caller value, no
// wallet UTXO" test arm above. populateSKAValueIn treats any non-nil error
// from the lookup callback as "wallet does not own this prevout" and routes
// to the refusal arm.
var errExampleNotInWallet = &exampleErr{"not in wallet UTXO set"}

type exampleErr struct{ msg string }

func (e *exampleErr) Error() string { return e.msg }
