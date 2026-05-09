// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package udb

import (
	"context"
	"testing"

	"github.com/monetarium/monetarium-node/chaincfg"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-wallet/wallet/walletdb"
)

// TestExistsRawUnspentFindsDeactivatedCoinTypeUTXO pins the [1, 256) fallback
// loop in existsRawUnspent. A coin type may have been active when its UTXO
// was recorded but later removed from chain params (governance deactivation,
// chain-param refactor, or a wallet opened against a network missing that
// SKA coin entry). Without the fallback walk those credits would be invisible
// to existsRawUnspent and produce double-spend errors. Pin from the
// 2026-05-04 review: this scenario was untested.
func TestExistsRawUnspentFindsDeactivatedCoinTypeUTXO(t *testing.T) {
	ctx := context.Background()
	db, _, _, teardown, err := cloneDB(ctx, "TestExistsRawUnspentFindsDeactivatedCoinTypeUTXO.kv")
	if err != nil {
		t.Fatal(err)
	}
	defer teardown()

	// Synthetic 36-byte unspent key (32-byte tx hash + 4-byte vout BE).
	k := make([]byte, 36)
	for i := range k {
		k[i] = byte(i + 1)
	}
	// Synthetic 36-byte body (block hash + 4-byte vout) the lookup will copy
	// into the constructed credit key.
	v := make([]byte, 36)
	for i := range v {
		v[i] = byte(0xA0 + i)
	}

	// Use a coin type that is NOT in chain params, simulating a previously
	// active SKA coin that has since been removed from configuration.
	const deactivatedCT = cointype.CoinType(7)

	// Seed the per-coin-type unspent bucket directly.
	err = walletdb.Update(ctx, db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(wtxmgrBucketKey)
		bucket := ns.NestedReadWriteBucket(bucketUnspentForCoinType(deactivatedCT))
		if bucket == nil {
			b, err := ns.CreateBucket(bucketUnspentForCoinType(deactivatedCT))
			if err != nil {
				return err
			}
			bucket = b
		}
		return bucket.Put(k, v)
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Chain params with NO SKA coins configured — getActiveSKACoinTypesFromParams
	// will return an empty slice, forcing existsRawUnspent into the fallback.
	params := &chaincfg.Params{}

	err = walletdb.View(ctx, db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(wtxmgrBucketKey)
		credKey := existsRawUnspent(ns, k, params)
		if credKey == nil {
			t.Fatalf("expected fallback to find deactivated coin type %d UTXO; got nil", deactivatedCT)
		}
		if len(credKey) != 72 {
			t.Errorf("credit key length = %d, want 72", len(credKey))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("view: %v", err)
	}
}

// TestExistsRawUnspentReturnsNilWhenAbsent confirms the fallback walk does
// not produce false positives. With nothing seeded in any coin-type bucket,
// the [1, 256) loop must terminate with a nil return.
func TestExistsRawUnspentReturnsNilWhenAbsent(t *testing.T) {
	ctx := context.Background()
	db, _, _, teardown, err := cloneDB(ctx, "TestExistsRawUnspentReturnsNilWhenAbsent.kv")
	if err != nil {
		t.Fatal(err)
	}
	defer teardown()

	k := make([]byte, 36)
	for i := range k {
		k[i] = 0xFF
	}

	params := &chaincfg.Params{}
	err = walletdb.View(ctx, db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(wtxmgrBucketKey)
		if got := existsRawUnspent(ns, k, params); got != nil {
			t.Errorf("expected nil for absent key; got %x", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("view: %v", err)
	}
}

// TestExistsRawUnspentRejectsShortKey pins the input-length guard. A future
// caller passing a malformed key shorter than 36 bytes must get nil rather
// than indexing past the slice bounds.
func TestExistsRawUnspentRejectsShortKey(t *testing.T) {
	ctx := context.Background()
	db, _, _, teardown, err := cloneDB(ctx, "TestExistsRawUnspentRejectsShortKey.kv")
	if err != nil {
		t.Fatal(err)
	}
	defer teardown()

	params := &chaincfg.Params{}
	err = walletdb.View(ctx, db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(wtxmgrBucketKey)
		for _, n := range []int{0, 1, 35} {
			short := make([]byte, n)
			if got := existsRawUnspent(ns, short, params); got != nil {
				t.Errorf("len(k)=%d: got %x, want nil", n, got)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("view: %v", err)
	}
}
