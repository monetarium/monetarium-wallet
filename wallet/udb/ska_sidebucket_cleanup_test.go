// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package udb

import (
	"context"
	"math/big"
	"testing"

	"github.com/monetarium/monetarium-node/chaincfg"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-wallet/wallet/walletdb"
)

// TestDeleteRawCreditCascadesToSKABucket verifies that deleteRawCredit
// removes any SKA side-bucket entry keyed by the same credit key. Without
// this cascade, every rollback / stake-invalidate / unmined-credit removal
// of an SKA credit leaks an entry (the original MEDIUM finding).
func TestDeleteRawCreditCascadesToSKABucket(t *testing.T) {
	ctx := context.Background()
	db, _, _, teardown, err := cloneDB(ctx, "TestDeleteRawCreditCascadesToSKABucket.kv")
	if err != nil {
		t.Fatal(err)
	}
	defer teardown()

	// Synthetic 72-byte mined credit key.
	k := make([]byte, 72)
	for i := range k {
		k[i] = byte(i)
	}
	skaAmt := cointype.NewSKAAmount(big.NewInt(int64(1<<62) + 12345))

	err = walletdb.Update(ctx, db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(wtxmgrBucketKey)
		// Seed both buckets — the live credit body (any non-empty value)
		// and the SKA side-bucket entry.
		if err := ns.NestedReadWriteBucket(bucketCredits).Put(k, []byte("dummy")); err != nil {
			return err
		}
		return putSKACreditAmount(ns, k, skaAmt)
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	err = walletdb.Update(ctx, db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(wtxmgrBucketKey)
		return deleteRawCredit(ns, k)
	})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Verify both buckets are clean.
	err = walletdb.View(ctx, db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(wtxmgrBucketKey)
		if v := ns.NestedReadBucket(bucketCredits).Get(k); v != nil {
			t.Errorf("expected bucketCredits[k] removed; got %x", v)
		}
		got, err := fetchSKACreditAmount(ns, k)
		if err != nil {
			t.Errorf("fetchSKACreditAmount: %v", err)
		}
		if !got.IsZero() {
			t.Errorf("expected SKA side-bucket entry removed; got non-zero amount")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
}

// TestDeleteRawUnminedCreditCascadesToSKABucket verifies the unmined
// counterpart: deleteRawUnminedCredit clears bucketSKAUnminedCreditAmounts.
func TestDeleteRawUnminedCreditCascadesToSKABucket(t *testing.T) {
	ctx := context.Background()
	db, _, _, teardown, err := cloneDB(ctx, "TestDeleteRawUnminedCreditCascadesToSKABucket.kv")
	if err != nil {
		t.Fatal(err)
	}
	defer teardown()

	// Synthetic 36-byte unmined credit key.
	k := make([]byte, 36)
	for i := range k {
		k[i] = byte(i)
	}
	skaAmt := cointype.NewSKAAmount(big.NewInt(int64(1<<60) + 7))
	params := chaincfg.TestNet3Params()

	err = walletdb.Update(ctx, db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(wtxmgrBucketKey)
		// The cascade runs unconditionally regardless of whether a primary
		// per-coin-type unmined credit exists. Seed only the side bucket.
		return putSKAUnminedCreditAmount(ns, k, skaAmt)
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	err = walletdb.Update(ctx, db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(wtxmgrBucketKey)
		return deleteRawUnminedCredit(ns, k, params)
	})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	err = walletdb.View(ctx, db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(wtxmgrBucketKey)
		got, err := fetchSKAUnminedCreditAmount(ns, k)
		if err != nil {
			t.Errorf("fetchSKAUnminedCreditAmount: %v", err)
		}
		if !got.IsZero() {
			t.Errorf("expected SKA unmined side-bucket entry removed; got non-zero")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
}

// TestSKAStakeInvalidatedAccessors verifies the new archive-bucket put/fetch/
// delete trio. The archive bucket is created by the v33 upgrade, but cloneDB
// returns a wallet at the latest version, so it is already present.
func TestSKAStakeInvalidatedAccessors(t *testing.T) {
	ctx := context.Background()
	db, _, _, teardown, err := cloneDB(ctx, "TestSKAStakeInvalidatedAccessors.kv")
	if err != nil {
		t.Fatal(err)
	}
	defer teardown()

	k := make([]byte, 72)
	for i := range k {
		k[i] = byte(i + 1)
	}
	// Use a big.Int that overflows int64 to ensure the round-trip preserves
	// the full big.Int precision and not just the low int64 bits.
	wantBig, _ := new(big.Int).SetString("9223372036854775808", 10) // 2^63
	want := cointype.NewSKAAmount(wantBig)

	err = walletdb.Update(ctx, db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(wtxmgrBucketKey)
		return putSKAStakeInvalidatedCreditAmount(ns, k, want)
	})
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	err = walletdb.View(ctx, db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(wtxmgrBucketKey)
		got, err := fetchSKAStakeInvalidatedCreditAmount(ns, k)
		if err != nil {
			t.Errorf("fetchSKAStakeInvalidatedCreditAmount: %v", err)
		}
		if got.BigInt().Cmp(want.BigInt()) != 0 {
			t.Errorf("fetch mismatch: got %s, want %s", got.BigInt(), want.BigInt())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify-fetch: %v", err)
	}

	err = walletdb.Update(ctx, db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(wtxmgrBucketKey)
		return deleteSKAStakeInvalidatedCreditAmount(ns, k)
	})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	err = walletdb.View(ctx, db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(wtxmgrBucketKey)
		got, err := fetchSKAStakeInvalidatedCreditAmount(ns, k)
		if err != nil {
			t.Errorf("fetchSKAStakeInvalidatedCreditAmount: %v", err)
		}
		if !got.IsZero() {
			t.Errorf("expected zero after delete; got %s", got.BigInt())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify-delete: %v", err)
	}
}

