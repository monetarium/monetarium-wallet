// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"context"
	"testing"

	"github.com/monetarium/monetarium-node/chaincfg"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/txscript/stdaddr"
	"github.com/monetarium/monetarium-wallet/errors"
)

// TestTotalReceivedForAddrSKACoinTypeRejected pins the HIGH-2 contract from
// the 2026-05-04 review: TotalReceivedForAddr is VAR-only because cred.Amount
// is int64 and silently zero for SKA credits. Passing a SKA coin type must
// surface an Invalid error, not silently sum zero.
func TestTotalReceivedForAddrSKACoinTypeRejected(t *testing.T) {
	w := &Wallet{chainParams: chaincfg.SimNetParams()}
	addr := mustDecodeSimnetAddr(t, "SsWKp7wtdTZYabYFYSc9cnxhwFEjA5g4pFc")

	tests := []struct {
		name     string
		coinType cointype.CoinType
		wantErr  bool
	}{
		{"SKA1 rejected", cointype.CoinType(1), true},
		{"SKA255 rejected", cointype.CoinType(255), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := w.TotalReceivedForAddr(context.Background(), addr, 1, tt.coinType)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected Invalid error for SKA coin type, got nil")
			}
			if !errors.Is(err, errors.Invalid) {
				t.Errorf("error kind = %v, want errors.Invalid", err)
			}
		})
	}
}

// TestTotalReceivedSKAForAddrVARCoinTypeRejected pins the symmetric contract:
// TotalReceivedSKAForAddr requires a SKA coin type. Passing VAR (or 0) must
// surface an Invalid error so callers route through TotalReceivedForAddr.
func TestTotalReceivedSKAForAddrVARCoinTypeRejected(t *testing.T) {
	w := &Wallet{chainParams: chaincfg.SimNetParams()}
	addr := mustDecodeSimnetAddr(t, "SsWKp7wtdTZYabYFYSc9cnxhwFEjA5g4pFc")

	_, err := w.TotalReceivedSKAForAddr(context.Background(), addr, 1, cointype.CoinTypeVAR)
	if err == nil {
		t.Fatal("expected Invalid error for VAR coin type, got nil")
	}
	if !errors.Is(err, errors.Invalid) {
		t.Errorf("error kind = %v, want errors.Invalid", err)
	}
}

func mustDecodeSimnetAddr(t *testing.T, s string) stdaddr.Address {
	t.Helper()
	addr, err := stdaddr.DecodeAddress(s, chaincfg.SimNetParams())
	if err != nil {
		t.Fatalf("decode addr %q: %v", s, err)
	}
	return addr
}
