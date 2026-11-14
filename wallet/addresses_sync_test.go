// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"context"
	"testing"

	"github.com/monetarium/monetarium-node/txscript/stdaddr"
	"github.com/monetarium/monetarium-node/wire"
	"github.com/monetarium/monetarium-wallet/wallet/udb"
)

// recordingNetwork records the addresses passed to LoadTxFilter.
type recordingNetwork struct {
	mockNetwork
	loadTxFilterAddrs []stdaddr.Address
}

func (n *recordingNetwork) LoadTxFilter(ctx context.Context, reload bool, addrs []stdaddr.Address, outpoints []wire.OutPoint) error {
	n.loadTxFilterAddrs = addrs
	return nil
}

// TestSyncLastReturnedAddressMovesRequestedBranch verifies that
// SyncLastReturnedAddress advances the cursor of the branch it is asked
// to sync and leaves the other branch alone. It is a regression test for
// the external branch case picking up the internal address buffer, so
// accountsyncaddressindex <account> 0 <index> moved the change-address
// cursor instead of the external one.
func TestSyncLastReturnedAddressMovesRequestedBranch(t *testing.T) {
	ctx := context.Background()
	cfg := basicWalletConfig
	w, teardown := testWallet(ctx, t, &cfg, nil)
	defer teardown()

	w.SetNetworkBackend(mockNetwork{})

	// The cursor of a branch only moves when the synced child is beyond
	// the branch's last used child. Give both branches a last used child
	// so the test observes exactly which buffer is advanced.
	w.addressBuffersMu.Lock()
	w.addressBuffers[0].albExternal.lastUsed = 10
	w.addressBuffers[0].albInternal.lastUsed = 5
	w.addressBuffersMu.Unlock()

	cursors := func() (ext, internal uint32) {
		w.addressBuffersMu.Lock()
		defer w.addressBuffersMu.Unlock()
		b := w.addressBuffers[0]
		return b.albExternal.cursor, b.albInternal.cursor
	}

	err := w.SyncLastReturnedAddress(ctx, 0, udb.ExternalBranch, 30)
	if err != nil {
		t.Fatal(err)
	}

	ext, internal := cursors()
	if ext != 20 {
		t.Fatalf("external cursor: got %d, want 20 (child 30 past last used 10)", ext)
	}
	if internal != 0 {
		t.Fatalf("internal cursor: got %d, want 0 (the internal branch was not synced)",
			internal)
	}

	// The same call for the internal branch must move the internal
	// cursor and leave the external one alone.
	err = w.SyncLastReturnedAddress(ctx, 0, udb.InternalBranch, 25)
	if err != nil {
		t.Fatal(err)
	}

	ext, internal = cursors()
	if internal != 20 {
		t.Fatalf("internal cursor: got %d, want 20 (child 25 past last used 5)", internal)
	}
	if ext != 20 {
		t.Fatalf("external cursor: got %d, want 20 (unchanged by the internal sync)", ext)
	}
}

// TestSyncLastReturnedAddressLoadsExternalBranchAddresses verifies that the
// transaction filter is loaded with the addresses of the branch being
// synced, not those of the other branch (the second symptom in #33). With
// lastUsed 10 and a gap limit of 20, child 30 is exactly lastWatched and no
// addresses are loaded, so a child past 30 (35) is what drives the
// LoadTxFilter call: external children 31 to 35.
func TestSyncLastReturnedAddressLoadsExternalBranchAddresses(t *testing.T) {
	ctx := context.Background()
	cfg := basicWalletConfig
	w, teardown := testWallet(ctx, t, &cfg, nil)
	defer teardown()

	network := &recordingNetwork{}
	w.SetNetworkBackend(network)

	w.addressBuffersMu.Lock()
	w.addressBuffers[0].albExternal.lastUsed = 10
	w.addressBuffers[0].albInternal.lastUsed = 5
	w.addressBuffersMu.Unlock()

	err := w.SyncLastReturnedAddress(ctx, 0, udb.ExternalBranch, 35)
	if err != nil {
		t.Fatal(err)
	}

	w.addressBuffersMu.Lock()
	extXpub := w.addressBuffers[0].albExternal.branchXpub
	w.addressBuffersMu.Unlock()
	want, err := deriveChildAddresses(extXpub, 10+w.gapLimit+1, 35-(10+w.gapLimit),
		w.chainParams)
	if err != nil {
		t.Fatal(err)
	}
	if len(want) == 0 {
		t.Fatal("no expected addresses derived")
	}

	got := network.loadTxFilterAddrs
	if len(got) != len(want) {
		t.Fatalf("LoadTxFilter got %d addresses, want %d (external children %d to 35)",
			len(got), len(want), 10+w.gapLimit+1)
	}
	for i := range want {
		if got[i].String() != want[i].String() {
			t.Fatalf("LoadTxFilter address %d: got %v, want external child address %v",
				i, got[i], want[i])
		}
	}
}

