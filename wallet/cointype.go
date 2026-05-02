// Copyright (c) 2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"context"

	"github.com/monetarium/monetarium-wallet/errors"
	"github.com/monetarium/monetarium-wallet/wallet/udb"
	"github.com/monetarium/monetarium-wallet/wallet/walletdb"
	"github.com/monetarium/monetarium-node/hdkeychain"
)

// UpgradeToSLIP0044CoinType upgrades the wallet from the legacy BIP0044 coin
// type to one of the coin types assigned to Decred in SLIP0044.  This should be
// called after a new wallet is created with a random (not imported) seed.
//
// This function does not register addresses from the new account 0 with the
// wallet's network backend.  This is intentional as it allows offline
// activities, such as wallet creation, to perform this upgrade.
func (w *Wallet) UpgradeToSLIP0044CoinType(ctx context.Context) error {
	const op errors.Op = "wallet.UpgradeToSLIP0044CoinType"

	var acctXpub, extBranchXpub, intBranchXpub *hdkeychain.ExtendedKey

	err := walletdb.Update(ctx, w.db, func(dbtx walletdb.ReadWriteTx) error {
		err := w.manager.UpgradeToSLIP0044CoinType(dbtx)
		if err != nil {
			return err
		}

		acctXpub, err = w.manager.AccountExtendedPubKey(dbtx, 0)
		if err != nil {
			return err
		}

		extBranchXpub, err = w.manager.AccountBranchExtendedPubKey(dbtx, 0,
			udb.ExternalBranch)
		if err != nil {
			return err
		}
		intBranchXpub, err = w.manager.AccountBranchExtendedPubKey(dbtx, 0,
			udb.InternalBranch)
		return err
	})
	if err != nil {
		return errors.E(op, err)
	}

	w.addressBuffersMu.Lock()
	w.addressBuffers[0] = &bip0044AccountData{
		xpub:        acctXpub,
		albExternal: addressBuffer{branchXpub: extBranchXpub, lastUsed: ^uint32(0)},
		albInternal: addressBuffer{branchXpub: intBranchXpub, lastUsed: ^uint32(0)},
	}
	w.addressBuffersMu.Unlock()

	return nil
}

// NeedsReregisteredSLIP0044Migration reports whether the wallet's currently
// active SLIP0044 cointype keys were derived under a coin type other than
// chaincfg.Params.SLIP0044CoinType.  When true, MigrateToReregisteredSLIP0044
// must be invoked (with the wallet's seed) to bring derivations onto the new
// path.  Detection does not require the wallet to be unlocked.
func (w *Wallet) NeedsReregisteredSLIP0044Migration(ctx context.Context) (bool, error) {
	const op errors.Op = "wallet.NeedsReregisteredSLIP0044Migration"

	var needs bool
	err := walletdb.View(ctx, w.db, func(dbtx walletdb.ReadTx) error {
		var err error
		needs, err = w.manager.NeedsReregisteredSLIP0044Migration(dbtx)
		return err
	})
	if err != nil {
		return false, errors.E(op, err)
	}
	return needs, nil
}

// MigrateToReregisteredSLIP0044 migrates a wallet's SLIP0044 cointype keys
// from a previously-used coin type (for Monetarium: 42, inherited from
// upstream Decred) to a freshly-registered coin type now declared in
// chaincfg.Params.SLIP0044CoinType (for Monetarium: 9508, registered via
// https://github.com/satoshilabs/slips/pull/2013).
//
// Because the seed is no longer stored in the database after database
// version 4 (noEncryptedSeedVersion), and BIP32 does not allow deriving
// sibling cointype keys from an existing cointype key, the seed must be
// supplied by the caller — typically by prompting the user for their
// recovery phrase.  An Invalid error is returned when the supplied seed
// does not match the wallet's existing keys, which UI layers may surface
// as "wrong seed" feedback.
//
// The wallet must be unlocked.  The historical cointype keys (e.g.
// 42-derived) are preserved under a separate database location so funds
// at the old derivation path remain spendable for sweep flows.
//
// This function is idempotent: calling it on an already-migrated wallet,
// a fresh wallet created at the new SLIP0044 path, or a chain where the
// legacy and SLIP0044 cointypes are equal (e.g. testnets using the
// universal testnet slot) returns nil with no database changes.  Callers
// may use NeedsReregisteredSLIP0044Migration to skip the seed prompt
// entirely when no migration is required.
//
// Concurrency: the migration commits the new on-disk derivations in a
// walletdb transaction, then takes addressBuffersMu to swap the cached
// branch xpubs.  Between those two steps, a concurrent reader of
// w.addressBuffers[0] may briefly observe the old buffer entries against
// the new on-disk derivations.  Callers must therefore serialize this
// against all other wallet operations on the same *Wallet — UI layers
// should gate it behind a "wallet is upgrading" state and only resume
// normal operations after the call returns.
func (w *Wallet) MigrateToReregisteredSLIP0044(ctx context.Context, seed []byte) error {
	const op errors.Op = "wallet.MigrateToReregisteredSLIP0044"

	var acctXpub, extBranchXpub, intBranchXpub *hdkeychain.ExtendedKey

	err := walletdb.Update(ctx, w.db, func(dbtx walletdb.ReadWriteTx) error {
		err := w.manager.MigrateToReregisteredSLIP0044(dbtx, seed)
		if err != nil {
			return err
		}

		acctXpub, err = w.manager.AccountExtendedPubKey(dbtx, 0)
		if err != nil {
			return err
		}

		extBranchXpub, err = w.manager.AccountBranchExtendedPubKey(dbtx, 0,
			udb.ExternalBranch)
		if err != nil {
			return err
		}
		intBranchXpub, err = w.manager.AccountBranchExtendedPubKey(dbtx, 0,
			udb.InternalBranch)
		return err
	})
	if err != nil {
		return errors.E(op, err)
	}

	w.addressBuffersMu.Lock()
	w.addressBuffers[0] = &bip0044AccountData{
		xpub:        acctXpub,
		albExternal: addressBuffer{branchXpub: extBranchXpub, lastUsed: ^uint32(0)},
		albInternal: addressBuffer{branchXpub: intBranchXpub, lastUsed: ^uint32(0)},
	}
	w.addressBuffersMu.Unlock()

	return nil
}
