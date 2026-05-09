// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package udb

import (
	"math/big"
	"testing"

	"github.com/monetarium/monetarium-node/chaincfg/chainhash"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/dcrutil"
	"github.com/monetarium/monetarium-node/wire"
	"golang.org/x/crypto/ripemd160"
)

// TestMultisigOutV2RoundTripVAR covers the common VAR case: the v2 tail carries
// CoinType=VAR and an empty SKAAmount. fetchMultisigOut must reconstruct the
// original struct exactly.
func TestMultisigOutV2RoundTripVAR(t *testing.T) {
	var sh [ripemd160.Size]byte
	for i := range sh {
		sh[i] = byte(i)
	}
	var bh, sb, th chainhash.Hash
	for i := range bh {
		bh[i] = byte(i + 1)
		sb[i] = byte(i + 2)
		th[i] = byte(i + 3)
	}
	key := keyMultisigOut(th, 7)
	val, err := valueMultisigOut(sh, 2, 3, false, wire.TxTreeRegular,
		bh, 100, dcrutil.Amount(1_000_000_000), sb, 42, th,
		cointype.CoinTypeVAR, cointype.Zero())
	if err != nil {
		t.Fatalf("valueMultisigOut: %v", err)
	}

	mso, err := fetchMultisigOut(key, val)
	if err != nil {
		t.Fatalf("fetchMultisigOut: %v", err)
	}
	if mso.CoinType != cointype.CoinTypeVAR {
		t.Errorf("CoinType: got %v want VAR", mso.CoinType)
	}
	if !mso.SKAAmount.IsZero() {
		t.Errorf("SKAAmount must be zero for VAR, got %v", mso.SKAAmount)
	}
	if mso.Amount != dcrutil.Amount(1_000_000_000) {
		t.Errorf("Amount: got %v want 1e9", mso.Amount)
	}
	if mso.M != 2 || mso.N != 3 {
		t.Errorf("M/N: got %d/%d want 2/3", mso.M, mso.N)
	}
	if mso.BlockHeight != 100 {
		t.Errorf("BlockHeight: got %d want 100", mso.BlockHeight)
	}
}

// TestValueMultisigOutMinimumV2Length pins the schema-evolution invariant: a
// v2 record with SKAAmount=Zero (skaBytes=nil, length=0) must be exactly
// multisigOutV1Len+2 = 137 bytes — never 135. A 135-byte v2 record would
// alias the v1 length and be silently misread by fetchMultisigOut as a
// pre-dual-coin VAR record, corrupting persisted multisig data. This test
// catches any future tail-encoding regression that breaks that invariant.
func TestValueMultisigOutMinimumV2Length(t *testing.T) {
	var sh [ripemd160.Size]byte
	var bh, sb, th chainhash.Hash

	v, err := valueMultisigOut(sh, 1, 1, false, wire.TxTreeRegular,
		bh, 0, 0, sb, 0, th, cointype.CoinTypeVAR, cointype.Zero())
	if err != nil {
		t.Fatalf("valueMultisigOut: %v", err)
	}
	if got, want := len(v), multisigOutV1Len+2; got != want {
		t.Fatalf("valueMultisigOut(SKAAmount=Zero) length: got %d want %d",
			got, want)
	}
}

// TestMultisigOutV2RoundTripSKALargeAmount is the CoinType+SKAAmount case. It
// uses an SKA atom count just above math.MaxInt64 to prove the serialization
// preserves big.Int precision that the legacy int64 Amount field could not.
func TestMultisigOutV2RoundTripSKALargeAmount(t *testing.T) {
	var sh [ripemd160.Size]byte
	var bh, sb, th chainhash.Hash
	key := keyMultisigOut(th, 0)

	// 2^70 atoms — far beyond int64's range, representative of a real SKA
	// holding where AtomsPerCoin = 1e18 (2^70 ≈ 1.18 × 10^21).
	skaAtoms := new(big.Int).Lsh(big.NewInt(1), 70)
	ska := cointype.NewSKAAmount(skaAtoms)
	coinType := cointype.CoinType(1)

	val, err := valueMultisigOut(sh, 1, 1, false, wire.TxTreeRegular,
		bh, 0, 0, sb, 0, th, coinType, ska)
	if err != nil {
		t.Fatalf("valueMultisigOut: %v", err)
	}

	mso, err := fetchMultisigOut(key, val)
	if err != nil {
		t.Fatalf("fetchMultisigOut: %v", err)
	}
	if mso.CoinType != coinType {
		t.Errorf("CoinType: got %v want %v", mso.CoinType, coinType)
	}
	if mso.SKAAmount.BigInt().Cmp(skaAtoms) != 0 {
		t.Errorf("SKAAmount round-trip mismatch: got %v want %v",
			mso.SKAAmount.BigInt(), skaAtoms)
	}
}

