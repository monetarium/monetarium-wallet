// Copyright (c) 2017-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package udb

import (
	"bytes"
	"context"
	"testing"

	"github.com/monetarium/monetarium-wallet/errors"
	"github.com/monetarium/monetarium-wallet/internal/compat"
	"github.com/monetarium/monetarium-wallet/wallet/walletdb"
	"github.com/monetarium/monetarium-node/chaincfg"
	"github.com/monetarium/monetarium-node/hdkeychain"
	"github.com/monetarium/monetarium-node/txscript/stdaddr"
)

func TestCoinTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		params                           *chaincfg.Params
		legacyCoinType, slip0044CoinType uint32
	}{
		{chaincfg.MainNetParams(), 42, 9508},
		{chaincfg.TestNet3Params(), 11, 1},
		{chaincfg.SimNetParams(), 115, 1},
	}
	for _, test := range tests {
		legacyCoinType, slip0044CoinType := CoinTypes(test.params)
		if legacyCoinType != test.legacyCoinType {
			t.Errorf("%s: got legacy coin type %d, expected %d", test.params.Name,
				legacyCoinType, test.legacyCoinType)
		}
		if slip0044CoinType != test.slip0044CoinType {
			t.Errorf("%s: got SLIP0044 coin type %d, expected %d", test.params.Name,
				slip0044CoinType, test.slip0044CoinType)
		}
	}
}

func deriveChildAddress(accountExtKey *hdkeychain.ExtendedKey, branch, child uint32, params *chaincfg.Params) (stdaddr.Address, error) {
	branchKey, err := accountExtKey.Child(branch)
	if err != nil {
		return nil, err
	}
	addressKey, err := branchKey.Child(child)
	if err != nil {
		return nil, err
	}
	return compat.HD2Address(addressKey, params)
}

func equalExtKeys(k0, k1 *hdkeychain.ExtendedKey) bool {
	return k0.String() == k1.String()
}

