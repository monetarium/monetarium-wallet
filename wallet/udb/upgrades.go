// Copyright (c) 2017-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package udb

import (
	"context"
	"crypto/sha256"

	"github.com/monetarium/monetarium-wallet/errors"
	"github.com/monetarium/monetarium-wallet/internal/compat"
	"github.com/monetarium/monetarium-wallet/wallet/internal/snacl"
	"github.com/monetarium/monetarium-wallet/wallet/walletdb"
	"github.com/monetarium/monetarium-node/blockchain/stake"
	"github.com/monetarium/monetarium-node/chaincfg/chainhash"
	"github.com/monetarium/monetarium-node/chaincfg"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/dcrutil"
	"github.com/monetarium/monetarium-node/gcs/blockcf2"
	"github.com/monetarium/monetarium-node/hdkeychain"
	"github.com/monetarium/monetarium-node/txscript/stdscript"
	"github.com/monetarium/monetarium-node/wire"
)

// Note: all manager functions always use the latest version of the database.
// Therefore it is extremely important when adding database upgrade code to
// never call any methods of the managers and instead only use the db primitives
// with the correct version passed as parameters.

const (
	initialVersion = 1

	// lastUsedAddressIndexVersion is the second version of the database.  It
	// adds indexes for the last used address of BIP0044 accounts, removes the
	// next to use address indexes, removes all references to address pools, and
	// removes all per-address usage tracking.
	//
	// See lastUsedAddressIndexUpgrade for the code that implements the upgrade
	// path.
	lastUsedAddressIndexVersion = 2

	// votingPreferencesVersion is the third version of the database.  It
	// removes all per-ticket vote bits, replacing them with vote preferences
	// for choices on individual agendas from the current stake version.
	votingPreferencesVersion = 3

	// noEncryptedSeedVersion is the fourth version of the database.  It removes
	// the encrypted seed that earlier versions may have saved in the database
	// (or more commonly, encrypted zeros on mainnet wallets).
	noEncryptedSeedVersion = 4

	// lastReturnedAddressVersion is the fifth version of the database.  It adds
	// additional indexes to each BIP0044 account row that keep track of the
	// index of the last returned child address in the internal and external
	// account branches.  This is used to prevent returning identical addresses
	// across application restarts.
	lastReturnedAddressVersion = 5

	// ticketBucketVersion is the sixth version of the database.  It adds a
	// bucket for recording the hashes of all tickets and provides additional
	// APIs to check the status of tickets and whether they are spent by a vote
	// or revocation.
	ticketBucketVersion = 6

	// slip0044CoinTypeVersion is the seventh version of the database.  It
	// introduces the possibility of the BIP0044 coin type key being either the
	// legacy coin type used by earlier versions of the wallet, or the coin type
	// assigned to Decred in SLIP0044.  The upgrade does not add or remove any
	// required keys (the upgrade is done in a backwards-compatible way) but the
	// database version is bumped to prevent older software from assuming that
	// coin type 20 exists (the upgrade is not forwards-compatible).
	slip0044CoinTypeVersion = 7

	// hasExpiryVersion is the eight version of the database. It adds the
	// hasExpiry field to the credit struct, adds fetchRawCreditHasExpiry
	// helper func and extends sstxchange type utxo checks to only make sstchange
	// with expiries set available to spend after coinbase maturity (16 blocks).
	hasExpiryVersion = 8

	// hasExpiryFixedVersion is the ninth version of the database.  It corrects
	// the previous upgrade by writing the has expiry bit to an unused bit flag
	// rather than in the stake flags and fixes various UTXO selection issues
	// caused by misinterpreting ticket outputs as spendable by regular
	// transactions.
	hasExpiryFixedVersion = 9

	// cfVersion is the tenth version of the database.  It adds a bucket to
	// store compact filters, which are required for Decred's SPV
	// implementation, and a txmgr namespace root key which tracks whether all
	// main chain compact filters were saved.  This version does not begin to
	// save compact filter headers, since the SPV implementation is expected to
	// use header commitments in a later release for validation.
	cfVersion = 10

	// lastProcessedTxsBlockVersion is the eleventh version of the database.  It
	// adds a txmgr namespace root key which records the final hash of all
	// blocks since the genesis block which have been processed for relevant
	// transactions.  This is required to distinguish between the main chain tip
	// (which is advanced during headers fetch) and the point at which a startup
	// rescan should occur.  During upgrade, the current tip block is recorded
	// as this block to avoid an additional or extra long rescan from occurring
	// from properly-synced wallets.
	lastProcessedTxsBlockVersion = 11

	// ticketCommitmentsVersion the twelfth version of the database. It adds
	// the ticketCommitment bucket to the txmgr namespace. This bucket is meant
	// to track outstanding ticket commitment outputs for the purposes of
	// correct balance calculation: it allows non-voting wallets (eg: funding
	// wallets in solo-voting setups or non-voter participants of split tickets)
	// to track their proportional locked funds. In standard (single-voter) VSP
	// setups, it also allows the wallet to discount the pool fee for correct
	// accounting of total locked funds.
	ticketCommitmentsVersion = 12

	// importedXpubAccountVersion is the thirteenth version of the
	// database.  It introduces the ability to import and track child
	// indexes of arbitrary HD extended public keys.  Imported xpub accounts
	// are associated with a uint32 value in the upper range (above
	// ImportedAddrAccount).  The upgrade does not add or remove any
	// required keys (the upgrade is done in a backwards-compatible way) but
	// the database version is bumped to prevent older software from using
	// an upgraded database with account identifiers that would induce
	// panics.
	importedXpubAccountVersion = 13

	// unencryptedRedeemScriptsVersion is the 14th version of the database.
	// The goal of this upgrade is to make it possible to read P2SH redeem
	// scripts through the address manager without needing the wallet to be
	// unlocked.  This is made possible by relying on the fact that since
	// mainnet launch, upstream Decred wallet has always recorded imported P2SH redeem
	// scripts both encrypted in the address manager bucket, and unencrypted
	// in the transaction store buckets.  During this upgrade, a check is
	// performed ensuring that no unencrypted scripts are missing.
	// Unencrypted scripts are read from the transaction store buckets and
	// the address manager values are rewritten with these plaintext
	// scripts.  The now-unnecessary transaction store scripts are removed
	// in the upgrade.
	unencryptedRedeemScriptsVersion = 14

	// blockcf2Version is the 15th version of the databse. This upgrade
	// drops the existing cfilter bucket and recreates it, triggering a
	// re-download of missing cfilters. This is intended to switch the
	// wallet to using the new, consensus-enforced version 2 committed
	// filters.
	blockcf2Version = 15

	// perTicketVotingPreferencesVersion is the 16th version of the database.
	// It creates a top-level ticketsagendaprefs bucket for storing voting
	// preferences for individual tickets.
	perTicketVotingPreferencesVersion = 16

	// accountVariablesVersion is the 17th version of the database.  It adds
	// a bucket for each account which records individual key/value pairs
	// for account metadata which can change over time (such as account
	// names, last used child indexes, etc).  This is more efficient than
	// reserializing all account metadata into a single field when only some
	// fields changed.
	accountVariablesVersion = 17

	// unpublishedTxsVersion is the 18th version of the database.  It
	// adds a bucket for recording unmined transactions which have not yet
	// been published to the network, but still recording them to the
	// database and not considering spent previous outputs to be UTXOs
	// unless the transaction is abandoned.
	unpublishedTxsVersion = 18

	// tspendPolicyVersion is the 19th version of the database.  It adds a
	// top-level bucket for recording voting policy on treasury-spending
	// transactions.
	tspendPolicyVersion = 19

	// vspBucketVersion is the 20th version of the database. It adds a
	// top-level bucket for recording vsp ticket hashes as key and its
	// related fee txs hash.
	vspBucketVersion = 20

	// vspStatusVersion is the 21st version of the database.  It adds a
	// status field to tracked vspd tickets.
	vspStatusVersion = 21

	// tspendHashPolicyVersion is the 22nd version of the database.  It adds a
	// top-level bucket for recording voting policy on treasury-spending
	// transactions by transaction hash, rather than by pi key.
	tspendHashPolicyVersion = 22

	// vspHostVersion is the 23nd version of the database.  It adds a
	// vsp host ane vsp pubkey buckets to the db.
	vspHostVersion = 23

	// vspTreasuryPoliciesVersion is the 24th version of the database.  It
	// adds top-level buckets for recording the voting policies
	// treasury-spending transactions for specific customer's tickets served
	// by a VSP.
	vspTreasuryPoliciesVersion = 24

	// importVotingAccount is the 25th version of the database. This version
	// indicates that importing a new account type has been enabled. This
	// account facilitates voting using a special account where the private
	// key from indexes of the internal branch are shared with a vsp. The
	// new type is not recognized by previous wallet versions. This version
	// only updates the db version number so that previous versions will
	// error on startup.
	importVotingAccountVersion = 25

	// birthBlockVersion is the 26th version of the database. It allows
	// saving a birth state. For all current wallets, set the birth block to
	// the genesis block.
	birthBlockVersion = 26

	// dualCoinVersion is the 27th version of the database. It adds support
	// for the dual-coin system by extending unmined credits to store coin type,
	// adding per-coin-type balance tracking, and creating coin-type-aware
	// unspent indexes for efficient queries.
	dualCoinVersion = 27

	// coinTypeBucketsVersion is the 28th version of the database. It creates
	// separate buckets for each coin type to enable efficient coin-type-specific
	// UTXO queries without runtime filtering.
	coinTypeBucketsVersion = 28

	// consolidationAddressVersion is the 29th version of the database. It creates
	// a bucket for storing per-account consolidation addresses used for SSFee
	// UTXO consolidation in vote transactions.
	consolidationAddressVersion = 29

	// skaBucketsVersion is the 30th version of the database. It creates
	// buckets for SKA credit amounts (big.Int storage) and SSFee transaction
	// markers (to avoid deserializing transactions for marker checks).
	skaBucketsVersion = 30

	// wireFormatV13Version is the 31st version of the database. It migrates
	// all transaction records from the V12 wire format (where TxOut was serialized
	// as [Value:8][CoinType:1][Version:2][PkScript:var]) to the V13 wire format
	// (where TxOut is serialized as [CoinType:1][Value:8+][Version:2][PkScript:var]).
	// This change was necessary to support variable-length SKA amounts (big.Int).
	wireFormatV13Version = 31

	// multisigCoinTypeVersion is the 32nd version of the database. It extends
	// the persisted P2SH multisig-output record with CoinType (1 byte) and
	// SKAAmount (length-prefixed big.Int bytes) tail fields. All pre-upgrade
	// entries are VAR-only and backfilled as CoinType=VAR / SKAAmount=Zero.
	multisigCoinTypeVersion = 32

	// skaSideBucketCleanupVersion is the 33rd version of the database. It
	// creates bucketSKAStakeInvalidatedCreditAmounts and prunes any orphan
	// entries from bucketSKACreditAmounts and bucketSKAUnminedCreditAmounts
	// whose primary credit no longer exists. Such orphans were leaked by the
	// pre-fix code on every rollback / stake-invalidate / unmined-credit
	// removal of an SKA credit. Where an orphan corresponds to a
	// stake-invalidated credit (still in bucketStakeInvalidatedCredits) the
	// SKA amount is migrated to the new archive bucket so a subsequent
	// stakeValidate restores the original value.
	skaSideBucketCleanupVersion = 33

	// DBVersion is the latest version of the database that is understood by the
	// program.  Databases with recorded versions higher than this will fail to
	// open (meaning any upgrades prevent reverting to older software).
	DBVersion = skaSideBucketCleanupVersion
)

