// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package udb

import (
	"context"
	"testing"
	"time"

	"github.com/monetarium/monetarium-node/chaincfg"
	"github.com/monetarium/monetarium-node/chaincfg/chainhash"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/dcrutil"
	"github.com/monetarium/monetarium-node/txscript/stdaddr"
	"github.com/monetarium/monetarium-node/wire"
	"github.com/monetarium/monetarium-wallet/wallet/walletdb"
)

// TestRemoveUnconfirmedDrainsMultisigOutputBuckets pins the root-cause fix
// for the "phantom multisig credit" bug: when a wallet-authored multisig tx
// fails to publish, AbandonTransaction → RemoveUnconfirmed must also drain
// bucketMultisig and bucketMultisigUsp for the abandoned tx's outputs. Before
// the fix, only deleteRawUnminedCredit ran on the output side and the
// speculative entry from insertMultisigOutIntoTxMgr leaked permanently,
// surfacing later as a redeemmultisigouts "transaction for an output that
// doesn't exist."
func TestRemoveUnconfirmedDrainsMultisigOutputBuckets(t *testing.T) {
	ctx := context.Background()
	db, _, store, teardown, err := cloneDB(ctx, "TestRemoveUnconfirmedDrainsMultisigOutputBuckets.kv")
	if err != nil {
		t.Fatal(err)
	}
	defer teardown()

	// Build a synthetic single-output tx. The output's canonical key is what
	// insertMultisigOutIntoTxMgr would seed into the multisig buckets.
	tx := wire.NewMsgTx()
	tx.AddTxOut(&wire.TxOut{Value: 1_000_000, PkScript: []byte{0xa9, 0x14}}) // P2SH-shaped prefix, content irrelevant for this test
	txHash := tx.TxHash()
	k := canonicalOutPoint(&txHash, 0)

	// Seed both buckets with dummy values; existsMultisigOut only checks for
	// non-nil presence, so we don't need a fully-formed multisig record.
	err = walletdb.Update(ctx, db, func(dbtx walletdb.ReadWriteTx) error {
		ns := dbtx.ReadWriteBucket(wtxmgrBucketKey)
		if err := ns.NestedReadWriteBucket(bucketMultisig).Put(k, []byte("dummy-multisig-record")); err != nil {
			return err
		}
		return putMultisigOutUS(ns, k)
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Sanity-check the seed reads back.
	err = walletdb.View(ctx, db, func(dbtx walletdb.ReadTx) error {
		ns := dbtx.ReadBucket(wtxmgrBucketKey)
		if existsMultisigOut(ns, k) == nil {
			t.Fatalf("seed missing in bucketMultisig")
		}
		if !existsMultisigOutUS(ns, k) {
			t.Fatalf("seed missing in bucketMultisigUsp")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed verify: %v", err)
	}

	// Run RemoveUnconfirmed for the synthetic tx. We construct a TxRecord
	// inline rather than going through InsertMemPoolTx — the fix lives in
	// the output-iteration loop and doesn't depend on the unmined record
	// being present.
	rec, err := NewTxRecordFromMsgTx(tx, time.Now())
	if err != nil {
		t.Fatalf("NewTxRecordFromMsgTx: %v", err)
	}
	err = walletdb.Update(ctx, db, func(dbtx walletdb.ReadWriteTx) error {
		ns := dbtx.ReadWriteBucket(wtxmgrBucketKey)
		return store.RemoveUnconfirmed(ns, &rec.MsgTx, &rec.Hash)
	})
	if err != nil {
		t.Fatalf("RemoveUnconfirmed: %v", err)
	}

	// Both multisig buckets must now be empty for this outpoint. Before the
	// fix, both still contained the seeded entry.
	err = walletdb.View(ctx, db, func(dbtx walletdb.ReadTx) error {
		ns := dbtx.ReadBucket(wtxmgrBucketKey)
		if v := existsMultisigOut(ns, k); v != nil {
			t.Errorf("bucketMultisig still contains the abandoned outpoint: %x", v)
		}
		if existsMultisigOutUS(ns, k) {
			t.Errorf("bucketMultisigUsp still contains the abandoned outpoint")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	// Verify a non-existent outpoint is a no-op (existsMultisigOut check
	// gates the delete, so a tx without speculative multisig records must
	// not error out).
	var otherHash chainhash.Hash
	otherHash[0] = 0xFF
	otherTx := wire.NewMsgTx()
	otherTx.AddTxOut(&wire.TxOut{Value: 0})
	otherRec, err := NewTxRecordFromMsgTx(otherTx, time.Now())
	if err != nil {
		t.Fatalf("NewTxRecordFromMsgTx other: %v", err)
	}
	err = walletdb.Update(ctx, db, func(dbtx walletdb.ReadWriteTx) error {
		ns := dbtx.ReadWriteBucket(wtxmgrBucketKey)
		return store.RemoveUnconfirmed(ns, &otherRec.MsgTx, &otherRec.Hash)
	})
	if err != nil {
		t.Errorf("RemoveUnconfirmed on tx with no multisig records should succeed, got: %v", err)
	}
}

// TestRemoveUnconfirmedHidesDrainedOutpointFromQuery extends the bucket-level
// drain assertion with operator-visible behaviour: after RemoveUnconfirmed, the
// public UnspentMultisigCreditsForAddress query must no longer surface the
// abandoned outpoint. The bucket-level test alone would not catch a future
// regression where a new bucket holds multisig-credit state.
func TestRemoveUnconfirmedHidesDrainedOutpointFromQuery(t *testing.T) {
	ctx := context.Background()
	db, mgr, store, teardown, err := cloneDB(ctx, "TestRemoveUnconfirmedHidesDrainedOutpointFromQuery.kv")
	if err != nil {
		t.Fatal(err)
	}
	defer teardown()

	// 1-of-1 multisig redeem script reused from addressmanager_test.go: a
	// minimal valid script whose hash160 is db7e6d50…1f4f9c. Importing the
	// script registers it with the addrmgr so that
	// UnspentMultisigCreditsForAddress's redeemScriptForHash160 lookup
	// succeeds for the corresponding P2SH address.
	redeemScript := hexToBytes("51210373c717acda38b5aa4c00c33932e059cdbc" +
		"11deceb5f00490a9101704cc444c5151ae")
	scriptHash160 := hexToBytes("db7e6d507e3e291a5ab2fac10107f4479c1f4f9c")
	var scriptHash [20]byte
	copy(scriptHash[:], scriptHash160)

	err = walletdb.Update(ctx, db, func(dbtx walletdb.ReadWriteTx) error {
		ns := dbtx.ReadWriteBucket(waddrmgrBucketKey)
		if err := mgr.Unlock(ns, privPassphrase); err != nil {
			return err
		}
		_, err := mgr.ImportScript(ns, redeemScript)
		return err
	})
	if err != nil {
		t.Fatalf("import multisig script: %v", err)
	}

	// Synthetic tx containing a single P2SH output. The PkScript content is
	// not consulted by RemoveUnconfirmed (the drain is keyed by canonical
	// outpoint), and UnspentMultisigCreditsForAddress reads scriptHash from
	// the bucketMultisig value rather than the txout.
	tx := wire.NewMsgTx()
	tx.AddTxOut(&wire.TxOut{Value: 1_000_000, PkScript: []byte{0xa9, 0x14}})
	txHash := tx.TxHash()
	k := canonicalOutPoint(&txHash, 0)

	// Seed bucketMultisig with a real valueMultisigOut payload so the public
	// query can fully decode the entry, and seed bucketMultisigUsp so the
	// cursor surfaces it.
	value, err := valueMultisigOut(scriptHash, 1, 1, false,
		wire.TxTreeRegular, chainhash.Hash{}, 0,
		dcrutil.Amount(1_000_000), chainhash.Hash{}, 0, txHash,
		cointype.CoinTypeVAR, cointype.SKAAmount{})
	if err != nil {
		t.Fatalf("valueMultisigOut: %v", err)
	}
	err = walletdb.Update(ctx, db, func(dbtx walletdb.ReadWriteTx) error {
		ns := dbtx.ReadWriteBucket(wtxmgrBucketKey)
		if err := ns.NestedReadWriteBucket(bucketMultisig).Put(k, value); err != nil {
			return err
		}
		return putMultisigOutUS(ns, k)
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	p2shAddr, err := stdaddr.NewAddressScriptHashV0FromHash(scriptHash[:],
		chaincfg.TestNet3Params())
	if err != nil {
		t.Fatalf("p2sh addr: %v", err)
	}

	// Pre-drain sanity check: the query must surface the seeded credit.
	// Without this assertion a regression that breaks the seed shape would
	// silently make the post-drain check vacuous.
	err = walletdb.View(ctx, db, func(dbtx walletdb.ReadTx) error {
		mscs, err := store.UnspentMultisigCreditsForAddress(dbtx, p2shAddr)
		if err != nil {
			return err
		}
		if len(mscs) != 1 {
			t.Fatalf("pre-drain: expected 1 multisig credit, got %d", len(mscs))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("pre-drain view: %v", err)
	}

	// Abandon the tx. The fix in RemoveUnconfirmed must drain both buckets.
	rec, err := NewTxRecordFromMsgTx(tx, time.Now())
	if err != nil {
		t.Fatalf("NewTxRecordFromMsgTx: %v", err)
	}
	err = walletdb.Update(ctx, db, func(dbtx walletdb.ReadWriteTx) error {
		ns := dbtx.ReadWriteBucket(wtxmgrBucketKey)
		return store.RemoveUnconfirmed(ns, &rec.MsgTx, &rec.Hash)
	})
	if err != nil {
		t.Fatalf("RemoveUnconfirmed: %v", err)
	}

	// Post-drain: the public query must return zero credits for the
	// abandoned outpoint's P2SH address. Before the fix this returned the
	// phantom entry, which is what reached redeemmultisigouts and produced
	// an "output that doesn't exist" raw transaction.
	err = walletdb.View(ctx, db, func(dbtx walletdb.ReadTx) error {
		mscs, err := store.UnspentMultisigCreditsForAddress(dbtx, p2shAddr)
		if err != nil {
			return err
		}
		if len(mscs) != 0 {
			t.Errorf("post-drain: expected 0 multisig credits, got %d", len(mscs))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("post-drain view: %v", err)
	}
}
