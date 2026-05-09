// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"context"
	"math/big"
	"testing"

	"github.com/monetarium/monetarium-node/chaincfg/chainhash"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/dcrec/secp256k1"
	"github.com/monetarium/monetarium-node/dcrutil"
	"github.com/monetarium/monetarium-node/txscript/stdaddr"
	"github.com/monetarium/monetarium-node/txscript/stdscript"
	"github.com/monetarium/monetarium-node/wire"
	"github.com/monetarium/monetarium-wallet/errors"
	"github.com/monetarium/monetarium-wallet/wallet/txrules"
	"github.com/monetarium/monetarium-wallet/wallet/txsizes"
)

// TestPrepareRedeemMultiSigOutSKADust verifies that PrepareRedeemMultiSigOutTxOutput
// rejects an SKA redemption whose output value (after fee) would be below the
// 30-atom dust threshold. Without this check the wallet signs and broadcasts a
// tx that the node silently rejects post-broadcast as non-standard, surfacing
// as a confusing error to operators.
func TestPrepareRedeemMultiSigOutSKADust(t *testing.T) {
	ctx := context.Background()
	cfg := basicWalletConfig
	w, teardown := testWallet(ctx, t, &cfg, nil)
	defer teardown()

	// SKA1 is the only active SKA coin on simnet at genesis.
	ct := cointype.CoinType(1)

	// Construct a synthetic 2-of-2 P2SH redeem script so we can size the
	// pkScript realistically. The script's signing keys don't matter — only
	// the redemption pkScript size feeds into EstimateSerializeSizeSKA.
	priv1, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	priv2, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	redeemScript, err := stdscript.MultiSigScriptV0(2,
		priv1.PubKey().SerializeCompressed(),
		priv2.PubKey().SerializeCompressed())
	if err != nil {
		t.Fatal(err)
	}
	p2shAddr, err := stdaddr.NewAddressScriptHashV0(redeemScript, cfg.Params)
	if err != nil {
		t.Fatal(err)
	}
	_, pkScript := p2shAddr.PaymentScript()

	// Compute the exact feeEst that PrepareRedeemMultiSigOutTxOutput will
	// compute for a single-input, single-output SKA redemption tx.
	scriptSizes := []int{txsizes.RedeemP2SHSigScriptSize}
	probeOut := wire.NewTxOut(0, pkScript)
	probeOut.CoinType = ct
	feeSize := txsizes.EstimateSerializeSizeSKA(scriptSizes, []*wire.TxOut{probeOut}, 0)
	relayFee := w.RelayFeeForCoinType(ctx, ct)
	feeEst := txrules.FeeForSerializeSizeSKA(relayFee, feeSize)

	// Common P2SHMultiSigOutput skeleton; SKAOutputAmount is set per-case.
	baseOutput := func(skaAmount cointype.SKAAmount) *P2SHMultiSigOutput {
		return &P2SHMultiSigOutput{
			OutPoint:        wire.OutPoint{Hash: chainhash.HashH([]byte("test")), Index: 0},
			OutputAmount:    0,
			SKAOutputAmount: skaAmount,
			CoinType:        ct,
			P2SHAddress:     p2shAddr,
			RedeemScript:    redeemScript,
			M:               2,
			N:               2,
		}
	}

	mkInputTx := func() *wire.MsgTx {
		tx := wire.NewMsgTx()
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{Hash: chainhash.HashH([]byte("prev")), Index: 0},
		})
		return tx
	}

	t.Run("dust below threshold rejected", func(t *testing.T) {
		// toReceive = feeEst + 29 atoms - feeEst = 29 atoms < MinSKADustAtoms.
		amt := cointype.NewSKAAmount(new(big.Int).Add(feeEst.BigInt(),
			big.NewInt(int64(cointype.MinSKADustAtoms-1))))
		tx := mkInputTx()
		err := w.PrepareRedeemMultiSigOutTxOutput(ctx, tx, baseOutput(amt), &pkScript, ct)
		if err == nil {
			t.Fatal("expected dust-threshold rejection, got nil")
		}
		var kind errors.Kind
		if !errors.As(err, &kind) || kind != errors.Policy {
			t.Fatalf("expected errors.Policy, got %v (kind=%v)", err, kind)
		}
		if len(tx.TxOut) != 0 {
			t.Fatalf("rejected redemption must not append TxOut; got %d", len(tx.TxOut))
		}
	})

	t.Run("at threshold accepted", func(t *testing.T) {
		// toReceive = feeEst + 30 atoms - feeEst = 30 atoms == MinSKADustAtoms.
		amt := cointype.NewSKAAmount(new(big.Int).Add(feeEst.BigInt(),
			big.NewInt(int64(cointype.MinSKADustAtoms))))
		tx := mkInputTx()
		err := w.PrepareRedeemMultiSigOutTxOutput(ctx, tx, baseOutput(amt), &pkScript, ct)
		if err != nil {
			t.Fatalf("expected accept at exact dust threshold, got %v", err)
		}
		if len(tx.TxOut) != 1 {
			t.Fatalf("expected 1 TxOut after accept, got %d", len(tx.TxOut))
		}
		got := tx.TxOut[0].SKAValue
		want := big.NewInt(int64(cointype.MinSKADustAtoms))
		if got.Cmp(want) != 0 {
			t.Fatalf("SKAValue = %v, want %v", got, want)
		}
	})
}

