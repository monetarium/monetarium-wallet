// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcserver

import (
	"math/big"
	"strings"
	"testing"

	"github.com/monetarium/monetarium-node/chaincfg"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/wire"
)

// TestSetTxOutAmountVAR locks in the gRPC ConstructTransaction VAR amount
// contract: the same decimal-coin-string parser used by the JSON-RPC
// createrawtransaction handler. Previously this branch parsed o.Amount as
// raw int64 atoms via strconv.ParseInt, silently diverging by 1e8x from the
// JSON-RPC handler when a client switched APIs.
func TestSetTxOutAmountVAR(t *testing.T) {
	tests := []struct {
		name      string
		amount    string
		wantValue int64
		wantErr   string // substring match; empty means expect success
	}{
		{
			name:      "one and a half VAR",
			amount:    "1.5",
			wantValue: 150_000_000,
		},
		{
			name:      "exact integer VAR",
			amount:    "1",
			wantValue: 100_000_000,
		},
		{
			name:      "smallest atom",
			amount:    "0.00000001",
			wantValue: 1,
		},
		{
			name:      "max fractional precision",
			amount:    "12.34567890",
			wantValue: 1_234_567_890,
		},
		{
			name:    "more than 8 fractional digits rejected",
			amount:  "1500000000.5",
			wantErr: "exceeds VAR maximum",
		},
		{
			name:    "nine fractional digits rejected",
			amount:  "1.123456789",
			wantErr: "invalid amount",
		},
		{
			name:    "empty string rejected",
			amount:  "",
			wantErr: "amount required",
		},
		{
			name:    "zero rejected",
			amount:  "0",
			wantErr: "amount must be positive",
		},
		{
			name:    "negative rejected",
			amount:  "-1.0",
			wantErr: "invalid amount",
		},
		{
			name:    "non-numeric rejected",
			amount:  "abc",
			wantErr: "invalid amount",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &wire.TxOut{}
			err := setTxOutAmount(out, tt.amount, cointype.CoinTypeVAR, nil)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil (Value=%d)", tt.wantErr, out.Value)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out.Value != tt.wantValue {
				t.Errorf("Value = %d, want %d", out.Value, tt.wantValue)
			}
			if out.SKAValue != nil {
				t.Errorf("SKAValue should remain nil for VAR, got %s", out.SKAValue)
			}
		})
	}
}

// TestSetTxOutAmountSKA covers the SKA branch parity check: same decimal
// shape, but the result lands in SKAValue (a *big.Int) instead of Value.
func TestSetTxOutAmountSKA(t *testing.T) {
	skaCfg := &chaincfg.SKACoinConfig{
		AtomsPerCoin: new(big.Int).SetUint64(1_000_000_000_000_000_000), // 1e18 atoms/SKA coin
	}
	out := &wire.TxOut{}
	if err := setTxOutAmount(out, "1.5", cointype.CoinType(1), skaCfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := new(big.Int).SetUint64(1_500_000_000_000_000_000)
	if out.SKAValue == nil || out.SKAValue.Cmp(want) != 0 {
		t.Errorf("SKAValue = %v, want %s", out.SKAValue, want)
	}
	if out.Value != 0 {
		t.Errorf("Value should be 0 for SKA, got %d", out.Value)
	}
}