func TestCoinTypeUpgrade(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, teardown := tempDB(t)
	defer teardown()

	params := chaincfg.TestNet3Params()

	err := Initialize(ctx, db, params, seed, pubPass, privPassphrase)
	if err != nil {
		t.Fatal(err)
	}

	m, _, err := Open(ctx, db, params, pubPass)
	if err != nil {
		t.Fatal(err)
	}

	legacyCoinType, slip0044CoinType := CoinTypes(params)

	masterExtKey, err := hdkeychain.NewMaster(seed, params)
	if err != nil {
		t.Fatal(err)
	}
	legacyCoinTypeExtKey, err := deriveCoinTypeKey(masterExtKey, legacyCoinType)
	if err != nil {
		t.Fatal(err)
	}
	slip0044CoinTypeExtKey, err := deriveCoinTypeKey(masterExtKey, slip0044CoinType)
	if err != nil {
		t.Fatal(err)
	}
	slip0044Account0ExtKey, err := deriveAccountKey(slip0044CoinTypeExtKey, 0)
	if err != nil {
		t.Fatal(err)
	}
	slip0044Account0ExtKey = slip0044Account0ExtKey.Neuter()
	slip0044Account1ExtKey, err := deriveAccountKey(slip0044CoinTypeExtKey, 1)
	if err != nil {
		t.Fatal(err)
	}
	slip0044Account1ExtKey = slip0044Account1ExtKey.Neuter()
	slip0044Account0Address0, err := deriveChildAddress(slip0044Account0ExtKey, 0, 0, params)
	if err != nil {
		t.Fatal(err)
	}
	slip0044Account1Address0, err := deriveChildAddress(slip0044Account1ExtKey, 0, 0, params)
	if err != nil {
		t.Fatal(err)
	}
	slip0044Account0Address0Hash160 := slip0044Account0Address0.(stdaddr.Hash160er).Hash160()
	slip0044Account1Address0Hash160 := slip0044Account1Address0.(stdaddr.Hash160er).Hash160()

	err = walletdb.Update(ctx, db, func(dbtx walletdb.ReadWriteTx) error {
		ns := dbtx.ReadWriteBucket(waddrmgrBucketKey)
		err := m.Unlock(ns, privPassphrase)
		if err != nil {
			t.Fatal(err)
		}

		// Check reported initial coin type and compare the key itself against
		// the expected value.
		coinType, err := m.CoinType(dbtx)
		if err != nil {
			t.Fatal(err)
		}
		if coinType != legacyCoinType {
			t.Fatalf("initialized database has wrong coin type %d", coinType)
		}
		coinTypeExtKey, err := m.CoinTypePrivKey(dbtx)
		if err != nil {
			t.Fatal(err)
		}
		if !equalExtKeys(coinTypeExtKey, legacyCoinTypeExtKey) {
			t.Fatalf("initialized database has wrong coin type key")
		}

		// Perform the upgrade
		err = m.UpgradeToSLIP0044CoinType(dbtx)
		if err != nil {
			t.Fatal(err)
		}

		// Check upgraded coin type and keys.
		coinType, err = m.CoinType(dbtx)
		if err != nil {
			t.Fatal(err)
		}
		if coinType != slip0044CoinType {
			t.Fatalf("upgraded database has wrong coin type %d", coinType)
		}
		coinTypeExtKey, err = m.CoinTypePrivKey(dbtx)
		if err != nil {
			t.Fatal(err)
		}
		if !equalExtKeys(coinTypeExtKey, slip0044CoinTypeExtKey) {
			t.Fatalf("upgraded database has wrong coin type key")
		}

		// Check the account 0 xpub matches the one derived from the SLIP0044
		// coin type.
		accountExtKey, err := m.AccountExtendedPubKey(dbtx, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !equalExtKeys(accountExtKey, slip0044Account0ExtKey) {
			t.Fatalf("upgraded database has wrong account xpub")
		}

		// Check that the SLIP0044-derived account 0's first address can be
		// created and is indexed.
		err = m.SyncAccountToAddrIndex(ns, 0, 1, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !m.ExistsHash160(ns, slip0044Account0Address0Hash160[:]) {
			t.Fatalf("upgraded database does not record SLIP0044-derived account 0 branch 0 address 0")
		}

		// Create the next account, and perform all of the same checks on it as
		// the first account.
		_, err = m.NewAccount(ns, "account-1")
		if err != nil {
			t.Fatal(err)
		}
		accountExtKey, err = m.AccountExtendedPubKey(dbtx, 1)
		if err != nil {
			t.Fatal(err)
		}
		if !equalExtKeys(accountExtKey, slip0044Account1ExtKey) {
			t.Fatal("upgraded database derived wrong account xpub")
		}
		err = m.SyncAccountToAddrIndex(ns, 1, 1, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !m.ExistsHash160(ns, slip0044Account1Address0Hash160[:]) {
			t.Fatalf("upgraded database does not record SLIP0044-derived account 1 branch 0 address 0")
		}

		// Check that the upgrade can not be performed a second time.
		err = m.UpgradeToSLIP0044CoinType(dbtx)
		if !errors.Is(err, errors.Invalid) {
			t.Fatalf("upgrade database did not refuse second upgrade with errors.Invalid")
		}

		return nil
	})
	if err != nil {
		t.Error(err)
	}
}

// paramsWithCoinTypes returns a copy of base with LegacyCoinType and
// SLIP0044CoinType overridden.  Useful for tests that simulate a chain
// reconfiguring its derivation paths between releases.
//
// The copy is shallow: pointer/map/slice fields on chaincfg.Params (such
// as SKACoins) are aliased with base.  Tests that need to mutate one of
// those fields must deep-copy it themselves before assigning, otherwise
// they will corrupt the shared global params and pollute sibling tests.
func paramsWithCoinTypes(base *chaincfg.Params, legacy, slip0044 uint32) *chaincfg.Params {
	p := *base
	p.LegacyCoinType = legacy
	p.SLIP0044CoinType = slip0044
	return &p
}

// TestReregisteredSLIP0044Migration exercises MigrateToReregisteredSLIP0044
// — the wallet upgrade path used when a chain registers a new SLIP-0044
// coin type and existing wallets must move from the historical (e.g.
// upstream-inherited) value to the freshly registered one.
//
// The test simulates Monetarium's situation: a wallet was originally
// created with SLIP0044CoinType=42 (Decred's slot, inherited), and after
// SLIP-0044 PR #2013 lands the chaincfg now declares SLIP0044CoinType=9508
// with LegacyCoinType=42.  Existing wallet databases must be migrated
// without losing access to funds at the old derivation path.
func TestReregisteredSLIP0044Migration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, teardown := tempDB(t)
	defer teardown()

	// Phase 1 — set up a wallet at the "old" SLIP0044 path.  Use
	// TestNet3Params as the base; after upgrade the wallet's cointype
	// keys live at path 1.
	oldParams := chaincfg.TestNet3Params()
	if err := Initialize(ctx, db, oldParams, seed, pubPass, privPassphrase); err != nil {
		t.Fatal(err)
	}
	oldMgr, _, err := Open(ctx, db, oldParams, pubPass)
	if err != nil {
		t.Fatal(err)
	}
	err = walletdb.Update(ctx, db, func(dbtx walletdb.ReadWriteTx) error {
		ns := dbtx.ReadWriteBucket(waddrmgrBucketKey)
		if err := oldMgr.Unlock(ns, privPassphrase); err != nil {
			return err
		}
		return oldMgr.UpgradeToSLIP0044CoinType(dbtx)
	})
	if err != nil {
		t.Fatal(err)
	}
	oldMgr.Close()

	// Phase 2 — open the manager with new params declaring the chain's
	// re-registered SLIP-0044 slot.  Stored cointype keys are still at
	// path 1, but chaincfg now wants path 9508.
	const newSLIP0044 uint32 = 9508
	newParams := paramsWithCoinTypes(oldParams, oldParams.SLIP0044CoinType, newSLIP0044)
	m, _, err := Open(ctx, db, newParams, pubPass)
	if err != nil {
		t.Fatal(err)
	}

	// Detection helper should report migration is needed.
	var needs bool
	err = walletdb.View(ctx, db, func(dbtx walletdb.ReadTx) error {
		var err error
		needs, err = m.NeedsReregisteredSLIP0044Migration(dbtx)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !needs {
		t.Fatalf("NeedsReregisteredSLIP0044Migration returned false; want true")
	}

	// Compute what we expect post-migration so we can verify exact keys.
	masterExtKey, err := hdkeychain.NewMaster(seed, newParams)
	if err != nil {
		t.Fatal(err)
	}
	expectedNewCT, err := deriveCoinTypeKey(masterExtKey, newParams.SLIP0044CoinType)
	if err != nil {
		t.Fatal(err)
	}
	expectedNewAcct0Priv, err := deriveAccountKey(expectedNewCT, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Phase 3 — migration must reject a wrong seed with errors.Invalid
	// AND must leave on-disk state untouched.  The "must not mutate"
	// assertion is a regression guard for any future refactor that moves
	// state-changing logic above the seed-verification check.
	var preEncSLIP0044Pub, preEncSLIP0044Priv []byte
	var preOldPubExisted, preOldPrivExisted bool
	err = walletdb.View(ctx, db, func(dbtx walletdb.ReadTx) error {
		mainBucket := dbtx.ReadBucket(waddrmgrBucketKey).NestedReadBucket(mainBucketName)
		preEncSLIP0044Pub = append([]byte(nil), mainBucket.Get(coinTypeSLIP0044PubKeyName)...)
		preEncSLIP0044Priv = append([]byte(nil), mainBucket.Get(coinTypeSLIP0044PrivKeyName)...)
		preOldPubExisted = mainBucket.Get(coinTypeOldSLIP0044PubKeyName) != nil
		preOldPrivExisted = mainBucket.Get(coinTypeOldSLIP0044PrivKeyName) != nil
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = walletdb.Update(ctx, db, func(dbtx walletdb.ReadWriteTx) error {
		ns := dbtx.ReadWriteBucket(waddrmgrBucketKey)
		if err := m.Unlock(ns, privPassphrase); err != nil {
			return err
		}
		wrongSeed := append([]byte(nil), seed...)
		wrongSeed[0] ^= 0xff
		return m.MigrateToReregisteredSLIP0044(dbtx, wrongSeed)
	})
	if !errors.Is(err, errors.Invalid) {
		t.Fatalf("wrong seed not rejected with errors.Invalid: %v", err)
	}

	err = walletdb.View(ctx, db, func(dbtx walletdb.ReadTx) error {
		mainBucket := dbtx.ReadBucket(waddrmgrBucketKey).NestedReadBucket(mainBucketName)
		if !bytes.Equal(mainBucket.Get(coinTypeSLIP0044PubKeyName), preEncSLIP0044Pub) {
			t.Fatalf("wrong-seed error path mutated SLIP0044 cointype xpub on disk")
		}
		if !bytes.Equal(mainBucket.Get(coinTypeSLIP0044PrivKeyName), preEncSLIP0044Priv) {
			t.Fatalf("wrong-seed error path mutated SLIP0044 cointype xpriv on disk")
		}
		if !preOldPubExisted && mainBucket.Get(coinTypeOldSLIP0044PubKeyName) != nil {
			t.Fatalf("wrong-seed error path wrote the old-SLIP0044 backup pub bucket")
		}
		if !preOldPrivExisted && mainBucket.Get(coinTypeOldSLIP0044PrivKeyName) != nil {
			t.Fatalf("wrong-seed error path wrote the old-SLIP0044 backup priv bucket")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Phase 4 — correct seed migrates successfully.
	err = walletdb.Update(ctx, db, func(dbtx walletdb.ReadWriteTx) error {
		return m.MigrateToReregisteredSLIP0044(dbtx, seed)
	})
	if err != nil {
		t.Fatalf("migration with correct seed failed: %v", err)
	}

	// Phase 5 — verify post-migration state.
	err = walletdb.View(ctx, db, func(dbtx walletdb.ReadTx) error {
		ctKey, err := m.CoinTypePrivKey(dbtx)
		if err != nil {
			return err
		}
		if !equalExtKeys(ctKey, expectedNewCT) {
			t.Fatalf("post-migration cointype xpriv mismatch:\n  got:  %s\n  want: %s",
				ctKey.String(), expectedNewCT.String())
		}

		accountExtKey, err := m.AccountExtendedPubKey(dbtx, 0)
		if err != nil {
			return err
		}
		if !equalExtKeys(accountExtKey, expectedNewAcct0Priv.Neuter()) {
			t.Fatalf("post-migration account 0 xpub mismatch")
		}

		oldPubEnc, oldPrivEnc, err := fetchCoinTypeOldSLIP0044Keys(dbtx.ReadBucket(waddrmgrBucketKey))
		if err != nil {
			return err
		}
		if oldPubEnc == nil || oldPrivEnc == nil {
			t.Fatalf("old SLIP0044 keys not preserved after migration")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Phase 6 — detection now reports false; idempotent re-run is a no-op.
	err = walletdb.View(ctx, db, func(dbtx walletdb.ReadTx) error {
		var err error
		needs, err = m.NeedsReregisteredSLIP0044Migration(dbtx)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if needs {
		t.Fatalf("NeedsReregisteredSLIP0044Migration returned true after migration; want false")
	}
	err = walletdb.Update(ctx, db, func(dbtx walletdb.ReadWriteTx) error {
		return m.MigrateToReregisteredSLIP0044(dbtx, seed)
	})
	if err != nil {
		t.Fatalf("idempotent re-run failed: %v", err)
	}
}