// TestPrepareRedeemMultiSigOutVARDust verifies that the VAR multisig redemption
// path now enforces the same dust-threshold guarantee as the SKA path. Without
// this check a 1-atom VAR redemption would be constructed and broadcast, only
// to be rejected by the mempool's standardness rules — the SKA path failed
// these symmetric inputs up front while VAR did not.
func TestPrepareRedeemMultiSigOutVARDust(t *testing.T) {
	ctx := context.Background()
	cfg := basicWalletConfig
	w, teardown := testWallet(ctx, t, &cfg, nil)
	defer teardown()

	ct := cointype.CoinTypeVAR

	priv1, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	priv2, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	redeemScript, err := stdscript.MultiSigScriptV0(2,
		priv1.PubKey().SerializeCompressed(),
		priv2.PubKey().SerializeCompressed())
	if err != nil {
		t.Fatal(err)
	}
	p2shAddr, err := stdaddr.NewAddressScriptHashV0(redeemScript, cfg.Params)
	if err != nil {
		t.Fatal(err)
	}
	_, pkScript := p2shAddr.PaymentScript()

	scriptSizes := []int{txsizes.RedeemP2SHSigScriptSize}
	probeOut := wire.NewTxOut(0, pkScript)
	probeOut.CoinType = ct
	relayFee := w.RelayFee()
	feeSize := txsizes.EstimateSerializeSize(scriptSizes, []*wire.TxOut{probeOut}, 0)
	feeEst := txrules.FeeForSerializeSize(relayFee, feeSize)

	// Find the smallest non-dust amount for this pkScript+relayFee combination.
	// IsDustAmount is the oracle the production path now consults; deriving the
	// boundary from it keeps the test stable across relay-fee tuning.
	var minNonDust dcrutil.Amount
	for v := dcrutil.Amount(1); v < dcrutil.Amount(1_000_000); v++ {
		if !txrules.IsDustAmount(v, len(pkScript), relayFee) {
			minNonDust = v
			break
		}
	}
	if minNonDust == 0 {
		t.Fatalf("dust threshold not found below 1e6 atoms; relay fee %v misconfigured?", relayFee)
	}

	baseOutput := func(amt dcrutil.Amount) *P2SHMultiSigOutput {
		return &P2SHMultiSigOutput{
			OutPoint:     wire.OutPoint{Hash: chainhash.HashH([]byte("test")), Index: 0},
			OutputAmount: amt,
			CoinType:     ct,
			P2SHAddress:  p2shAddr,
			RedeemScript: redeemScript,
			M:            2,
			N:            2,
		}
	}

	mkInputTx := func() *wire.MsgTx {
		tx := wire.NewMsgTx()
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{Hash: chainhash.HashH([]byte("prev")), Index: 0},
		})
		return tx
	}

	t.Run("dust below threshold rejected", func(t *testing.T) {
		// toReceive = feeEst + (minNonDust - 1) - feeEst = minNonDust - 1 → dust.
		amt := feeEst + minNonDust - 1
		tx := mkInputTx()
		err := w.PrepareRedeemMultiSigOutTxOutput(ctx, tx, baseOutput(amt), &pkScript, ct)
		if err == nil {
			t.Fatal("expected dust-threshold rejection, got nil")
		}
		var kind errors.Kind
		if !errors.As(err, &kind) || kind != errors.Policy {
			t.Fatalf("expected errors.Policy, got %v (kind=%v)", err, kind)
		}
		if len(tx.TxOut) != 0 {
			t.Fatalf("rejected redemption must not append TxOut; got %d", len(tx.TxOut))
		}
	})

	t.Run("at threshold accepted", func(t *testing.T) {
		// toReceive = feeEst + minNonDust - feeEst = minNonDust → not dust.
		amt := feeEst + minNonDust
		tx := mkInputTx()
		err := w.PrepareRedeemMultiSigOutTxOutput(ctx, tx, baseOutput(amt), &pkScript, ct)
		if err != nil {
			t.Fatalf("expected accept at exact dust threshold, got %v", err)
		}
		if len(tx.TxOut) != 1 {
			t.Fatalf("expected 1 TxOut after accept, got %d", len(tx.TxOut))
		}
		if tx.TxOut[0].Value != int64(minNonDust) {
			t.Fatalf("Value = %d, want %d", tx.TxOut[0].Value, int64(minNonDust))
		}
	})
}
