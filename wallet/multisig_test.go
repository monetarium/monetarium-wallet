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
	"github.com/monetarium/monetarium-node/dcrec"
	"github.com/monetarium/monetarium-node/dcrec/secp256k1"
	"github.com/monetarium/monetarium-node/dcrutil"
	"github.com/monetarium/monetarium-node/txscript"
	"github.com/monetarium/monetarium-node/txscript/sign"
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
	// compute for a single-input, single-output SKA redemption tx. The
	// production code sizes each multisig input with
	// RedeemP2SHMultiSigSigScriptSize(M, len(redeemScript)) — mirror that
	// here so this test catches the production fee, not a stale constant.
	scriptSizes := []int{txsizes.RedeemP2SHMultiSigSigScriptSize(2, len(redeemScript))}
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

	// Mirror the production path: size the multisig input with
	// RedeemP2SHMultiSigSigScriptSize so this test reflects what
	// PrepareRedeemMultiSigOutTxOutput now computes.
	scriptSizes := []int{txsizes.RedeemP2SHMultiSigSigScriptSize(2, len(redeemScript))}
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

// TestPrepareRedeemMultiSigOutMatchesSignedSize is the regression test for the
// fee-undercount bug that motivated RedeemP2SHMultiSigSigScriptSize. It
// constructs a real N-of-M P2SH multisig, runs the wallet's redemption-tx
// preparation, signs the input with the genuine multisig flow, and asserts
// that the wallet's worst-case size estimate is >= the actual serialized
// signed-tx size. Prior to the fix the wallet under-estimated this by 70-150
// bytes per multisig input, which the node rejected with `insufficient fee
// for coin type N`. The earlier dust tests fed the same estimator both as the
// production code and as the oracle, so they couldn't catch this drift.
func TestPrepareRedeemMultiSigOutMatchesSignedSize(t *testing.T) {
	ctx := context.Background()
	cfg := basicWalletConfig
	w, teardown := testWallet(ctx, t, &cfg, nil)
	defer teardown()

	skaCT := cointype.CoinType(1)

	cases := []struct {
		name      string
		required  int
		totalKeys int
		coinType  cointype.CoinType
	}{
		{"VAR_1-of-2", 1, 2, cointype.CoinTypeVAR},
		{"VAR_2-of-3", 2, 3, cointype.CoinTypeVAR},
		{"SKA_1-of-2", 1, 2, skaCT},
		{"SKA_2-of-3", 2, 3, skaCT},
		{"SKA_3-of-5", 3, 5, skaCT},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Build N keys and the multisig redeem script.
			privs := make([]*secp256k1.PrivateKey, tc.totalKeys)
			pubkeys := make([][]byte, tc.totalKeys)
			for i := range privs {
				p, err := secp256k1.GeneratePrivateKey()
				if err != nil {
					t.Fatal(err)
				}
				privs[i] = p
				pubkeys[i] = p.PubKey().SerializeCompressed()
			}
			redeemScript, err := stdscript.MultiSigScriptV0(tc.required, pubkeys...)
			if err != nil {
				t.Fatal(err)
			}
			p2shAddr, err := stdaddr.NewAddressScriptHashV0(redeemScript, cfg.Params)
			if err != nil {
				t.Fatal(err)
			}
			_, p2shPkScript := p2shAddr.PaymentScript()

			// Destination output (a fresh P2PKH).
			destPriv, err := secp256k1.GeneratePrivateKey()
			if err != nil {
				t.Fatal(err)
			}
			destPkh := dcrutil.Hash160(destPriv.PubKey().SerializeCompressed())
			destAddr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(destPkh, cfg.Params)
			if err != nil {
				t.Fatal(err)
			}
			_, destPkScript := destAddr.PaymentScript()

			// Construct the unsigned redemption tx exactly the way
			// redeemMultiSigOut does: a single TxIn referencing the P2SH
			// multisig output, with the value carried in the right field for
			// the coin type. Then run PrepareRedeemMultiSigOutTxOutput to
			// append the destination output (after subtracting the wallet's
			// estimated fee).
			tx := wire.NewMsgTx()
			outPoint := wire.OutPoint{Hash: chainhash.HashH([]byte("prev:" + tc.name)), Index: 0}
			var p2shOutput *P2SHMultiSigOutput
			if tc.coinType.IsSKA() {
				// Fund the input with enough SKA to comfortably cover the
				// worst-case fee for these script sizes (≈2.1 SKA for 3-of-5
				// at the 4 SKA/kB simnet rate; 100 SKA is plenty of margin).
				inputAtoms := new(big.Int).SetUint64(100)
				inputAtoms.Mul(inputAtoms, new(big.Int).SetUint64(1_000_000_000_000_000_000))
				inputSKA := cointype.NewSKAAmount(inputAtoms)
				txIn := wire.NewTxIn(&outPoint, 0, nil)
				txIn.SKAValueIn = new(big.Int).Set(inputSKA.BigInt())
				tx.AddTxIn(txIn)
				p2shOutput = &P2SHMultiSigOutput{
					OutPoint:        outPoint,
					SKAOutputAmount: inputSKA,
					CoinType:        tc.coinType,
					P2SHAddress:     p2shAddr,
					RedeemScript:    redeemScript,
					M:               uint8(tc.required),
					N:               uint8(tc.totalKeys),
				}
			} else {
				inputVAR := dcrutil.Amount(100_000_000) // 1 VAR — plenty.
				tx.AddTxIn(wire.NewTxIn(&outPoint, int64(inputVAR), nil))
				p2shOutput = &P2SHMultiSigOutput{
					OutPoint:     outPoint,
					OutputAmount: inputVAR,
					CoinType:     tc.coinType,
					P2SHAddress:  p2shAddr,
					RedeemScript: redeemScript,
					M:            uint8(tc.required),
					N:            uint8(tc.totalKeys),
				}
			}
			if err := w.PrepareRedeemMultiSigOutTxOutput(ctx, tx, p2shOutput, &destPkScript, tc.coinType); err != nil {
				t.Fatalf("PrepareRedeemMultiSigOutTxOutput: %v", err)
			}

			// Estimate as the production code does (mirrors multisig.go).
			sigScriptSize := txsizes.RedeemP2SHMultiSigSigScriptSize(tc.required, len(redeemScript))
			var estimatedSize int
			if tc.coinType.IsSKA() {
				estimatedSize = txsizes.EstimateSerializeSizeSKA(
					[]int{sigScriptSize}, tx.TxOut, 0)
			} else {
				estimatedSize = txsizes.EstimateSerializeSize(
					[]int{sigScriptSize}, tx.TxOut, 0)
			}

			// Sign the multisig input. SignTxOutput will recurse into the
			// P2SH redeem script and produce a real sigScript with M
			// signatures + redeem-script push.
			keyDB := sign.KeyClosure(func(addr stdaddr.Address) ([]byte, dcrec.SignatureType, bool, error) {
				for _, p := range privs {
					a, err := stdaddr.NewAddressPubKeyEcdsaSecp256k1V0Raw(
						p.PubKey().SerializeCompressed(), cfg.Params)
					if err != nil {
						return nil, 0, false, err
					}
					if a.String() == addr.String() {
						return p.Serialize(), dcrec.STEcdsaSecp256k1, true, nil
					}
				}
				return nil, 0, false, errors.E("no key for " + addr.String())
			})
			scriptDB := sign.ScriptClosure(func(addr stdaddr.Address) ([]byte, error) {
				if addr.String() == p2shAddr.String() {
					return redeemScript, nil
				}
				return nil, errors.E("no script for " + addr.String())
			})
			sigScript, err := sign.SignTxOutput(cfg.Params, tx, 0, p2shPkScript,
				txscript.SigHashAll, keyDB, scriptDB, nil, false)
			if err != nil {
				t.Fatalf("SignTxOutput: %v", err)
			}
			tx.TxIn[0].SignatureScript = sigScript

			actualSize := tx.SerializeSize()
			if actualSize > estimatedSize {
				t.Fatalf("signed tx size %d exceeds wallet estimate %d (delta %d)"+
					" — fee will be insufficient at the consensus check",
					actualSize, estimatedSize, actualSize-estimatedSize)
			}
			// Loose lower bound. Two sources of slack:
			//   • Each DER ECDSA sig can be 1-3 bytes shorter than the
			//     worst-case 73-byte budget (M signatures → 3*M bytes).
			//   • For SKA inputs, EstimateInputWitnessSizeSKA reserves a
			//     worst-case 16-byte SKAValueIn, but a typical funding
			//     amount big.Int-encodes to 10-12 bytes (≤ 6 bytes slack).
			// Anything materially beyond that suggests the estimator is
			// over-allocating wire space.
			//
			// The SKA bonus is 16 (not 8) to give clear headroom over the
			// 3*M + 6 worst-case combined savings: for the 3-of-5 row this
			// avoids the exact-tie at 21 bytes that would flake when every
			// signed signature lands at the short end of DER variance.
			slack := tc.required*3 + 4
			if tc.coinType.IsSKA() {
				slack += 16
			}
			if actualSize < estimatedSize-slack {
				t.Errorf("signed tx size %d much smaller than estimate %d (delta %d > slack %d)"+
					" — estimator may be over-budgeting fees",
					actualSize, estimatedSize, estimatedSize-actualSize, slack)
			}
		})
	}
}
