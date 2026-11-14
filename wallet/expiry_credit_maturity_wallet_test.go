// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/monetarium/monetarium-node/chaincfg/chainhash"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/dcrutil"
	gcs2 "github.com/monetarium/monetarium-node/gcs"
	"github.com/monetarium/monetarium-node/gcs/blockcf2"
	"github.com/monetarium/monetarium-node/wire"
	werrors "github.com/monetarium/monetarium-wallet/errors"
	"github.com/monetarium/monetarium-wallet/wallet/udb"
	"github.com/monetarium/monetarium-wallet/wallet/walletdb"
)

// TestExpiryCreditReserveAndListUnspent checks the wallet-level maturity rule
// for credits of transactions with an expiry behind ReserveOutputsForAmount
// (which funds ticket purchases and VSP fee payments) and ListUnspent: both
// exclude the credit until CoinbaseMaturity blocks have passed over it, and
// include it afterwards, for VAR and SKA alike.  An SKA transaction with an
// expiry is a real case: raw transactions carry an expiry regardless of coin.
func TestExpiryCreditReserveAndListUnspent(t *testing.T) {
	for _, ct := range []cointype.CoinType{cointype.CoinTypeVAR, cointype.CoinType(1)} {
		t.Run(ct.String(), func(t *testing.T) {
			testExpiryCreditReserveAndListUnspent(t, ct)
		})
	}
}

func testExpiryCreditReserveAndListUnspent(t *testing.T, ct cointype.CoinType) {
	ctx := context.Background()
	cfg := basicWalletConfig
	w, teardown := testWallet(ctx, t, &cfg, nil)
	defer teardown()

	params := w.chainParams
	addr, err := w.NewExternalAddress(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, pkScript := addr.PaymentScript()

	tx := wire.NewMsgTx()
	tx.Expiry = 100
	tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&chainhash.Hash{0x01}, 0,
		wire.TxTreeRegular), 0, nil))
	if ct.IsSKA() {
		tx.AddTxOut(wire.NewTxOutSKA(big.NewInt(1e8), ct, pkScript))
	} else {
		tx.AddTxOut(wire.NewTxOut(1e8, pkScript))
	}
	rec, err := udb.NewTxRecordFromMsgTx(tx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	txHash := rec.Hash

	// VAR reserves target the int64 amount, SKA reserves the big.Int amount.
	var amount dcrutil.Amount
	var amountSKA = cointype.Zero()
	if ct.IsSKA() {
		amountSKA = cointype.SKAAmountFromInt64(1e8)
	} else {
		amount = 1e8
	}

	emptyFilter, _ := gcs2.FromBytesV2(blockcf2.B, blockcf2.M, nil)

	// Mine the credit: one block past genesis, the credit at height 1.
	// Blocks approve their parent (BlockValid), or stake invalidation
	// moves the credit out of the spendable set.
	b1 := &wire.BlockHeader{PrevBlock: params.GenesisHash, VoteBits: dcrutil.BlockValid, Height: 1}
	b1Hash := b1.BlockHash()
	err = walletdb.Update(ctx, w.db, func(dbtx walletdb.ReadWriteTx) error {
		if err := w.txStore.InsertMemPoolTx(dbtx, rec); err != nil {
			return err
		}
		if err := w.txStore.AddCredit(dbtx, rec, nil, 0, false, 0); err != nil {
			return err
		}
		ns := dbtx.ReadWriteBucket(wtxmgrNamespaceKey)
		if err := w.txStore.ExtendMainChain(ns, b1, &b1Hash, emptyFilter); err != nil {
			return err
		}
		return w.txStore.InsertMinedTx(dbtx, rec, &b1Hash)
	})
	if err != nil {
		t.Fatal(err)
	}

	extend := func(n int32) error {
		return walletdb.Update(ctx, w.db, func(dbtx walletdb.ReadWriteTx) error {
			ns := dbtx.ReadWriteBucket(wtxmgrNamespaceKey)
			prev, height := b1Hash, int32(1)
			for i := int32(0); i < n; i++ {
				height++
				h := &wire.BlockHeader{PrevBlock: prev, VoteBits: dcrutil.BlockValid, Height: uint32(height)}
				hHash := h.BlockHash()
				if err := w.txStore.ExtendMainChain(ns, h, &hHash, emptyFilter); err != nil {
					return err
				}
				prev = hHash
			}
			return nil
		})
	}

	// Immature: the ticket-purchase/VSP-fee reserve path cannot fund the
	// amount from the credit (insufficient balance), and listunspent does
	// not offer it.
	if _, err := w.ReserveOutputsForAmount(ctx, 0, amount, amountSKA, 1, ct); !werrors.Is(err, werrors.InsufficientBalance) {
		t.Fatalf("immature: ReserveOutputsForAmount error = %v, want insufficient balance", err)
	}
	unspent, err := w.ListUnspent(ctx, 1, 9999999, nil, "default", &ct)
	if err != nil {
		t.Fatal(err)
	}
	if len(unspent) != 0 {
		t.Fatalf("immature: ListUnspent returned %d outputs, want 0", len(unspent))
	}

	// Extend the chain past coinbase maturity.
	maturity := int32(params.CoinbaseMaturity)
	if err := extend(maturity + 5); err != nil {
		t.Fatal(err)
	}

	// Matured: the same credit is listed and reservable.
	unspent, err = w.ListUnspent(ctx, 1, 9999999, nil, "default", &ct)
	if err != nil {
		t.Fatal(err)
	}
	if len(unspent) != 1 {
		t.Fatalf("matured: ListUnspent returned %d outputs, want 1", len(unspent))
	}
	reserved, err := w.ReserveOutputsForAmount(ctx, 0, amount, amountSKA, 1, ct)
	if err != nil {
		t.Fatal(err)
	}
	if len(reserved) != 1 || reserved[0].OutPoint.Hash != txHash {
		t.Fatalf("matured: ReserveOutputsForAmount = %v, want the expiry credit %v",
			reserved, txHash)
	}
}

