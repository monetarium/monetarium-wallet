// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package jsonrpc

import (
	"context"
	"testing"

	"github.com/monetarium/monetarium-node/chaincfg"
	"github.com/monetarium/monetarium-node/dcrutil"
	"github.com/monetarium/monetarium-node/txscript/stdaddr"
	"github.com/monetarium/monetarium-wallet/rpc/jsonrpc/types"
)

// simnetP2PKH derives a simnet P2PKH address from a 20-byte public-key hash
// literal. Used to construct deterministic, well-formed addresses for tests
// without spinning up a wallet.
func simnetP2PKH(t *testing.T, pkh20 [20]byte) string {
	t.Helper()
	addr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(pkh20[:], chaincfg.SimNetParams())
	if err != nil {
		t.Fatalf("NewAddressPubKeyHashEcdsaSecp256k1V0: %v", err)
	}
	return addr.String()
}

// TestCreateRawTransactionVARDeterministicOutputOrdering verifies that
// createrawtransaction produces the same on-the-wire output ordering across
// repeated calls with the same Amounts map. Go's map iteration is randomized,
// so iterating cmd.Amounts directly produced different tx hashes each call —
// flaky for any multisig coordinator that builds a tx and hands it to N
// signers expecting them to verify against the same hash.
//
// The handler now sorts encoded addresses before iterating; this test runs
// the handler several times and asserts the result is byte-identical.
func TestCreateRawTransactionVARDeterministicOutputOrdering(t *testing.T) {
	ctx := context.Background()

	// Build four well-formed simnet P2PKH addresses from deterministic pkh
	// literals. Hash160 is just the identity-sized blob here since we never
	// spend these — only their address-encoded form matters for ordering.
	makePKH := func(seed byte) [20]byte {
		var pkh [20]byte
		copy(pkh[:], dcrutil.Hash160([]byte{seed}))
		return pkh
	}
	addrA := simnetP2PKH(t, makePKH(0xA0))
	addrB := simnetP2PKH(t, makePKH(0xB1))
	addrC := simnetP2PKH(t, makePKH(0xC2))
	addrD := simnetP2PKH(t, makePKH(0xD3))

	amounts := map[string]string{
		addrA: "1.0",
		addrB: "2.0",
		addrC: "3.0",
		addrD: "4.0",
	}

	server := &Server{activeNet: chaincfg.SimNetParams()}

	cmd := &types.CreateRawTransactionCmd{
		Inputs:  nil, // empty: skips wallet-loader access path
		Amounts: amounts,
	}

	first, err := server.createRawTransaction(ctx, cmd)
	if err != nil {
		t.Fatalf("createRawTransaction: %v", err)
	}
	firstHex, ok := first.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", first)
	}

	// Run the handler several more times. Map iteration order is randomized
	// per call, so without the deterministic-ordering fix this loop would
	// fail probabilistically (1 in N!, where N is the number of entries —
	// 4 entries → 23/24 of the time without the fix).
	for i := 0; i < 16; i++ {
		got, err := server.createRawTransaction(ctx, cmd)
		if err != nil {
			t.Fatalf("createRawTransaction iteration %d: %v", i, err)
		}
		gotHex, ok := got.(string)
		if !ok {
			t.Fatalf("iteration %d: expected string result, got %T", i, got)
		}
		if gotHex != firstHex {
			t.Fatalf("iteration %d: tx hex differs from first run\n  first: %s\n  got:   %s",
				i, firstHex, gotHex)
		}
	}
}