// TestSyncLastReturnedAddressFreshBranch verifies that the cursor moves on a
// branch with no used address yet (lastUsed == ^uint32(0), every new account
// starts so): child - ^uint32(0) == child + 1 is the right cursor across the
// wrap. With the guard requiring a used address, the cursor is not moved at
// all, and after getnewaddress returned children 0 to 2,
// accountsyncaddressindex <account> 0 10 leaves the cursor at 3, so the
// next getnewaddress returns child 3, below the synced index — the third
// symptom in #33.
func TestSyncLastReturnedAddressFreshBranch(t *testing.T) {
	ctx := context.Background()
	cfg := basicWalletConfig
	w, teardown := testWallet(ctx, t, &cfg, nil)
	defer teardown()

	w.addressBuffersMu.Lock()
	lastUsed := w.addressBuffers[0].albExternal.lastUsed
	extXpub := w.addressBuffers[0].albExternal.branchXpub
	w.addressBuffersMu.Unlock()
	if lastUsed != ^uint32(0) {
		t.Fatalf("premise: a fresh external branch has lastUsed ^uint32(0), got %d",
			lastUsed)
	}

	for i := 0; i < 3; i++ {
		if _, err := w.NewExternalAddress(ctx, 0); err != nil {
			t.Fatal(err)
		}
	}
	err := w.SyncLastReturnedAddress(ctx, 0, udb.ExternalBranch, 10)
	if err != nil {
		t.Fatal(err)
	}

	addr, err := w.NewExternalAddress(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	want, err := deriveChildAddress(extXpub, 11, w.chainParams)
	if err != nil {
		t.Fatal(err)
	}
	if addr.String() != want.String() {
		t.Fatalf("next address after syncing to index 10: got %v, want child 11 (%v)",
			addr, want)
	}
}

// TestSyncLastReturnedAddressDoesNotMoveCursorBack verifies that syncing a
// child below the branch's current cursor never moves the cursor back: with
// children up to 30 already handed out (lastUsed 10, cursor 20), a sync to
// child 15 must leave the cursor at 20. Moving it to 5 would make the next
// getnewaddress return child 16, an address already given out — and
// MarkReturnedChildIndex only moves forward, so the database and memory
// would disagree until the buffers are rebuilt.
func TestSyncLastReturnedAddressDoesNotMoveCursorBack(t *testing.T) {
	ctx := context.Background()
	cfg := basicWalletConfig
	w, teardown := testWallet(ctx, t, &cfg, nil)
	defer teardown()

	w.addressBuffersMu.Lock()
	w.addressBuffers[0].albExternal.lastUsed = 10
	w.addressBuffers[0].albExternal.cursor = 20
	w.addressBuffersMu.Unlock()

	err := w.SyncLastReturnedAddress(ctx, 0, udb.ExternalBranch, 15)
	if err != nil {
		t.Fatal(err)
	}

	// The cursor stays at 20, so the next child remains lastUsed+1+cursor
	// == 31; at 5 it would be 16, an address already given out.
	w.addressBuffersMu.Lock()
	cursor := w.addressBuffers[0].albExternal.cursor
	w.addressBuffersMu.Unlock()
	if cursor != 20 {
		t.Fatalf("external cursor after syncing child 15: got %d, want 20 (unchanged)",
			cursor)
	}

	// A sync past the cursor still moves it forward.
	err = w.SyncLastReturnedAddress(ctx, 0, udb.ExternalBranch, 35)
	if err != nil {
		t.Fatal(err)
	}
	w.addressBuffersMu.Lock()
	cursor = w.addressBuffers[0].albExternal.cursor
	w.addressBuffersMu.Unlock()
	if cursor != 25 {
		t.Fatalf("external cursor after syncing child 35: got %d, want 25 (35-10)",
			cursor)
	}
}

// TestSyncLastReturnedAddressUsedBranchBelowLastUsed verifies that syncing a
// child below the branch's last used child takes no update at all: only a
// fresh branch (lastUsed == ^uint32(0)) or a child past lastUsed moves the
// cursor. With lastUsed 10 and children up to 25 already handed out
// (cursor 15), a sync to child 5 must leave the cursor alone — taking the
// update would set it to child - lastUsed == 4294967291 across the wrap, and
// the next getnewaddress would then return child 6, an address already
// handed out (or fail with the gap-limit error, by policy).
func TestSyncLastReturnedAddressUsedBranchBelowLastUsed(t *testing.T) {
	ctx := context.Background()
	cfg := basicWalletConfig
	w, teardown := testWallet(ctx, t, &cfg, nil)
	defer teardown()

	w.addressBuffersMu.Lock()
	extXpub := w.addressBuffers[0].albExternal.branchXpub
	w.addressBuffers[0].albExternal.lastUsed = 10
	w.addressBuffers[0].albExternal.cursor = 15
	w.addressBuffersMu.Unlock()

	err := w.SyncLastReturnedAddress(ctx, 0, udb.ExternalBranch, 5)
	if err != nil {
		t.Fatal(err)
	}

	w.addressBuffersMu.Lock()
	cursor := w.addressBuffers[0].albExternal.cursor
	w.addressBuffersMu.Unlock()
	if cursor != 15 {
		t.Fatalf("external cursor after syncing child 5 below lastUsed 10: got %d, "+
			"want 15 (unchanged)", cursor)
	}

	// The next address is still the one the cursor points at.
	addr, err := w.NewExternalAddress(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	want, err := deriveChildAddress(extXpub, 26, w.chainParams)
	if err != nil {
		t.Fatal(err)
	}
	if addr.String() != want.String() {
		t.Fatalf("next address after syncing child 5: got %v, want child 26 (%v)",
			addr, want)
	}
}

// TestSyncLastReturnedAddressFreshBranchBackwardSync verifies that the
// forward-only check covers a fresh branch too. After five getnewaddress
// calls the cursor is 5, and syncing child 2 computes next =
// 2 - ^uint32(0) == 3, below the cursor: taking it would make the next
// address child 3, already handed out.
func TestSyncLastReturnedAddressFreshBranchBackwardSync(t *testing.T) {
	ctx := context.Background()
	cfg := basicWalletConfig
	w, teardown := testWallet(ctx, t, &cfg, nil)
	defer teardown()

	w.addressBuffersMu.Lock()
	extXpub := w.addressBuffers[0].albExternal.branchXpub
	lastUsed := w.addressBuffers[0].albExternal.lastUsed
	w.addressBuffersMu.Unlock()
	if lastUsed != ^uint32(0) {
		t.Fatalf("premise: a fresh external branch has lastUsed ^uint32(0), got %d",
			lastUsed)
	}

	for i := 0; i < 5; i++ {
		if _, err := w.NewExternalAddress(ctx, 0); err != nil {
			t.Fatal(err)
		}
	}
	err := w.SyncLastReturnedAddress(ctx, 0, udb.ExternalBranch, 2)
	if err != nil {
		t.Fatal(err)
	}

	addr, err := w.NewExternalAddress(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	want, err := deriveChildAddress(extXpub, 5, w.chainParams)
	if err != nil {
		t.Fatal(err)
	}
	if addr.String() != want.String() {
		t.Fatalf("next address after syncing child 2: got %v, want child 5 (%v), "+
			"not child 3 which was already handed out", addr, want)
	}
}
