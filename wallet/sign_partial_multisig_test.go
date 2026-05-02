// Copyright (c) 2024 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"context"
	"testing"

	"github.com/monetarium/monetarium-node/chaincfg/chainhash"
	"github.com/monetarium/monetarium-node/dcrec"
	"github.com/monetarium/monetarium-node/dcrec/secp256k1"
	"github.com/monetarium/monetarium-node/dcrutil"
	"github.com/monetarium/monetarium-node/txscript"
	"github.com/monetarium/monetarium-node/txscript/stdaddr"
	"github.com/monetarium/monetarium-node/txscript/stdscript"
	"github.com/monetarium/monetarium-node/wire"
)

// TestSignTransactionPartialMultisigComplete verifies that SignTransaction's
// returned `complete` boolean reflects whether the script actually validates,
// not merely whether the per-input signing call returned a nil error. Before
// the fix, partially-signed multisig inputs (where SignTxOutput happily
// produces a signature script that doesn't yet satisfy the threshold) caused
// SignTransaction to report complete=true, which fooled higher-level callers
// into broadcasting unspendable transactions.
//
// Test strategy: hand-build a 2-of-2 multisig P2SH and a tx spending it. Sign
// twice — first with one key (expect complete=false), then with both
// (expect complete=true) — using SignTransaction's additionalPrevScripts /
// additionalKeysByAddress / p2shRedeemScriptsByAddress hooks so we don't need
// to fund a real prevout in the wallet.
func TestSignTransactionPartialMultisigComplete(t *testing.T) {
	ctx := context.Background()
	cfg := basicWalletConfig
	w, teardown := testWallet(ctx, t, &cfg, nil)
	defer teardown()

	params := cfg.Params

	priv1, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	priv2, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}

	pub1 := priv1.PubKey().SerializeCompressed()
	pub2 := priv2.PubKey().SerializeCompressed()

	redeemScript, err := stdscript.MultiSigScriptV0(2, pub1, pub2)
	if err != nil {
		t.Fatal(err)
	}

	p2shAddr, err := stdaddr.NewAddressScriptHashV0(redeemScript, params)
	if err != nil {
		t.Fatal(err)
	}
	_, p2shPkScript := p2shAddr.PaymentScript()

	// Destination for the spend.
	destPriv, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	destAddr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(
		stdaddr.Hash160(destPriv.PubKey().SerializeCompressed()), params)
	if err != nil {
		t.Fatal(err)
	}
	_, destPkScript := destAddr.PaymentScript()

	// Build a tx spending a synthetic prevout.
	prevOut := wire.OutPoint{Hash: chainhash.HashH([]byte("multisig-prevout")), Index: 0}
	newTx := func() *wire.MsgTx {
		tx := wire.NewMsgTx()
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: prevOut,
			ValueIn:          1_000_000,
		})
		tx.AddTxOut(wire.NewTxOut(900_000, destPkScript))
		return tx
	}

	addrFor := func(priv *secp256k1.PrivateKey) stdaddr.Address {
		a, err := stdaddr.NewAddressPubKeyEcdsaSecp256k1V0(priv.PubKey(), params)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	wifFor := func(priv *secp256k1.PrivateKey) *dcrutil.WIF {
		wif, err := dcrutil.NewWIF(priv.Serialize(), params.PrivateKeyID, dcrec.STEcdsaSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		return wif
	}

	addr1 := addrFor(priv1).String()
	addr2 := addrFor(priv2).String()

	prevScripts := map[wire.OutPoint][]byte{prevOut: p2shPkScript}
	scripts := map[string][]byte{p2shAddr.String(): redeemScript}

	t.Run("one of two keys yields complete=false", func(t *testing.T) {
		tx := newTx()
		keys := map[string]*dcrutil.WIF{addr1: wifFor(priv1)}
		signErrors, complete, err := w.SignTransaction(ctx, tx,
			txscript.SigHashAll, prevScripts, keys, scripts)
		if err != nil {
			t.Fatalf("SignTransaction returned error: %v", err)
		}
		if complete {
			t.Fatalf("expected complete=false for 1-of-2 sigs; signErrors=%v", signErrors)
		}
	})

	t.Run("both keys yield complete=true", func(t *testing.T) {
		tx := newTx()
		keys := map[string]*dcrutil.WIF{
			addr1: wifFor(priv1),
			addr2: wifFor(priv2),
		}
		signErrors, complete, err := w.SignTransaction(ctx, tx,
			txscript.SigHashAll, prevScripts, keys, scripts)
		if err != nil {
			t.Fatalf("SignTransaction returned error: %v", err)
		}
		if !complete {
			t.Fatalf("expected complete=true for 2-of-2 sigs; signErrors=%v", signErrors)
		}
	})
}

// TestSignTransactionPartialMultisigCompleteWalletInternal exercises the same
// completeness contract as TestSignTransactionPartialMultisigComplete, but
// drives SignTransaction down its wallet-internal prev-script lookup path
// (txStore.TxDetails) rather than the caller-supplied additionalPrevScripts
// map.  The two paths populate prevOutScript from different sources, and the
// fix at wallet.go:5661 (using the resolved prevOutScript instead of
// additionalPrevScripts[outpoint] when classifying the multisig underflow)
// only matters here.  A regression of that line would re-introduce
// complete=true for partially-signed P2SH multisig inputs whose pkScript
// comes from the wallet's own UTXO set.
func TestSignTransactionPartialMultisigCompleteWalletInternal(t *testing.T) {
	ctx := context.Background()
	cfg := basicWalletConfig
	w, teardown := testWallet(ctx, t, &cfg, nil)
	defer teardown()

	params := cfg.Params

	priv1, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	priv2, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}

	pub1 := priv1.PubKey().SerializeCompressed()
	pub2 := priv2.PubKey().SerializeCompressed()

	redeemScript, err := stdscript.MultiSigScriptV0(2, pub1, pub2)
	if err != nil {
		t.Fatal(err)
	}

	p2shAddr, err := stdaddr.NewAddressScriptHashV0(redeemScript, params)
	if err != nil {
		t.Fatal(err)
	}
	_, p2shPkScript := p2shAddr.PaymentScript()

	destPriv, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	destAddr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(
		stdaddr.Hash160(destPriv.PubKey().SerializeCompressed()), params)
	if err != nil {
		t.Fatal(err)
	}
	_, destPkScript := destAddr.PaymentScript()

	// Build and insert a "previous" transaction whose only output is the
	// 2-of-2 P2SH.  Inserting via AddTransaction makes it discoverable to
	// SignTransaction via txStore.TxDetails when the caller does not
	// supply additionalPrevScripts.
	prevTx := wire.NewMsgTx()
	prevTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.HashH([]byte("multisig-internal-funding")),
			Index: 0,
		},
		ValueIn: 1_000_000,
	})
	prevTx.AddTxOut(wire.NewTxOut(900_000, p2shPkScript))
	if err := w.AddTransaction(ctx, prevTx, nil); err != nil {
		t.Fatalf("AddTransaction(prevTx): %v", err)
	}
	prevHash := prevTx.TxHash()
	prevOut := wire.OutPoint{Hash: prevHash, Index: 0}

	newSpend := func() *wire.MsgTx {
		tx := wire.NewMsgTx()
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: prevOut,
			ValueIn:          900_000,
		})
		tx.AddTxOut(wire.NewTxOut(800_000, destPkScript))
		return tx
	}

	addrFor := func(priv *secp256k1.PrivateKey) stdaddr.Address {
		a, err := stdaddr.NewAddressPubKeyEcdsaSecp256k1V0(priv.PubKey(), params)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	wifFor := func(priv *secp256k1.PrivateKey) *dcrutil.WIF {
		wif, err := dcrutil.NewWIF(priv.Serialize(), params.PrivateKeyID, dcrec.STEcdsaSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		return wif
	}

	addr1 := addrFor(priv1).String()
	addr2 := addrFor(priv2).String()

	// Note: no entry for prevOut in additionalPrevScripts — this is the
	// whole point of the test.  SignTransaction must fall through to
	// txStore.TxDetails(prevHash) and use prevOutScript from
	// txDetails.MsgTx.TxOut[0].PkScript when classifying the multisig
	// underflow on the partial-sig path.
	scripts := map[string][]byte{p2shAddr.String(): redeemScript}

	t.Run("one of two keys yields complete=false with no signErrors", func(t *testing.T) {
		tx := newSpend()
		keys := map[string]*dcrutil.WIF{addr1: wifFor(priv1)}
		signErrors, complete, err := w.SignTransaction(ctx, tx,
			txscript.SigHashAll, nil, keys, scripts)
		if err != nil {
			t.Fatalf("SignTransaction returned error: %v", err)
		}
		if complete {
			t.Fatalf("expected complete=false for 1-of-2 sigs via wallet-internal lookup; signErrors=%v", signErrors)
		}
		// signrawtransaction surfaces Complete as
		// `complete && len(signErrors) == 0 && err == nil`, so a
		// regression that mis-classifies a partial multisig as a hard
		// signing error (and populates signErrors) leaks a misleading
		// SignatureError up to RPC callers even though Complete is
		// already false from the outer branch.  Assert empty signErrors
		// to catch reverts of the prevOutScript fix on this path.
		if len(signErrors) != 0 {
			t.Fatalf("expected no signErrors for partial multisig on wallet-internal lookup; got %v", signErrors)
		}
	})

	t.Run("both keys yield complete=true", func(t *testing.T) {
		tx := newSpend()
		keys := map[string]*dcrutil.WIF{
			addr1: wifFor(priv1),
			addr2: wifFor(priv2),
		}
		signErrors, complete, err := w.SignTransaction(ctx, tx,
			txscript.SigHashAll, nil, keys, scripts)
		if err != nil {
			t.Fatalf("SignTransaction returned error: %v", err)
		}
		if !complete {
			t.Fatalf("expected complete=true for 2-of-2 sigs via wallet-internal lookup; signErrors=%v", signErrors)
		}
	})
}

