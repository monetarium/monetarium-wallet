// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txrules_test

import (
	"math/big"
	"strings"
	"testing"

	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/wire"
	"github.com/monetarium/monetarium-wallet/wallet/txrules"
)

func TestValidateCoinTypeUniformity(t *testing.T) {
	tests := []struct {
		name      string
		outputs   []*wire.TxOut
		wantError bool
	}{
		{
			name:    "no outputs",
			outputs: nil,
		},
		{
			name: "single VAR output",
			outputs: []*wire.TxOut{
				{Value: 100, CoinType: cointype.CoinTypeVAR},
			},
		},
		{
			name: "two matching VAR outputs",
			outputs: []*wire.TxOut{
				{Value: 100, CoinType: cointype.CoinTypeVAR},
				{Value: 200, CoinType: cointype.CoinTypeVAR},
			},
		},
		{
			name: "two matching SKA1 outputs",
			outputs: []*wire.TxOut{
				{SKAValue: big.NewInt(100), CoinType: cointype.CoinType(1)},
				{SKAValue: big.NewInt(200), CoinType: cointype.CoinType(1)},
			},
		},
		{
			name: "VAR + SKA mixed",
			outputs: []*wire.TxOut{
				{Value: 100, CoinType: cointype.CoinTypeVAR},
				{SKAValue: big.NewInt(200), CoinType: cointype.CoinType(1)},
			},
			wantError: true,
		},
		{
			name: "SKA1 + SKA2 mixed",
			outputs: []*wire.TxOut{
				{SKAValue: big.NewInt(100), CoinType: cointype.CoinType(1)},
				{SKAValue: big.NewInt(200), CoinType: cointype.CoinType(2)},
			},
			wantError: true,
		},
		{
			name: "first VAR followed by VAR followed by SKA",
			outputs: []*wire.TxOut{
				{Value: 100, CoinType: cointype.CoinTypeVAR},
				{Value: 200, CoinType: cointype.CoinTypeVAR},
				{SKAValue: big.NewInt(50), CoinType: cointype.CoinType(1)},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := txrules.ValidateCoinTypeUniformity(tt.outputs)
			if tt.wantError && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantError && !strings.Contains(err.Error(), "coin type") {
				t.Fatalf("expected error message to mention 'coin type', got: %v", err)
			}
		})
	}
}
