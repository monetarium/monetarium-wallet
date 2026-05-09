// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package jsonrpc

import (
	"math/big"
	"strings"
	"testing"

	"github.com/monetarium/monetarium-node/chaincfg"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/dcrec/secp256k1"
)

// TestCreateUnsignedSKAEmissionTransactionExpiryBound pins the M5 fix from
// the 2026-05-05 review. Expiry is a uint32 on the wire; an unbounded
// int64-to-uint32 cast on emissionEnd would silently wrap if a future
// chain-params misconfiguration produced a window end outside [0, MaxUint32].
// The check rejects loudly instead.
//
// Given the current int32 EmissionHeight + int32 EmissionWindow fields,
// emissionEnd cannot exceed 2*MaxInt32 = 4_294_967_294, which is one less
// than MaxUint32 — so the upper-bound branch is unreachable today and the
// check is purely defensive against future widening of those fields. The
// negative-emissionEnd branch IS reachable (a misconfigured negative
// EmissionHeight produces a negative emissionEnd), and that's what this
// test exercises end-to-end.
func TestCreateUnsignedSKAEmissionTransactionExpiryBound(t *testing.T) {
	priv, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("generate priv key: %v", err)
	}
	pub := priv.PubKey()

	const ct = cointype.CoinType(1)
	atomsPerCoin := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)

	// Synthetic chain params with a NEGATIVE EmissionHeight so that
	// emissionEnd = emissionStart + EmissionWindow is negative — the only
	// reachable branch of the M5 check given current int32 field types.
	makeParams := func(emissionHeight, emissionWindow int32) *chaincfg.Params {
		return &chaincfg.Params{
			SKACoins: map[cointype.CoinType]*chaincfg.SKACoinConfig{
				ct: {
					CoinType:       ct,
					Active:         true,
					AtomsPerCoin:   new(big.Int).Set(atomsPerCoin),
					EmissionHeight: emissionHeight,
					EmissionWindow: emissionWindow,
					EmissionKey:    pub,
					MinRelayTxFee:  big.NewInt(1000),
				},
			},
		}
	}

	addr := "DcuRpZRkRqp35hbtuoMC5UrqkXe7TaQ2cQU" // any non-empty placeholder
	amount := new(big.Int).Set(atomsPerCoin)        // 1 SKA
	auth := func(height int64) *chaincfg.SKAEmissionAuth {
		return &chaincfg.SKAEmissionAuth{
			EmissionKey: pub,
			Nonce:       1,
			CoinType:    ct,
			Amount:      new(big.Int).Set(amount),
			Height:      height,
		}
	}

	t.Run("negative emissionEnd is rejected with actionable error", func(t *testing.T) {
		// EmissionHeight = -200, Window = 100 → emissionEnd = -100.
		// auth.Height = -150 falls inside the window so the height check
		// passes; the M5 guard then fires.
		params := makeParams(-200, 100)
		tx, err := createUnsignedSKAEmissionTransaction(
			auth(-150), []string{addr}, []*big.Int{amount}, params)
		if err == nil {
			t.Fatalf("expected error for negative emissionEnd; got tx=%v", tx)
		}
		if !strings.Contains(err.Error(), "32-bit Expiry") &&
			!strings.Contains(err.Error(), "misconfigured") {
			t.Errorf("error must explain the cause; got %q", err.Error())
		}
	})

	t.Run("max-int32 boundary does not trip the Expiry guard", func(t *testing.T) {
		// EmissionHeight = MaxInt32 - 1, EmissionWindow = 1 →
		// emissionEnd = MaxInt32, well within MaxUint32. The M5 guard
		// must NOT fire false positives at the upper boundary of the
		// legitimate uint32 range. Address decoding (which runs after
		// the M5 guard) fails on synthetic params without address
		// encoding configured — that's expected and not what this test
		// pins. Only assert the error (if any) is not the M5 message.
		params := makeParams(2147483646, 1)
		_, err := createUnsignedSKAEmissionTransaction(
			auth(2147483646), []string{addr}, []*big.Int{amount}, params)
		if err != nil &&
			(strings.Contains(err.Error(), "32-bit Expiry") ||
				strings.Contains(err.Error(), "misconfigured")) {
			t.Fatalf("M5 guard fired at the upper boundary; should have passed; err=%v", err)
		}
	})
}