// TestCompressWalletInternalExcludesImmatureExpiryCredit checks the
// consolidation path behind compressWalletInternal, whose findEligibleOutputs
// call is reached by no other test here: a credit of a transaction with an
// expiry is eligible only once CoinbaseMaturity blocks have passed over it,
// so the consolidation spends the plain credits and leaves the expiry credit
// out of the transaction's inputs.
//
// The fixture mines two plain credits and one credit of a transaction with
// an expiry at height 1 and extends the chain to height 2: past the minconf
// 1 gate of compressWalletInternal, still far short of coinbase maturity.
func TestCompressWalletInternalExcludesImmatureExpiryCredit(t *testing.T) {
	ctx := context.Background()
	cfg := basicWalletConfig
	w, teardown := testWallet(ctx, t, &cfg, nil)
	defer teardown()

	params := w.chainParams
	// The change address of the consolidation comes from the internal
	// branch, which needs the private keys.
	if err := w.Unlock(ctx, testPrivPass, nil); err != nil {
		t.Fatal(err)
	}
	addr, err := w.NewExternalAddress(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, pkScript := addr.PaymentScript()

	newRec := func(withExpiry bool, v int64) *udb.TxRecord {
		tx := wire.NewMsgTx()
		if withExpiry {
			tx.Expiry = 100
		}
		tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&chainhash.Hash{0x01}, 0,
			wire.TxTreeRegular), 0, nil))
		tx.AddTxOut(wire.NewTxOut(v, pkScript))
		rec, err := udb.NewTxRecordFromMsgTx(tx, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return rec
	}
	plainA := newRec(false, 1e8)
	plainB := newRec(false, 2e8)
	expiryRec := newRec(true, 3e8)

	emptyFilter, _ := gcs2.FromBytesV2(blockcf2.B, blockcf2.M, nil)

	b1 := &wire.BlockHeader{PrevBlock: params.GenesisHash, VoteBits: dcrutil.BlockValid, Height: 1}
	b1Hash := b1.BlockHash()
	b2 := &wire.BlockHeader{PrevBlock: b1Hash, VoteBits: dcrutil.BlockValid, Height: 2}
	b2Hash := b2.BlockHash()
	err = walletdb.Update(ctx, w.db, func(dbtx walletdb.ReadWriteTx) error {
		ns := dbtx.ReadWriteBucket(wtxmgrNamespaceKey)
		if err := w.txStore.ExtendMainChain(ns, b1, &b1Hash, emptyFilter); err != nil {
			return err
		}
		if err := w.txStore.ExtendMainChain(ns, b2, &b2Hash, emptyFilter); err != nil {
			return err
		}
		for _, rec := range []*udb.TxRecord{plainA, plainB, expiryRec} {
			if err := w.txStore.InsertMemPoolTx(dbtx, rec); err != nil {
				return err
			}
			if err := w.txStore.AddCredit(dbtx, rec, nil, 0, false, 0); err != nil {
				return err
			}
			if err := w.txStore.InsertMinedTx(dbtx, rec, &b1Hash); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var msgtx *wire.MsgTx
	var lockedOps []wire.OutPoint
	err = walletdb.Update(ctx, w.db, func(dbtx walletdb.ReadWriteTx) error {
		var err error
		_, msgtx, lockedOps, err = w.compressWalletInternal(ctx,
			werrors.Op("test"), dbtx, 3, 0, nil, cointype.CoinTypeVAR)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, op := range lockedOps {
			delete(w.lockedOutpoints, outpoint{op.Hash, op.Index})
		}
	}()

	if len(msgtx.TxIn) != 2 {
		t.Fatalf("consolidation has %d inputs, want the 2 plain credits "+
			"(the immature expiry credit must be left out)", len(msgtx.TxIn))
	}
	seen := make(map[chainhash.Hash]bool, len(msgtx.TxIn))
	for _, in := range msgtx.TxIn {
		seen[in.PreviousOutPoint.Hash] = true
	}
	for _, rec := range []*udb.TxRecord{plainA, plainB} {
		if !seen[rec.Hash] {
			t.Errorf("plain credit %v not spent by the consolidation", &rec.Hash)
		}
	}
	if seen[expiryRec.Hash] {
		t.Errorf("immature expiry credit %v spent by the consolidation", &expiryRec.Hash)
	}
}