// upgrades maps between old database versions and the upgrade function to
// upgrade the database to the next version.  Note that there was never a
// version zero so upgrades[0] is nil.
var upgrades = [...]func(walletdb.ReadWriteTx, []byte, *chaincfg.Params) error{
	lastUsedAddressIndexVersion - 1:       lastUsedAddressIndexUpgrade,
	votingPreferencesVersion - 1:          votingPreferencesUpgrade,
	noEncryptedSeedVersion - 1:            noEncryptedSeedUpgrade,
	lastReturnedAddressVersion - 1:        lastReturnedAddressUpgrade,
	ticketBucketVersion - 1:               ticketBucketUpgrade,
	slip0044CoinTypeVersion - 1:           slip0044CoinTypeUpgrade,
	hasExpiryVersion - 1:                  hasExpiryUpgrade,
	hasExpiryFixedVersion - 1:             hasExpiryFixedUpgrade,
	cfVersion - 1:                         cfUpgrade,
	lastProcessedTxsBlockVersion - 1:      lastProcessedTxsBlockUpgrade,
	ticketCommitmentsVersion - 1:          ticketCommitmentsUpgrade,
	importedXpubAccountVersion - 1:        importedXpubAccountUpgrade,
	unencryptedRedeemScriptsVersion - 1:   unencryptedRedeemScriptsUpgrade,
	blockcf2Version - 1:                   blockcf2Upgrade,
	perTicketVotingPreferencesVersion - 1: perTicketVotingPreferencesUpgrade,
	accountVariablesVersion - 1:           accountVariablesUpgrade,
	unpublishedTxsVersion - 1:             unpublishedTxsUpgrade,
	tspendPolicyVersion - 1:               tspendPolicyUpgrade,
	vspBucketVersion - 1:                  vspBucketUpgrade,
	vspStatusVersion - 1:                  vspStatusUpgrade,
	tspendHashPolicyVersion - 1:           tspendHashPolicyUpgrade,
	vspHostVersion - 1:                    vspHostVersionUpgrade,
	vspTreasuryPoliciesVersion - 1:        vspTreasuryPoliciesUpgrade,
	importVotingAccountVersion - 1:        importVotingAccountUpgrade,
	birthBlockVersion - 1:                 birthBlockUpgrade,
	dualCoinVersion - 1:                   dualCoinUpgrade,
	coinTypeBucketsVersion - 1:            coinTypeBucketsUpgrade,
	consolidationAddressVersion - 1:       consolidationAddressUpgrade,
	skaBucketsVersion - 1:                 skaBucketsUpgrade,
	wireFormatV13Version - 1:              wireFormatV13Upgrade,
	multisigCoinTypeVersion - 1:           multisigCoinTypeUpgrade,
	skaSideBucketCleanupVersion - 1:       skaSideBucketCleanupUpgrade,
}

func lastUsedAddressIndexUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 1
	const newVersion = 2

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())
	addrmgrBucket := tx.ReadWriteBucket(waddrmgrBucketKey)
	addressBucket := addrmgrBucket.NestedReadBucket(addrBucketName)
	usedAddrBucket := addrmgrBucket.NestedReadBucket(usedAddrBucketName)

	addressKey := func(hash160 []byte) []byte {
		sha := sha256.Sum256(hash160)
		return sha[:]
	}

	// Assert that this function is only called on version 1 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, "lastUsedAddressIndexUpgrade inappropriately called")
	}

	masterKeyPubParams, _, err := fetchMasterKeyParams(addrmgrBucket)
	if err != nil {
		return err
	}
	var masterKeyPub snacl.SecretKey
	err = masterKeyPub.Unmarshal(masterKeyPubParams)
	if err != nil {
		return errors.E(errors.IO, errors.Errorf("unmarshal master pubkey params: %v", err))
	}
	err = masterKeyPub.DeriveKey(&publicPassphrase)
	if err != nil {
		return errors.E(errors.Passphrase, "incorrect public passphrase")
	}

	cryptoPubKeyEnc, _, err := fetchCryptoKeys(addrmgrBucket)
	if err != nil {
		return err
	}
	cryptoPubKeyCT, err := masterKeyPub.Decrypt(cryptoPubKeyEnc)
	if err != nil {
		return errors.E(errors.Crypto, errors.Errorf("decrypt public crypto key: %v", err))
	}
	cryptoPubKey := &cryptoKey{snacl.CryptoKey{}}
	copy(cryptoPubKey.CryptoKey[:], cryptoPubKeyCT)

	// Determine how many BIP0044 accounts have been created.  Each of these
	// accounts must be updated.
	lastAccount, err := fetchLastAccount(addrmgrBucket)
	if err != nil {
		return err
	}

	// Perform account updates on all BIP0044 accounts created thus far.
	for account := uint32(0); account <= lastAccount; account++ {
		// Load the old account info.
		row, err := fetchAccountInfo(addrmgrBucket, account, oldVersion)
		if err != nil {
			return err
		}

		// Use the crypto public key to decrypt the account public extended key
		// and each branch key.
		serializedKeyPub, err := cryptoPubKey.Decrypt(row.pubKeyEncrypted)
		if err != nil {
			return errors.E(errors.Crypto, errors.Errorf("decrypt extended pubkey: %v", err))
		}
		xpub, err := hdkeychain.NewKeyFromString(string(serializedKeyPub), params)
		if err != nil {
			return errors.E(errors.IO, err)
		}
		xpubExtBranch, err := xpub.Child(ExternalBranch)
		if err != nil {
			return err
		}
		xpubIntBranch, err := xpub.Child(InternalBranch)
		if err != nil {
			return err
		}

		// Determine the last used internal and external address indexes.  The
		// sentinel value ^uint32(0) means that there has been no usage at all.
		lastUsedExtIndex := ^uint32(0)
		lastUsedIntIndex := ^uint32(0)
		for child := uint32(0); child < hdkeychain.HardenedKeyStart; child++ {
			xpubChild, err := xpubExtBranch.Child(child)
			if errors.Is(err, hdkeychain.ErrInvalidChild) {
				continue
			}
			if err != nil {
				return err
			}
			// This can't error because the function always passes good input to
			// dcrutil.NewAddressPubKeyHash.  Also, while it looks like a
			// mistake to hardcode the mainnet parameters here, it doesn't make
			// any difference since only the pubkey hash is used.  (Why is there
			// no exported method to just return the serialized public key?)
			addr, _ := compat.HD2Address(xpubChild, params)
			if addressBucket.Get(addressKey(addr.Hash160()[:])) == nil {
				// No more recorded addresses for this account.
				break
			}
			if usedAddrBucket.Get(addressKey(addr.Hash160()[:])) != nil {
				lastUsedExtIndex = child
			}
		}
		for child := uint32(0); child < hdkeychain.HardenedKeyStart; child++ {
			// Same as above but search the internal branch.
			xpubChild, err := xpubIntBranch.Child(child)
			if errors.Is(err, hdkeychain.ErrInvalidChild) {
				continue
			}
			if err != nil {
				return err
			}
			addr, _ := compat.HD2Address(xpubChild, params)
			if addressBucket.Get(addressKey(addr.Hash160()[:])) == nil {
				break
			}
			if usedAddrBucket.Get(addressKey(addr.Hash160()[:])) != nil {
				lastUsedIntIndex = child
			}
		}

		// Convert account row values to the new serialization format that
		// replaces the next to use indexes with the last used indexes.
		row = bip0044AccountInfo(row.pubKeyEncrypted, row.privKeyEncrypted,
			0, 0, lastUsedExtIndex, lastUsedIntIndex, 0, 0, row.name, newVersion)
		err = putBIP0044AccountInfo(addrmgrBucket, account, row)
		if err != nil {
			return err
		}

		// Remove all data saved for address pool handling.
		addrmgrMetaBucket := addrmgrBucket.NestedReadWriteBucket(metaBucketName)
		err = addrmgrMetaBucket.Delete(accountNumberToAddrPoolKey(false, account))
		if err != nil {
			return err
		}
		err = addrmgrMetaBucket.Delete(accountNumberToAddrPoolKey(true, account))
		if err != nil {
			return err
		}
	}

	// Remove the used address tracking bucket.
	err = addrmgrBucket.DeleteNestedBucket(usedAddrBucketName)
	if err != nil {
		return errors.E(errors.IO, err)
	}

	// Write the new database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

func votingPreferencesUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 2
	const newVersion = 3

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())

	// Assert that this function is only called on version 2 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, "votingPreferencesUpgrade inappropriately called")
	}

	// This upgrade originally updated every ticket in the "wstakemgr" bucket,
	// however that bucket became redundant due to the removal of legacy
	// stakepool functionality. Therefore, that part of the upgrade has been
	// removed, and the only thing remaining is the creation of a new
	// "ticketsagendaprefs" bucket.

	// Create the top level bucket for agenda preferences.
	_, err = tx.CreateTopLevelBucket(agendaPreferences.defaultBucketKey())
	if err != nil {
		return err
	}

	// Write the new database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

func noEncryptedSeedUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 3
	const newVersion = 4

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())
	addrmgrBucket := tx.ReadWriteBucket(waddrmgrBucketKey)
	mainBucket := addrmgrBucket.NestedReadWriteBucket(mainBucketName)

	// Assert that this function is only called on version 3 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, "noEncryptedSeedUpgrade inappropriately called")
	}

	// Remove encrypted seed (or encrypted zeros).
	err = mainBucket.Delete(seedName)
	if err != nil {
		return err
	}

	// Write the new database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

func lastReturnedAddressUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 4
	const newVersion = 5

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())
	addrmgrBucket := tx.ReadWriteBucket(waddrmgrBucketKey)

	// Assert that this function is only called on version 4 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, "accountAddressCursorsUpgrade inappropriately called")
	}

	upgradeAcct := func(account uint32) error {
		// Load the old account info.
		row, err := fetchAccountInfo(addrmgrBucket, account, oldVersion)
		if err != nil {
			return err
		}

		// Convert account row values to the new serialization format that adds
		// the last returned indexes.  Assume that the last used address is also
		// the last returned address.
		row = bip0044AccountInfo(row.pubKeyEncrypted, row.privKeyEncrypted,
			0, 0, row.lastUsedExternalIndex, row.lastUsedInternalIndex,
			row.lastUsedExternalIndex, row.lastUsedInternalIndex,
			row.name, newVersion)
		return putBIP0044AccountInfo(addrmgrBucket, account, row)
	}

	// Determine how many BIP0044 accounts have been created.  Each of these
	// accounts must be updated.
	lastAccount, err := fetchLastAccount(addrmgrBucket)
	if err != nil {
		return err
	}

	// Perform account updates on all BIP0044 accounts created thus far.
	for account := uint32(0); account <= lastAccount; account++ {
		err := upgradeAcct(account)
		if err != nil {
			return err
		}
	}

	// Perform upgrade on the imported account, which is also using the BIP0044
	// row serialization.  The last used and last returned indexes are not used
	// by the imported account but the row must be upgraded regardless to avoid
	// deserialization errors due to the row value length checks.
	err = upgradeAcct(ImportedAddrAccount)
	if err != nil {
		return err
	}

	// Write the new database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

func ticketBucketUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 5
	const newVersion = 6

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())
	txmgrBucket := tx.ReadWriteBucket(wtxmgrBucketKey)

	// Assert that this function is only called on version 5 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, "ticketBucketUpgrade inappropriately called")
	}

	// Create the tickets bucket.
	_, err = txmgrBucket.CreateBucket(bucketTickets)
	if err != nil {
		return err
	}

	// Add an entry in the tickets bucket for every mined and unmined ticket
	// purchase transaction.  Use -1 as the selected height since this value is
	// unknown at this time and the field is not yet being used.
	ticketHashes := make(map[chainhash.Hash]struct{})
	c := txmgrBucket.NestedReadBucket(bucketTxRecords).ReadCursor()
	for k, v := c.First(); v != nil; k, v = c.Next() {
		var hash chainhash.Hash
		err := readRawTxRecordHash(k, &hash)
		if err != nil {
			c.Close()
			return err
		}
		var rec TxRecord
		err = readRawTxRecordLegacy(&hash, v, &rec)
		if err != nil {
			c.Close()
			return err
		}
		if stake.IsSStx(&rec.MsgTx) {
			ticketHashes[hash] = struct{}{}
		}
	}
	c.Close()

	c = txmgrBucket.NestedReadBucket(bucketUnmined).ReadCursor()
	for k, v := c.First(); v != nil; k, v = c.Next() {
		var hash chainhash.Hash
		err := readRawUnminedHash(k, &hash)
		if err != nil {
			c.Close()
			return err
		}
		var rec TxRecord
		err = readRawTxRecordLegacy(&hash, v, &rec)
		if err != nil {
			c.Close()
			return err
		}
		if stake.IsSStx(&rec.MsgTx) {
			ticketHashes[hash] = struct{}{}
		}
	}
	c.Close()
	for ticketHash := range ticketHashes {
		err := putTicketRecord(txmgrBucket, &ticketHash, -1)
		if err != nil {
			return err
		}
	}

	// Remove previous stakebase input from the unmined inputs bucket, if any
	// was recorded.
	stakebaseOutpoint := canonicalOutPoint(&chainhash.Hash{}, ^uint32(0))
	err = txmgrBucket.NestedReadWriteBucket(bucketUnminedInputs).Delete(stakebaseOutpoint)
	if err != nil {
		return err
	}

	// Write the new database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

func slip0044CoinTypeUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 6
	const newVersion = 7

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())

	// Assert that this function is only called on version 6 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, "slip0044CoinTypeUpgrade inappropriately called")
	}

	// Write the new database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

func hasExpiryUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 7
	const newVersion = 8
	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())
	txmgrBucket := tx.ReadWriteBucket(wtxmgrBucketKey)

	// Assert that this function is only called on version 7 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, "hasExpiryUpgrade inappropriately called")
	}

	// Iterate through all mined credits
	creditsBucket := txmgrBucket.NestedReadWriteBucket(bucketCredits)
	cursor := creditsBucket.ReadWriteCursor()
	creditsKV := map[string][]byte{}
	for k, v := cursor.First(); v != nil; k, v = cursor.Next() {
		hash := extractRawCreditTxHash(k)
		block, err := fetchBlockRecord(txmgrBucket, extractRawCreditHeight(k))
		if err != nil {
			cursor.Close()
			return err
		}

		_, recV := existsTxRecord(txmgrBucket, &hash, &block.Block)
		record := &TxRecord{}
		err = readRawTxRecordLegacy(&hash, recV, record)
		if err != nil {
			cursor.Close()
			return err
		}

		// Only save credits that need their hasExpiry flag updated
		if record.MsgTx.Expiry != wire.NoExpiryValue {
			vCpy := make([]byte, len(v))
			copy(vCpy, v)

			vCpy[8] |= 1 << 4
			creditsKV[string(k)] = vCpy
		}
	}
	cursor.Close()

	for k, v := range creditsKV {
		err = creditsBucket.Put([]byte(k), v)
		if err != nil {
			return err
		}
	}

	// Iterate through all unmined credits
	unminedCreditsBucket := txmgrBucket.NestedReadWriteBucket(bucketUnminedCredits)
	unminedCursor := unminedCreditsBucket.ReadWriteCursor()
	unminedCreditsKV := map[string][]byte{}
	for k, v := unminedCursor.First(); v != nil; k, v = unminedCursor.Next() {
		hash, err := chainhash.NewHash(extractRawUnminedCreditTxHash(k))
		if err != nil {
			unminedCursor.Close()
			return err
		}

		recV := existsRawUnmined(txmgrBucket, hash[:])
		record := &TxRecord{}
		err = readRawTxRecordLegacy(hash, recV, record)
		if err != nil {
			unminedCursor.Close()
			return err
		}

		// Only save credits that need their hasExpiry flag updated
		if record.MsgTx.Expiry != wire.NoExpiryValue {
			vCpy := make([]byte, len(v))
			copy(vCpy, v)

			vCpy[8] |= 1 << 4
			unminedCreditsKV[string(k)] = vCpy
		}
	}
	unminedCursor.Close()

	for k, v := range unminedCreditsKV {
		err = unminedCreditsBucket.Put([]byte(k), v)
		if err != nil {
			return err
		}
	}

	// Write the new database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

func hasExpiryFixedUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 8
	const newVersion = 9
	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())
	txmgrBucket := tx.ReadWriteBucket(wtxmgrBucketKey)

	// Assert this function is only called on version 8 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, "hasExpiryFixedUpgrade inappropriately called")
	}

	// Iterate through all mined credits
	creditsBucket := txmgrBucket.NestedReadWriteBucket(bucketCredits)
	cursor := creditsBucket.ReadCursor()
	creditsKV := map[string][]byte{}
	for k, v := cursor.First(); v != nil; k, v = cursor.Next() {
		hash := extractRawCreditTxHash(k)
		block, err := fetchBlockRecord(txmgrBucket, extractRawCreditHeight(k))
		if err != nil {
			cursor.Close()
			return err
		}

		_, recV := existsTxRecord(txmgrBucket, &hash, &block.Block)
		record := &TxRecord{}
		err = readRawTxRecordLegacy(&hash, recV, record)
		if err != nil {
			cursor.Close()
			return err
		}

		// Only save credits that need their hasExpiry flag updated
		if record.MsgTx.Expiry != wire.NoExpiryValue {
			vCpy := make([]byte, len(v))
			copy(vCpy, v)

			vCpy[8] &^= 1 << 4 // Clear bad hasExpiry/OP_SSTXCHANGE flag
			vCpy[8] |= 1 << 6  // Set correct hasExpiry flag
			// Reset OP_SSTXCHANGE flag if this is a ticket purchase
			// OP_SSTXCHANGE output.
			out := record.MsgTx.TxOut[extractRawCreditIndex(k)]
			if stake.IsSStx(&record.MsgTx) &&
				(stdscript.IsStakeChangePubKeyHashScript(out.Version, out.PkScript) ||
					stdscript.IsStakeChangeScriptHashScript(out.Version, out.PkScript)) {
				vCpy[8] |= 1 << 4
			}

			creditsKV[string(k)] = vCpy
		}
	}
	cursor.Close()

	for k, v := range creditsKV {
		err = creditsBucket.Put([]byte(k), v)
		if err != nil {
			return err
		}
	}

	// Iterate through all unmined credits
	unminedCreditsBucket := txmgrBucket.NestedReadWriteBucket(bucketUnminedCredits)
	unminedCursor := unminedCreditsBucket.ReadCursor()
	unminedCreditsKV := map[string][]byte{}
	for k, v := unminedCursor.First(); v != nil; k, v = unminedCursor.Next() {
		hash, err := chainhash.NewHash(extractRawUnminedCreditTxHash(k))
		if err != nil {
			unminedCursor.Close()
			return err
		}

		recV := existsRawUnmined(txmgrBucket, hash[:])
		record := &TxRecord{}
		err = readRawTxRecordLegacy(hash, recV, record)
		if err != nil {
			unminedCursor.Close()
			return err
		}

		// Only save credits that need their hasExpiry flag updated
		if record.MsgTx.Expiry != wire.NoExpiryValue {
			vCpy := make([]byte, len(v))
			copy(vCpy, v)

			vCpy[8] &^= 1 << 4 // Clear bad hasExpiry/OP_SSTXCHANGE flag
			vCpy[8] |= 1 << 6  // Set correct hasExpiry flag
			// Reset OP_SSTXCHANGE flag if this is a ticket purchase
			// OP_SSTXCHANGE output.
			idx, err := fetchRawUnminedCreditIndex(k)
			if err != nil {
				unminedCursor.Close()
				return err
			}
			out := record.MsgTx.TxOut[idx]
			if stake.IsSStx(&record.MsgTx) &&
				(stdscript.IsStakeChangePubKeyHashScript(out.Version, out.PkScript) ||
					stdscript.IsStakeChangeScriptHashScript(out.Version, out.PkScript)) {
				vCpy[8] |= 1 << 4
			}

			unminedCreditsKV[string(k)] = vCpy
		}
	}
	unminedCursor.Close()

	for k, v := range unminedCreditsKV {
		err = unminedCreditsBucket.Put([]byte(k), v)
		if err != nil {
			return err
		}
	}

	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

func cfUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 9
	const newVersion = 10

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())
	txmgrBucket := tx.ReadWriteBucket(wtxmgrBucketKey)

	// Assert that this function is only called on version 9 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, "cfUpgrade inappropriately called")
	}

	err = txmgrBucket.Put(rootHaveCFilters, []byte{0})
	if err != nil {
		return errors.E(errors.IO, err)
	}
	_, err = txmgrBucket.CreateBucket(bucketCFilters)
	if err != nil {
		return errors.E(errors.IO, err)
	}

	// Record cfilter for genesis block.
	//
	// Note: This used to record the actual version 1 cfilter for the
	// genesis block, but this was changed as packages were updated.
	// Version 1 cfilters can no longer be created using the latest major
	// version of the gcs module.  Plus, version 1 cfilters are removed in a
	// later database upgrade, because only version 2 cfilters are used now.
	// So instead, this upgrade path has been modified to record nil bytes
	// for this genesis block v1 cfilter.
	err = putRawCFilter(txmgrBucket, params.GenesisHash[:], nil)
	if err != nil {
		return errors.E(errors.IO, err)
	}

	// Record all cfilters as saved when only the genesis block is saved.
	var tipHash chainhash.Hash
	copy(tipHash[:], txmgrBucket.Get(rootTipBlock))
	if tipHash == params.GenesisHash {
		err = txmgrBucket.Put(rootHaveCFilters, []byte{1})
		if err != nil {
			return errors.E(errors.IO, err)
		}
	}

	// Write the new database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

func lastProcessedTxsBlockUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 10
	const newVersion = 11

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())
	txmgrBucket := tx.ReadWriteBucket(wtxmgrBucketKey)

	// Assert that this function is only called on version 10 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, "lastProcessedTxsBlockUpgrade inappropriately called")
	}

	// Record the current tip block as the last block since genesis with
	// processed transaction.
	err = txmgrBucket.Put(rootLastTxsBlock, txmgrBucket.Get(rootTipBlock))
	if err != nil {
		return errors.E(errors.IO, err)
	}

	// Write the new database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

func ticketCommitmentsUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 11
	const newVersion = 12

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())
	txmgrBucket := tx.ReadWriteBucket(wtxmgrBucketKey)
	addrmgrBucket := tx.ReadWriteBucket(waddrmgrBucketKey)

	// Assert that this function is only called on version 11 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, "ticketCommitmentsUpgrade inappropriately called")
	}

	_, err = txmgrBucket.CreateBucket(bucketTicketCommitments)
	if err != nil {
		return errors.E(errors.IO, err)
	}

	_, err = txmgrBucket.CreateBucket(bucketTicketCommitmentsUsp)
	if err != nil {
		return errors.E(errors.IO, err)
	}

	// Helper function to handle the details of a single (mined or unmined)
	// transaction.
	handleTxRecCommitments := func(txrec *TxRecord, unmined bool) error {
		for i, txo := range txrec.MsgTx.TxOut {
			if txrec.TxType == stake.TxTypeSStx {
				if i%2 != 1 {
					// Ignore non ticket commitment outputs.
					continue
				}

				// Decode the address stored in the commitment.
				addr, err := stake.AddrFromSStxPkScrCommitment(txo.PkScript, params)
				if err != nil {
					return errors.E(errors.IO, err)
				}

				id, err := addressID(normalizeAddress(addr))
				if err != nil {
					return errors.E(errors.Bug, err)
				}
				acct, err := fetchAddrAccount(addrmgrBucket, id)
				if err != nil && errors.Is(err, errors.NotExist) {
					// If this address does not have an account associated
					// with it, it means it's not owned by the wallet.
					continue
				} else if err != nil {
					return errors.E(errors.IO, err)
				}

				// Decode the amount stored in the commitment.
				amount, err := stake.AmountFromSStxPkScrCommitment(txo.PkScript)
				if err != nil {
					return errors.E(errors.IO, err)
				}

				log.Debugf("Adding ticket commitment %s:%d (%s) for account %d",
					txrec.Hash, i, amount, acct)

				// Store both the ticket commitment info and an entry in the
				// unspent index.
				k := keyTicketCommitment(txrec.Hash, uint32(i))
				v := valueTicketCommitment(amount, acct)
				err = putRawTicketCommitment(txmgrBucket, k, v)
				if err != nil {
					return errors.E(errors.IO, err)
				}

				v = valueUnspentTicketCommitment(false)
				err = putRawUnspentTicketCommitment(txmgrBucket, k, v)
				if err != nil {
					return errors.E(errors.IO, err)
				}
			} else if (txrec.TxType == stake.TxTypeSSGen) || (txrec.TxType == stake.TxTypeSSRtx) {
				// txoIdx is the original output index of the commitment, given
				// that "i" is an output index on the spender transaction.
				txoIdx := uint32(i*2 + 1)

				// ticketHashIdx is the index of the input on the spender
				// transaction (txrec) where the original ticket hash can be
				// found.
				ticketHashIdx := uint32(0)
				if txrec.TxType == stake.TxTypeSSGen {
					if i < 2 {
						// Ignore previous block hash and vote bits outputs.
						continue
					}

					// To find the original output index on votes, we skip the
					// first two outputs (previous block and vote bits) and to
					// find the original input index we skip the first input
					// (stakebase).
					txoIdx = uint32((i-2)*2 + 1)
					ticketHashIdx = 1
				}

				ticketHash := txrec.MsgTx.TxIn[ticketHashIdx].PreviousOutPoint.Hash

				k := keyTicketCommitment(ticketHash, txoIdx)
				v := existsRawTicketCommitment(txmgrBucket, k)
				if v == nil {
					// Ignore commitments we were not originally tracking.
					continue
				}

				if unmined {
					// An unmined vote/revocation only marks the ticket
					// commitment as unminedSpent.
					log.Debugf("Marking ticket commitment %s:%d unmined spent",
						ticketHash, txoIdx)

					v = valueUnspentTicketCommitment(true)
					err = putRawUnspentTicketCommitment(txmgrBucket, k, v)
				} else {
					// A mined vote/revocation removes the entry from the
					// unspent ticket commitment index.
					log.Debugf("Removing unspent ticket commitment %s:%d",
						ticketHash, txoIdx)

					err = deleteRawUnspentTicketCommitment(txmgrBucket, k)
				}
				if err != nil {
					return errors.E(errors.IO, err)
				}
			}
		}

		return nil
	}

	// Rescan the database for stake transactions, creating the commitments when
	// a ticket is found and deleting it when a vote/revocation is found.
	it := makeReadBlockIterator(txmgrBucket, 0)
	var txrec TxRecord
	for it.next() {
		for _, txh := range it.elem.transactions {
			_, v := latestTxRecord(txmgrBucket, txh[:])
			err = readRawTxRecordLegacy(&txh, v, &txrec)
			if err != nil {
				return errors.E(errors.IO, err)
			}

			err = handleTxRecCommitments(&txrec, false)
			if err != nil {
				return err
			}
		}
	}

	// Rescan unmined transactions for tickets and votes.
	err = txmgrBucket.NestedReadBucket(bucketUnmined).ForEach(func(uk, uv []byte) error {
		var txHash chainhash.Hash
		var rec TxRecord

		err := readRawUnminedHash(uk, &txHash)
		if err != nil {
			return err
		}

		err = readRawTxRecordLegacy(&txHash, uv, &rec)
		if err != nil {
			return err
		}

		return handleTxRecCommitments(&rec, true)
	})
	if err != nil {
		return errors.E(errors.IO, err)
	}

	log.Debug("Ticket commitments db upgrade done")

	// Write the new database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

func importedXpubAccountUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 12
	const newVersion = 13

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())

	// Assert that this function is only called on version 12 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, "importedXpubAccountUpgrade inappropriately called")
	}

	// Write the new database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

func unencryptedRedeemScriptsUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 13
	const newVersion = 14

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())
	txmgrBucket := tx.ReadWriteBucket(wtxmgrBucketKey)
	addrmgrBucket := tx.ReadWriteBucket(waddrmgrBucketKey)

	// Assert that this function is only called on version 13 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, "unencryptedRedeemScriptsUpgrade inappropriately called")
	}

	// Open the encrypted public data crypto key.
	masterKeyPubParams, _, err := fetchMasterKeyParams(addrmgrBucket)
	if err != nil {
		return err
	}
	var masterKeyPub snacl.SecretKey
	err = masterKeyPub.Unmarshal(masterKeyPubParams)
	if err != nil {
		return errors.E(errors.IO, errors.Errorf("unmarshal master pubkey params: %v", err))
	}
	err = masterKeyPub.DeriveKey(&publicPassphrase)
	if err != nil {
		return errors.E(errors.Passphrase, "incorrect public passphrase")
	}
	cryptoPubKeyEnc, _, err := fetchCryptoKeys(addrmgrBucket)
	if err != nil {
		return err
	}
	cryptoPubKeyCT, err := masterKeyPub.Decrypt(cryptoPubKeyEnc)
	if err != nil {
		return errors.E(errors.Crypto, errors.Errorf("decrypt public crypto key: %v", err))
	}
	cryptoPubKey := &cryptoKey{snacl.CryptoKey{}}
	copy(cryptoPubKey.CryptoKey[:], cryptoPubKeyCT)

	// Read all unencrypted redeem scripts saved in the transaction store
	// into memory.
	scripts := make(map[[20]byte][]byte)
	scriptsBucket := txmgrBucket.NestedReadWriteBucket(bucketScripts)
	cursor := scriptsBucket.ReadCursor()
	for _, v := cursor.First(); v != nil; _, v = cursor.Next() {
		script := append(v[:0:0], v...) // copy
		var hash [20]byte
		copy(hash[:], dcrutil.Hash160(script))
		scripts[hash] = script
	}
	cursor.Close()

	// Read all script hashes of recorded managed p2sh addresses.
	// These are only recorded by the "imported" account.
	p2shRows := make(map[[20]byte]*dbScriptAddressRow)
	importedAddrsBucket := addrmgrBucket.NestedReadBucket(addrAcctIdxBucketName).
		NestedReadBucket(uint32ToBytes(ImportedAddrAccount))
	cursor = nil
	var ck, cv []byte
	if importedAddrsBucket != nil {
		cursor = importedAddrsBucket.ReadCursor()
		ck, cv = cursor.First()
	}
	for ; ck != nil; ck, cv = cursor.Next() {
		if cv == nil {
			continue // skip nested buckets
		}
		row, err := fetchAddressByHash(addrmgrBucket, ck)
		if err != nil {
			return errors.E(errors.IO, err)
		}
		r, ok := row.(*dbScriptAddressRow)
		if !ok {
			continue
		}
		// Decrypt HASH160 using the public data encryption key.
		decryptedHash160, err := cryptoPubKey.Decrypt(r.encryptedHash)
		if err != nil {
			return err
		}
		if len(decryptedHash160) != 20 {
			return errors.E(errors.IO, "decrypted hash160 is not 20 bytes")
		}
		var hash160 [20]byte
		copy(hash160[:], decryptedHash160)
		p2shRows[hash160] = r
	}
	if cursor != nil {
		cursor.Close()
	}

	// For every encrypted address manager script, ensure that an
	// unencrypted version of the script is recorded.  Rewrite each p2sh
	// address row with the cleartext script.
	for k, r := range p2shRows {
		script, ok := scripts[k]
		if !ok {
			err := errors.Errorf("missing cleartext script for p2sh hash160 %x", k[:])
			return errors.E(errors.IO, err)
		}
		r.script = script
		r.rawData = serializeScriptAddress(r.encryptedHash, r.script)
		err := putAddress(addrmgrBucket, k[:], &r.dbAddressRow)
		if err != nil {
			return err
		}
		err = scriptsBucket.Delete(k[:])
		if err != nil {
			return err
		}
		delete(scripts, k)
	}

	// For each remaining imported script that only appeared in the
	// transaction store buckets, add an imported p2sh address to the
	// address manager.
	for k, script := range scripts {
		encryptedHash, err := cryptoPubKey.Encrypt(k[:])
		if err != nil {
			return err
		}
		err = putScriptAddress(addrmgrBucket, k[:], ImportedAddrAccount,
			encryptedHash, script)
		if err != nil {
			return err
		}
		err = scriptsBucket.Delete(k[:])
		if err != nil {
			return err
		}
	}

	// Remove the transaction store's nested scripts bucket.
	err = txmgrBucket.DeleteNestedBucket(bucketScripts)
	if err != nil {
		return err
	}

	// Remove the address manager's sealed symmetric key for script encryption.
	addrmgrMainBucket := addrmgrBucket.NestedReadWriteBucket(mainBucketName)
	err = addrmgrMainBucket.Delete(cryptoScriptKeyName)
	if err != nil && !errors.Is(err, errors.NotExist) {
		return err
	}

	// Write the new database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

// genesisPrevScripter fulfills the blockcf2.PrevScripter interface but only
// for the genesis block which doesn't have any previous scripts.
type genesisPrevScripter struct{}

func (genesisPrevScripter) PrevScript(*wire.OutPoint) (uint16, []byte, bool) {
	return 0, nil, false
}

func blockcf2Upgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 14
	const newVersion = 15

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())

	// Assert that this function is only called on version 14 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, "blockcf2Upgrade inappropriately called")
	}

	txmgrBucket := tx.ReadWriteBucket(wtxmgrBucketKey)

	// Drop all existing blockcf filters by deleting then recreating the
	// cfilter bucket.
	err = txmgrBucket.DeleteNestedBucket(bucketCFilters)
	if err != nil {
		return errors.E(errors.IO, err)
	}
	_, err = txmgrBucket.CreateBucket(bucketCFilters)
	if err != nil {
		return errors.E(errors.IO, err)
	}

	// Reset the rootHaveCFilters flag to false so that cfilters will have
	// to be downloaded again.
	err = txmgrBucket.Put(rootHaveCFilters, []byte{0})
	if err != nil {
		return errors.E(errors.IO, err)
	}
	// Record cfilter for genesis block.
	f, err := blockcf2.Regular(params.GenesisBlock, genesisPrevScripter{})
	if err != nil {
		return err
	}
	genesisBcfKey := blockcf2.Key(&params.GenesisBlock.Header.MerkleRoot)
	err = putRawCFilter(txmgrBucket, params.GenesisHash[:],
		valueRawCFilter2(genesisBcfKey, f.Bytes()))
	if err != nil {
		return errors.E(errors.IO, err)
	}

	// Record all cfilters as saved when only the genesis block is saved.
	var tipHash chainhash.Hash
	copy(tipHash[:], txmgrBucket.Get(rootTipBlock))
	if tipHash == params.GenesisHash {
		err = txmgrBucket.Put(rootHaveCFilters, []byte{1})
		if err != nil {
			return errors.E(errors.IO, err)
		}
	}

	// Write the new database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

func perTicketVotingPreferencesUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 15
	const newVersion = 16

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())

	// Assert that this function is only called on version 15 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, "perTicketVotingPreferencesUpgrade inappropriately called")
	}

	// Create the top level bucket for per-ticket voting preferences.
	_, err = tx.CreateTopLevelBucket(agendaPreferences.ticketsBucketKey())
	if err != nil {
		return err
	}

	// Write the new database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

func accountVariablesUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 16
	const newVersion = 17

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())

	// Assert that this function is only called on version 16 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, "accountVariablesUpgrade inappropriately called")
	}

	addrmgrBucket := tx.ReadWriteBucket(waddrmgrBucketKey)

	// For each previous BIP0044 account, create the variables bucket for it
	// and rewrite both the row and all variables.
	_, err = addrmgrBucket.CreateBucket(acctVarsBucketName)
	if err != nil {
		return errors.E(errors.IO, err)
	}
	newAcct := new(dbBIP0044Account)
	err = forEachAccount(addrmgrBucket, func(account uint32) error {
		a, err := fetchDBAccount(addrmgrBucket, account, oldVersion)
		if err != nil {
			return err
		}
		switch a := a.(type) {
		case *dbBIP0044AccountRow:
			newAcct.dbAccountRow = a.dbAccountRow
			newAcct.dbAccountRow.acctType = actBIP0044
			newAcct.pubKeyEncrypted = a.pubKeyEncrypted
			newAcct.privKeyEncrypted = a.privKeyEncrypted
			newAcct.lastUsedExternalIndex = a.lastUsedExternalIndex
			newAcct.lastUsedInternalIndex = a.lastUsedInternalIndex
			newAcct.lastReturnedExternalIndex = a.lastReturnedExternalIndex
			newAcct.lastReturnedInternalIndex = a.lastReturnedInternalIndex
			newAcct.name = a.name
			newAcct.dbAccountRow.rawData = newAcct.serializeRow()
		default:
			return nil
		}

		return putNewBIP0044Account(addrmgrBucket, account, newAcct)
	})
	if err != nil {
		return errors.E(errors.IO, err)
	}

	// Write the new database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

func unpublishedTxsUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 17
	const newVersion = 18

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())

	// Assert that this function is only called on version 17 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, "unpublishedTxsUpgrade inappropriately called")
	}

	txmgrNs := tx.ReadWriteBucket(wtxmgrBucketKey)
	_, err = txmgrNs.CreateBucket(bucketUnpublished)
	if err != nil {
		return err
	}

	// Write the new database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

func tspendPolicyUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 18
	const newVersion = 19

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())

	// Assert that this function is only called on version 18 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, "tspendPolicyUpgrade inappropriately called")
	}

	_, err = tx.CreateTopLevelBucket(treasuryPolicyBucketKey)
	if err != nil {
		return errors.E(errors.IO, err)
	}

	// Write the new database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

func vspBucketUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 19
	const newVersion = 20

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())
	// Assert that this function is only called on version 19 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, "vspBucketUpgrade inappropriately called")
	}

	// Create the vsp tickets bucket.
	_, err = tx.CreateTopLevelBucket(vspBucketKey)
	if err != nil {
		return err
	}

	// Write the new database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

func vspStatusUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 20
	const newVersion = 21

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())
	// Assert that this function is only called on version 20 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, "vspStatusUpgrade inappropriately called")
	}

	bucket := tx.ReadWriteBucket(vspBucketKey)
	tix := make(map[string][]byte)
	cursor := bucket.ReadCursor()
	statusBytes := make([]byte, 4)
	byteOrder.PutUint32(statusBytes, uint32(VSPFeeProcessErrored))
	for k, v := cursor.First(); v != nil; k, v = cursor.Next() {
		tix[string(k)] = append(v[:len(v):len(v)], statusBytes...)
	}
	cursor.Close()
	for k, v := range tix {
		err := bucket.Put([]byte(k), v)
		if err != nil {
			return errors.E(errors.IO, err)
		}
	}

	// Write the new database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

func tspendHashPolicyUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 21
	const newVersion = 22

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())

	// Assert that this function is only called on version 21 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, "tspendHashPolicyUpgrade inappropriately called")
	}

	_, err = tx.CreateTopLevelBucket(tspendPolicyBucketKey)
	if err != nil {
		return errors.E(errors.IO, err)
	}

	// Write the new database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

func vspHostVersionUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 22
	const newVersion = 23

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())
	// Assert that this function is only called on version 20 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, "vspStatusUpgrade inappropriately called")
	}

	// Create new vsp host bucket
	_, err = tx.CreateTopLevelBucket(vspHostBucketKey)
	if err != nil {
		return errors.E(errors.IO, err)
	}
	// Create new vsp pubkey bucket
	_, err = tx.CreateTopLevelBucket(vspPubKeyBucketKey)
	if err != nil {
		return errors.E(errors.IO, err)
	}

	// Create first entry into vsp host bucket of an 0 length host, keyed
	// at 0.
	bucket := tx.ReadWriteBucket(vspHostBucketKey)
	k := make([]byte, 4)
	byteOrder.PutUint32(k, 0)
	buf := make([]byte, 4)
	byteOrder.PutUint32(buf[0:4], uint32(0))
	err = bucket.Put(k, make([]byte, 4))
	if err != nil {
		return errors.E(errors.IO, err)
	}

	// Enter current index of vsp host as 0
	vspIndexBytes := make([]byte, 4)
	byteOrder.PutUint32(vspIndexBytes, 0)
	err = bucket.Put(rootVSPHostIndex, vspIndexBytes)
	if err != nil {
		return errors.E(errors.IO, err)
	}

	// Set all existing tickets in the vspBucketKey to have the vsp host entry
	// of 0.  These will be noticed on the next time the process managed tickets
	// is completed and all entried will be updated to their proper values
	// once they have been confirmed at a particular VSP.
	ticketBucket := tx.ReadWriteBucket(vspBucketKey)
	tix := make(map[string][]byte)
	cursor := ticketBucket.ReadCursor()
	vspHostZero := make([]byte, 4)
	byteOrder.PutUint32(vspHostZero, uint32(0))
	for k, v := cursor.First(); v != nil; k, v = cursor.Next() {
		tix[string(k)] = append(v[:len(v):len(v)], vspHostZero...)
	}
	cursor.Close()
	for k, v := range tix {
		err := ticketBucket.Put([]byte(k), v)
		if err != nil {
			return errors.E(errors.IO, err)
		}
	}
	// Write the new database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

func vspTreasuryPoliciesUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 23
	const newVersion = 24

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())

	// Assert that this function is only called on version 22 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, "vspTreasuryPoliciesUpgrade inappropriately called")
	}

	_, err = tx.CreateTopLevelBucket(vspTreasuryPolicyBucketKey)
	if err != nil {
		return errors.E(errors.IO, err)
	}
	_, err = tx.CreateTopLevelBucket(vspTspendPolicyBucketKey)
	if err != nil {
		return errors.E(errors.IO, err)
	}

	// Write the new database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

func importVotingAccountUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 24
	const newVersion = 25

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())

	// Assert that this function is only called on version 24 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, "importVotingAccountUpgrade inappropriately called")
	}

	// Write the new database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

func birthBlockUpgrade(tx walletdb.ReadWriteTx, _ []byte, params *chaincfg.Params) error {
	const oldVersion = 25
	const newVersion = 26

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())

	// Assert that this function is only called on version 25 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, "birthBlockUpgrade inappropriately called")
	}

	bs := &BirthdayState{
		Hash: params.GenesisHash,
	}
	if err := SetBirthState(tx, bs); err != nil {
		return err
	}

	// Write the new database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

