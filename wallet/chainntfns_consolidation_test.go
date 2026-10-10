// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"bytes"
	"context"
	"encoding/hex"
	"testing"

	"github.com/monetarium/monetarium-node/chaincfg/chainhash"
	"github.com/monetarium/monetarium-node/txscript/stdaddr"
	"github.com/monetarium/monetarium-wallet/wallet/walletdb"
)

// TestTicketConsolidationHash160ForeignCommitment covers the configuration a
// VSP voting wallet is always in: it holds a ticket's imported voting key, but
// the ticket's commitment address belongs to the buyer and is unknown here.
//
// This must yield the ticket's own commitment hash160, not an error. Returning
// an error is what the caller used to do, and it abandoned every vote such a
// wallet was asked to cast.
func TestTicketConsolidationHash160ForeignCommitment(t *testing.T) {
	ctx := context.Background()
	cfg := basicWalletConfig
	w, teardown := testWallet(ctx, t, &cfg, seed)
	defer teardown()

	// An address this wallet has never derived — the buyer's.
	foreignHash160 := hexToBytes("00112233445566778899aabbccddeeff00112233")
	foreignAddr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(
		foreignHash160, w.chainParams)
	if err != nil {
		t.Fatal(err)
	}

	var got []byte
	err = walletdb.View(ctx, w.db, func(dbtx walletdb.ReadTx) error {
		addrmgrNs := dbtx.ReadBucket(waddrmgrNamespaceKey)
		got, err = w.ticketConsolidationHash160(dbtx, addrmgrNs,
			&chainhash.Hash{}, foreignAddr, foreignHash160)
		return err
	})
	if err != nil {
		t.Fatalf("ticketConsolidationHash160: %v", err)
	}

	if !bytes.Equal(got, foreignHash160) {
		t.Errorf("consolidation hash160 = %s, want the ticket's commitment "+
			"hash160 %s", hex.EncodeToString(got),
			hex.EncodeToString(foreignHash160))
	}
}

// TestTicketConsolidationHash160OwnCommitment covers the other side: a buyer
// voting their own ticket still consolidates to their account's address, not
// to the commitment address.
//
// The ticket's commitment address here is the account's *second* external
// address, so the two candidate answers differ and the test can tell which one
// was taken — checking against the first external address would pass either
// way.
func TestTicketConsolidationHash160OwnCommitment(t *testing.T) {
	ctx := context.Background()
	cfg := basicWalletConfig
	w, teardown := testWallet(ctx, t, &cfg, seed)
	defer teardown()

	var addrs []stdaddr.Address
	for i := 0; i < 2; i++ {
		addr, err := w.NewExternalAddress(ctx, defaultAccount)
		if err != nil {
			t.Fatalf("NewExternalAddress: %v", err)
		}
		addrs = append(addrs, addr)
	}

	hash160 := func(addr stdaddr.Address) []byte {
		ka, err := w.KnownAddress(ctx, addr)
		if err != nil {
			t.Fatalf("KnownAddress: %v", err)
		}
		return ka.(BIP0044Address).PubKeyHash()
	}
	firstExternal, commitment := hash160(addrs[0]), hash160(addrs[1])
	if bytes.Equal(firstExternal, commitment) {
		t.Fatal("the two external addresses are identical; the test cannot " +
			"distinguish the account address from the commitment address")
	}

	commitmentAddr, ok := addrs[1].(stdaddr.StakeAddress)
	if !ok {
		t.Fatalf("%T is not a stake address", addrs[1])
	}

	var got []byte
	err := walletdb.View(ctx, w.db, func(dbtx walletdb.ReadTx) error {
		var err error
		addrmgrNs := dbtx.ReadBucket(waddrmgrNamespaceKey)
		got, err = w.ticketConsolidationHash160(dbtx, addrmgrNs,
			&chainhash.Hash{}, commitmentAddr, commitment)
		return err
	})
	if err != nil {
		t.Fatalf("ticketConsolidationHash160: %v", err)
	}

	if !bytes.Equal(got, firstExternal) {
		t.Errorf("consolidation hash160 = %s, want the account's first "+
			"external address %s", hex.EncodeToString(got),
			hex.EncodeToString(firstExternal))
	}
}