// TestValueMultisigOutRejectsOversizeSKA verifies the L7 fix: when the
// SKAAmount magnitude exceeds the single-byte length prefix's range,
// valueMultisigOut must refuse the write rather than silently truncate
// the byte slice (which would persist a corrupted value).
func TestValueMultisigOutRejectsOversizeSKA(t *testing.T) {
	var sh [ripemd160.Size]byte
	var bh, sb, th chainhash.Hash

	// Synthesize an SKAAmount whose unsigned magnitude is >255 bytes.
	// 256 bytes * 8 bits = 2048 bits, so 1 << 2048 has a 257-byte magnitude.
	huge := new(big.Int).Lsh(big.NewInt(1), 256*8)
	ska := cointype.NewSKAAmount(huge)

	v, err := valueMultisigOut(sh, 1, 1, false, wire.TxTreeRegular,
		bh, 0, 0, sb, 0, th, cointype.CoinType(1), ska)
	if err == nil {
		t.Fatalf("valueMultisigOut: expected error for oversize SKAAmount, got nil and value of len=%d", len(v))
	}
	if v != nil {
		t.Errorf("expected nil byte slice on error, got len=%d", len(v))
	}
}

// TestFetchMultisigOutRejectsForbiddenLength locks in the HIGH-2 fix: any
// future v3 schema that accidentally produces a 136-byte record (between
// v1=135 and v2≥137) must be rejected at read time rather than silently
// misinterpreted. fetchMultisigOut explicitly errors on len==136 so a
// schema-evolution mistake fails loudly. See the comment block above
// multisigOutV1Len in txdb.go for the full rationale.
func TestFetchMultisigOutRejectsForbiddenLength(t *testing.T) {
	var th chainhash.Hash
	key := keyMultisigOut(th, 0)

	// Synthesize a 136-byte value. The first 135 bytes are a valid v1
	// payload (so we know the rejection is purely about length, not about
	// any other malformation), plus one trailing zero byte.
	var sh [ripemd160.Size]byte
	var bh, sb chainhash.Hash
	v := make([]byte, multisigOutV1Len+1)
	copy(v[0:20], sh[0:20])
	v[20], v[21], v[22] = 1, 1, 0
	copy(v[23:55], bh[:])
	byteOrder.PutUint32(v[55:59], 0)
	byteOrder.PutUint64(v[59:67], 0)
	copy(v[67:99], sb[:])
	byteOrder.PutUint32(v[99:103], 0)
	copy(v[103:135], th[:])
	// v[135] = 0 — would parse as VAR if length-collision check were absent.

	mso, err := fetchMultisigOut(key, v)
	if err == nil {
		t.Fatalf("fetchMultisigOut: expected error for 136-byte value, got nil and mso=%+v", mso)
	}
	if mso != nil {
		t.Errorf("expected nil MultisigOut on error, got %+v", mso)
	}
}

// TestFetchMultisigOutV1Backcompat confirms fetchMultisigOut remains able to
// read pre-migration v1 entries (135 bytes exactly, no dual-coin tail). This
// matters because the DB upgrade runs lazily at first open; readers may see
// v1 entries transiently, and must treat them as CoinType=VAR / SKAAmount=0
// — which is always the truth for pre-dual-coin entries.
func TestFetchMultisigOutV1Backcompat(t *testing.T) {
	var sh [ripemd160.Size]byte
	var bh, sb, th chainhash.Hash
	key := keyMultisigOut(th, 0)

	// Synthesize a v1 value manually (135 bytes, no tail).
	v1 := make([]byte, multisigOutV1Len)
	copy(v1[0:20], sh[0:20])
	v1[20], v1[21], v1[22] = 2, 3, 0
	copy(v1[23:55], bh[:])
	byteOrder.PutUint32(v1[55:59], 42)
	byteOrder.PutUint64(v1[59:67], 99)
	copy(v1[67:99], sb[:])
	byteOrder.PutUint32(v1[99:103], 0)
	copy(v1[103:135], th[:])

	mso, err := fetchMultisigOut(key, v1)
	if err != nil {
		t.Fatalf("fetchMultisigOut(v1): %v", err)
	}
	if mso.CoinType != cointype.CoinTypeVAR {
		t.Errorf("v1 entry must read back as VAR, got %v", mso.CoinType)
	}
	if !mso.SKAAmount.IsZero() {
		t.Errorf("v1 entry must read back with zero SKA amount, got %v",
			mso.SKAAmount)
	}
	if mso.Amount != dcrutil.Amount(99) {
		t.Errorf("v1 Amount round-trip wrong: got %v want 99", mso.Amount)
	}
}