// dualCoinUpgrade performs an upgrade from version 26 to 27. This upgrade adds
// support for dual-coin system by extending unmined credits to store coin type.
// It also migrates all existing transaction records from the legacy wire format
// (without CoinType) to the new format (with CoinType=VAR for all outputs).
func dualCoinUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 26
	const newVersion = 27

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())
	if metadataBucket == nil {
		return errors.E(errors.IO, "missing metadata bucket")
	}

	// Assert this function is only called on version 26 databases. Without
	// this guard a dispatcher refactor or direct caller would silently
	// rewrite a different version's data through the V12 wire-format reader.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, errors.Errorf("dualCoinUpgrade inappropriately called"))
	}

	txmgrBucket := tx.ReadWriteBucket(wtxmgrBucketKey)
	if txmgrBucket == nil {
		return errors.E(errors.IO, "missing transaction manager bucket")
	}

	// Migrate mined transaction records from legacy to new wire format.
	// This migration is only needed for wallets upgrading from Decred (pre-v27).
	// New Monetarium wallets start at v29+ and skip this upgrade entirely.
	// The migration reads transactions in legacy format (without CoinType) and
	// rewrites them with CoinType=VAR for all outputs.
	txRecordsBucket := txmgrBucket.NestedReadWriteBucket(bucketTxRecords)
	if txRecordsBucket != nil {
		migratedRecords := make(map[string][]byte)
		err := txRecordsBucket.ForEach(func(k, v []byte) error {
			// Read using legacy format (without CoinType)
			var hash chainhash.Hash
			err := readRawTxRecordHash(k, &hash)
			if err != nil {
				return err
			}
			var rec TxRecord
			err = readRawTxRecordLegacy(&hash, v, &rec)
			if err != nil {
				return err
			}

			// Set CoinType to VAR for all outputs (legacy transactions are all VAR)
			for i := range rec.MsgTx.TxOut {
				rec.MsgTx.TxOut[i].CoinType = cointype.CoinTypeVAR
			}

			// Re-serialize in new format
			newV, err := valueTxRecord(&rec)
			if err != nil {
				return err
			}
			migratedRecords[string(k)] = newV
			return nil
		})
		if err != nil {
			return errors.E(errors.IO, err)
		}

		// Write migrated records
		for k, v := range migratedRecords {
			if err := txRecordsBucket.Put([]byte(k), v); err != nil {
				return errors.E(errors.IO, err)
			}
		}
	}

	// Migrate unmined transaction records from legacy to new wire format
	unminedBucket := txmgrBucket.NestedReadWriteBucket(bucketUnmined)
	if unminedBucket != nil {
		migratedUnmined := make(map[string][]byte)
		err := unminedBucket.ForEach(func(k, v []byte) error {
			// Read using legacy format
			var hash chainhash.Hash
			err := readRawUnminedHash(k, &hash)
			if err != nil {
				return err
			}
			var rec TxRecord
			err = readRawTxRecordLegacy(&hash, v, &rec)
			if err != nil {
				return err
			}

			// Set CoinType to VAR for all outputs
			for i := range rec.MsgTx.TxOut {
				rec.MsgTx.TxOut[i].CoinType = cointype.CoinTypeVAR
			}

			// Re-serialize in new format
			newV, err := valueTxRecord(&rec)
			if err != nil {
				return err
			}
			migratedUnmined[string(k)] = newV
			return nil
		})
		if err != nil {
			return errors.E(errors.IO, err)
		}

		// Write migrated records
		for k, v := range migratedUnmined {
			if err := unminedBucket.Put([]byte(k), v); err != nil {
				return errors.E(errors.IO, err)
			}
		}
	}

	// Update the database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

// coinTypeBucketsUpgrade performs an upgrade from version 27 to 28. This upgrade creates
// separate buckets for each coin type to enable efficient coin-type-specific
// UTXO queries without runtime filtering.
func coinTypeBucketsUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 27
	const newVersion = 28

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())
	if metadataBucket == nil {
		return errors.E(errors.IO, "missing metadata bucket")
	}

	// Assert this function is only called on version 27 databases.
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, errors.Errorf("coinTypeBucketsUpgrade inappropriately called"))
	}

	txmgrBucket := tx.ReadWriteBucket(wtxmgrBucketKey)
	if txmgrBucket == nil {
		return errors.E(errors.IO, "missing transaction manager bucket")
	}

	// Create buckets for VAR (always exists)
	_, err = txmgrBucket.CreateBucketIfNotExists(bucketUnspentForCoinType(cointype.CoinTypeVAR))
	if err != nil {
		return errors.E(errors.IO, err)
	}
	_, err = txmgrBucket.CreateBucketIfNotExists(bucketUnminedCreditsForCoinType(cointype.CoinTypeVAR))
	if err != nil {
		return errors.E(errors.IO, err)
	}

	// Migrate existing unspent outputs to coin-type specific buckets
	unspentBucket := txmgrBucket.NestedReadBucket(bucketUnspent)
	creditsBucket := txmgrBucket.NestedReadBucket(bucketCredits)

	if unspentBucket != nil && creditsBucket != nil {
		// Migrate mined unspent outputs
		err = unspentBucket.ForEach(func(k, v []byte) error {
			// Construct credit key to fetch coin type
			if len(k) < 36 || len(v) < 36 {
				return nil // Skip invalid entries
			}

			credKey := make([]byte, 72)
			copy(credKey, k[:32])
			copy(credKey[32:68], v)
			copy(credKey[68:72], k[32:36])

			cVal := creditsBucket.Get(credKey)
			if cVal == nil {
				return nil // Skip if credit not found
			}

			coinType := fetchRawCreditCoinType(cVal)

			// Create bucket for this coin type if it doesn't exist
			bucketName := bucketUnspentForCoinType(coinType)
			targetBucket, err := txmgrBucket.CreateBucketIfNotExists(bucketName)
			if err != nil {
				return errors.E(errors.IO, err)
			}

			// Copy the UTXO to the coin-type specific bucket
			return targetBucket.Put(k, v)
		})
		if err != nil {
			return err
		}
	}

	// Migrate unmined credits
	unminedBucket := txmgrBucket.NestedReadBucket(bucketUnminedCredits)
	if unminedBucket != nil {
		err = unminedBucket.ForEach(func(k, v []byte) error {
			coinType := fetchRawUnminedCreditCoinType(v)

			// Create bucket for this coin type if it doesn't exist
			bucketName := bucketUnminedCreditsForCoinType(coinType)
			targetBucket, err := txmgrBucket.CreateBucketIfNotExists(bucketName)
			if err != nil {
				return errors.E(errors.IO, err)
			}

			// Copy the unmined credit to the coin-type specific bucket
			return targetBucket.Put(k, v)
		})
		if err != nil {
			return err
		}
	}

	// Update the database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

// Upgrade checks whether the any upgrades are necessary before the database is
// ready for application usage.  If any are, they are performed.
func Upgrade(ctx context.Context, db walletdb.DB, publicPassphrase []byte, params *chaincfg.Params) error {
	var version uint32
	err := walletdb.View(ctx, db, func(tx walletdb.ReadTx) error {
		var err error
		metadataBucket := tx.ReadBucket(unifiedDBMetadata{}.rootBucketKey())
		if metadataBucket == nil {
			// This could indicate either an unitialized db or one that hasn't
			// yet been migrated.
			return errors.E(errors.IO, "missing metadata bucket")
		}
		version, err = unifiedDBMetadata{}.getVersion(metadataBucket)
		return err
	})
	if err != nil {
		return err
	}

	if version > DBVersion {
		// The DB was migrated by a newer binary. Refuse to open: opening
		// silently would let this older binary read records via an older
		// wire-format codepath (e.g. V12 vs V13 TxOut layouts) and produce
		// phantom balances or silently-spent UTXOs.
		return errors.E(errors.Invalid, errors.Errorf(
			"wallet database version %d is newer than this binary supports (%d); "+
				"upgrade the binary instead of running an older one against a migrated DB",
			version, DBVersion))
	}
	if version == DBVersion {
		// No upgrades necessary.
		return nil
	}

	log.Infof("Upgrading database from version %d to %d", version, DBVersion)

	return walletdb.Update(ctx, db, func(tx walletdb.ReadWriteTx) error {
		// Execute all necessary upgrades in order.
		for _, upgrade := range upgrades[version:] {
			err := upgrade(tx, publicPassphrase, params)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// consolidationAddressUpgrade creates the account consolidation bucket for
// storing per-account SSFee consolidation addresses.
func consolidationAddressUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 28
	const newVersion = 29

	// Assert that this function is only called on version 28 databases.
	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, errors.Errorf("consolidationAddressUpgrade inappropriately called"))
	}

	// Create the account consolidation bucket
	_, err = tx.CreateTopLevelBucket(accountConsolidationBucketKey)
	if err != nil {
		return errors.E(errors.IO, err)
	}

	// Update the database version
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

// skaBucketsUpgrade performs an upgrade from version 29 to 30. This upgrade
// creates buckets for SKA credit amounts (big.Int storage that exceeds int64)
// and SSFee transaction markers (to avoid deserializing transactions for checks).
func skaBucketsUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 29
	const newVersion = 30

	// Assert that this function is only called on version 29 databases.
	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, errors.Errorf("skaBucketsUpgrade inappropriately called"))
	}

	txmgrBucket := tx.ReadWriteBucket(wtxmgrBucketKey)
	if txmgrBucket == nil {
		return errors.E(errors.IO, "missing transaction manager bucket")
	}

	// Create bucket for SKA credit amounts (big.Int storage)
	_, err = txmgrBucket.CreateBucket(bucketSKACreditAmounts)
	if err != nil {
		return errors.E(errors.IO, err)
	}

	// Create bucket for SSFee transaction markers
	_, err = txmgrBucket.CreateBucket(bucketSSFeeMarkers)
	if err != nil {
		return errors.E(errors.IO, err)
	}

	// Update the database version
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

// wireFormatV13Upgrade performs an upgrade from version 30 to 31. This upgrade
// migrates all transaction records from the V12 wire format to the V13 wire format.
// The V12 format serialized TxOut as [Value:8][CoinType:1][Version:2][PkScript:var]
// The V13 format serializes TxOut as [CoinType:1][Value:8+][Version:2][PkScript:var]
// This change was necessary to support variable-length SKA amounts (big.Int).
//
// Note: If a database went through dualCoinUpgrade after the V13 wire format was
// introduced, transactions may already be in V13 format. This upgrade tries V12
// first, falls back to V13 if that fails, and only re-serializes if needed.
func wireFormatV13Upgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 30
	const newVersion = 31

	// Assert that this function is only called on version 30 databases.
	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, errors.Errorf("wireFormatV13Upgrade inappropriately called"))
	}

	txmgrBucket := tx.ReadWriteBucket(wtxmgrBucketKey)
	if txmgrBucket == nil {
		return errors.E(errors.IO, "missing transaction manager bucket")
	}

	// Migrate mined transaction records from V12 to V13 wire format.
	txRecordsBucket := txmgrBucket.NestedReadWriteBucket(bucketTxRecords)
	if txRecordsBucket != nil {
		migratedRecords := make(map[string][]byte)
		err := txRecordsBucket.ForEach(func(k, v []byte) error {
			var hash chainhash.Hash
			err := readRawTxRecordHash(k, &hash)
			if err != nil {
				return err
			}

			// Try reading with V12 format first
			var rec TxRecord
			err = readRawTxRecordV12(&hash, v, &rec)
			if err != nil {
				// V12 decode failed, try V13 (data may already be migrated)
				err = readRawTxRecord(&hash, v, &rec)
				if err != nil {
					// Both failed, skip this record (shouldn't happen)
					return nil
				}
				// Already in V13 format, no migration needed
				return nil
			}

			// Re-serialize in V13 format
			newV, err := valueTxRecord(&rec)
			if err != nil {
				return err
			}
			migratedRecords[string(k)] = newV
			return nil
		})
		if err != nil {
			return errors.E(errors.IO, err)
		}

		// Write migrated records
		for k, v := range migratedRecords {
			if err := txRecordsBucket.Put([]byte(k), v); err != nil {
				return errors.E(errors.IO, err)
			}
		}
	}

	// Migrate unmined transaction records from V12 to V13 wire format
	unminedBucket := txmgrBucket.NestedReadWriteBucket(bucketUnmined)
	if unminedBucket != nil {
		migratedUnmined := make(map[string][]byte)
		err := unminedBucket.ForEach(func(k, v []byte) error {
			var hash chainhash.Hash
			err := readRawUnminedHash(k, &hash)
			if err != nil {
				return err
			}

			// Try reading with V12 format first
			var rec TxRecord
			err = readRawTxRecordV12(&hash, v, &rec)
			if err != nil {
				// V12 decode failed, try V13 (data may already be migrated)
				err = readRawTxRecord(&hash, v, &rec)
				if err != nil {
					// Both failed, skip this record
					return nil
				}
				// Already in V13 format, no migration needed
				return nil
			}

			// Re-serialize in V13 format
			newV, err := valueTxRecord(&rec)
			if err != nil {
				return err
			}
			migratedUnmined[string(k)] = newV
			return nil
		})
		if err != nil {
			return errors.E(errors.IO, err)
		}

		// Write migrated records
		for k, v := range migratedUnmined {
			if err := unminedBucket.Put([]byte(k), v); err != nil {
				return errors.E(errors.IO, err)
			}
		}
	}

	// Update the database version.
	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

// multisigCoinTypeUpgrade performs an upgrade from version 31 to 32.
// It extends every persisted P2SH multisig-output record with CoinType +
// SKAAmount tail fields. All pre-upgrade entries are VAR-only (before the
// dual-coin multisig path existed) and are backfilled as CoinType=VAR /
// SKAAmount=Zero. The tail is 2 bytes (CoinType + length prefix) since the
// length prefix is zero when SKAAmount is empty.
//
// Forward-compat note for v32→v33 and beyond: fetchMultisigOut currently
// distinguishes v1 from v2 by length alone (see multisigOutV1Len in
// txdb.go). Any future schema bump must add an unambiguous discriminator
// (leading version byte, or a length-prefixed trailer) so that v3 records
// cannot share a length with any earlier variant. Continuing the
// length-probe pattern with a third variant is unsafe and will silently
// corrupt persisted values.
func multisigCoinTypeUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 31
	const newVersion = 32

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, errors.Errorf("multisigCoinTypeUpgrade inappropriately called"))
	}

	txmgrBucket := tx.ReadWriteBucket(wtxmgrBucketKey)
	if txmgrBucket == nil {
		return errors.E(errors.IO, "missing transaction manager bucket")
	}
	msBucket := txmgrBucket.NestedReadWriteBucket(bucketMultisig)
	if msBucket == nil {
		// No multisig bucket yet — nothing to migrate.
		return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
	}

	// Collect pre-upgrade entries. Modifying the bucket during ForEach is
	// not supported by all backends, so buffer first.
	type rec struct {
		k, v []byte
	}
	var toMigrate []rec
	err = msBucket.ForEach(func(k, v []byte) error {
		if len(v) == multisigOutV1Len {
			kc := make([]byte, len(k))
			copy(kc, k)
			vc := make([]byte, len(v))
			copy(vc, v)
			toMigrate = append(toMigrate, rec{kc, vc})
		}
		return nil
	})
	if err != nil {
		return errors.E(errors.IO, err)
	}

	// Append CoinType=VAR (0) + zero-length SKAAmount tail to each v1 entry.
	for _, r := range toMigrate {
		v2 := make([]byte, multisigOutV1Len+2)
		copy(v2, r.v)
		v2[multisigOutV1Len] = uint8(cointype.CoinTypeVAR)
		v2[multisigOutV1Len+1] = 0
		if err := msBucket.Put(r.k, v2); err != nil {
			return errors.E(errors.IO, err)
		}
	}

	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}

// skaSideBucketCleanupUpgrade creates the SKA stake-invalidated archive bucket
// and prunes orphan entries from the existing SKA side buckets that were
// leaked by the pre-fix code (where deleteRawCredit / deleteRawUnminedCredit
// did not cascade to bucketSKACreditAmounts / bucketSKAUnminedCreditAmounts).
//
// For each entry in bucketSKACreditAmounts whose primary credit is gone:
//   - if the credit is currently in bucketStakeInvalidatedCredits, the SKA
//     amount is migrated to the new archive bucket so a future stakeValidate
//     restores it (recovers data orphaned by the prior leak);
//   - otherwise the entry is deleted as a pure orphan.
//
// For each entry in bucketSKAUnminedCreditAmounts whose primary unmined
// credit is gone, the entry is deleted.
func skaSideBucketCleanupUpgrade(tx walletdb.ReadWriteTx, publicPassphrase []byte, params *chaincfg.Params) error {
	const oldVersion = 32
	const newVersion = 33

	metadataBucket := tx.ReadWriteBucket(unifiedDBMetadata{}.rootBucketKey())
	dbVersion, err := unifiedDBMetadata{}.getVersion(metadataBucket)
	if err != nil {
		return err
	}
	if dbVersion != oldVersion {
		return errors.E(errors.Invalid, errors.Errorf("skaSideBucketCleanupUpgrade inappropriately called"))
	}

	txmgrBucket := tx.ReadWriteBucket(wtxmgrBucketKey)
	if txmgrBucket == nil {
		return errors.E(errors.IO, "missing transaction manager bucket")
	}

	// Create the new archive bucket (idempotent on retry).
	if _, err := txmgrBucket.CreateBucketIfNotExists(bucketSKAStakeInvalidatedCreditAmounts); err != nil {
		return errors.E(errors.IO, err)
	}

	// Prune orphans in bucketSKACreditAmounts. Buffer keys first because
	// modifying a bucket under ForEach is not supported by all backends.
	skaBucket := txmgrBucket.NestedReadWriteBucket(bucketSKACreditAmounts)
	creditsBucket := txmgrBucket.NestedReadBucket(bucketCredits)
	invCreditsBucket := txmgrBucket.NestedReadBucket(bucketStakeInvalidatedCredits)
	if skaBucket != nil {
		type rec struct {
			k, v []byte
		}
		var orphans []rec
		var migrate []rec
		err = skaBucket.ForEach(func(k, v []byte) error {
			liveExists := creditsBucket != nil && creditsBucket.Get(k) != nil
			if liveExists {
				return nil
			}
			invExists := invCreditsBucket != nil && invCreditsBucket.Get(k) != nil
			kc := make([]byte, len(k))
			copy(kc, k)
			vc := make([]byte, len(v))
			copy(vc, v)
			if invExists {
				migrate = append(migrate, rec{kc, vc})
			} else {
				orphans = append(orphans, rec{kc, vc})
			}
			return nil
		})
		if err != nil {
			return errors.E(errors.IO, err)
		}
		archiveBucket := txmgrBucket.NestedReadWriteBucket(bucketSKAStakeInvalidatedCreditAmounts)
		for _, r := range migrate {
			if err := archiveBucket.Put(r.k, r.v); err != nil {
				return errors.E(errors.IO, err)
			}
			if err := skaBucket.Delete(r.k); err != nil {
				return errors.E(errors.IO, err)
			}
		}
		for _, r := range orphans {
			if err := skaBucket.Delete(r.k); err != nil {
				return errors.E(errors.IO, err)
			}
		}
	}

	// Prune orphans in bucketSKAUnminedCreditAmounts. An entry is an orphan
	// when no per-coin-type unmined credit exists for its key.
	unminedSkaBucket := txmgrBucket.NestedReadWriteBucket(bucketSKAUnminedCreditAmounts)
	if unminedSkaBucket != nil {
		var orphanKeys [][]byte
		err = unminedSkaBucket.ForEach(func(k, _ []byte) error {
			if existsRawUnminedCredit(txmgrBucket, k, params) == nil {
				kc := make([]byte, len(k))
				copy(kc, k)
				orphanKeys = append(orphanKeys, kc)
			}
			return nil
		})
		if err != nil {
			return errors.E(errors.IO, err)
		}
		for _, k := range orphanKeys {
			if err := unminedSkaBucket.Delete(k); err != nil {
				return errors.E(errors.IO, err)
			}
		}
	}

	return unifiedDBMetadata{}.putVersion(metadataBucket, newVersion)
}
