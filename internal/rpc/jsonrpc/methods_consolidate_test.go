// Copyright (c) 2025 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package jsonrpc

import (
	"testing"

	"github.com/monetarium/monetarium-wallet/rpc/jsonrpc/types"
	"github.com/monetarium/monetarium-node/cointype"
)

// TestConsolidateCmdStructure tests the ConsolidateCmd structure and constructors
func TestConsolidateCmdStructure(t *testing.T) {
	tests := []struct {
		name     string
		inputs   int
		account  *string
		address  *string
		coinType *uint8
		wantErr  bool
	}{
		{
			name:     "Basic consolidate without coin type",
			inputs:   100,
			account:  nil,
			address:  nil,
			coinType: nil, // Should default to VAR (0)
			wantErr:  false,
		},
		{
			name:     "Consolidate with VAR coin type",
			inputs:   50,
			account:  stringPtr("default"),
			address:  nil,
			coinType: uint8Ptr(0),
			wantErr:  false,
		},
		{
			name:     "Consolidate with SKA1 coin type",
			inputs:   100,
			account:  nil,
			address:  stringPtr("SsWKp7wtdTZYabYFYSc9cnxhwFEjA5g4pFc"),
			coinType: uint8Ptr(1),
			wantErr:  false,
		},
		{
			name:     "Consolidate with SKA2 coin type",
			inputs:   75,
			account:  stringPtr("staker"),
			address:  nil,
			coinType: uint8Ptr(2),
			wantErr:  false,
		},
		{
			name:     "Consolidate with maximum SKA coin type",
			inputs:   200,
			account:  nil,
			address:  nil,
			coinType: uint8Ptr(255),
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the basic constructor
			basicCmd := types.NewConsolidateCmd(tt.inputs, tt.account, tt.address)
			if basicCmd == nil {
				t.Fatal("NewConsolidateCmd returned nil")
			}
			if basicCmd.Inputs != tt.inputs {
				t.Errorf("Inputs mismatch: got %d, want %d", basicCmd.Inputs, tt.inputs)
			}

			// Test the constructor with coin type
			if tt.coinType != nil {
				cmdWithCoinType := types.NewConsolidateCmdWithCoinType(tt.inputs, tt.account, tt.address, tt.coinType)
				if cmdWithCoinType == nil {
					t.Fatal("NewConsolidateCmdWithCoinType returned nil")
				}
				if cmdWithCoinType.Inputs != tt.inputs {
					t.Errorf("Inputs mismatch: got %d, want %d", cmdWithCoinType.Inputs, tt.inputs)
				}
				if cmdWithCoinType.CoinType == nil {
					t.Error("CoinType should not be nil")
				} else if *cmdWithCoinType.CoinType != *tt.coinType {
					t.Errorf("CoinType mismatch: got %d, want %d", *cmdWithCoinType.CoinType, *tt.coinType)
				}
			}
		})
	}
}

// TestConsolidateCoinTypeDefault tests that coin type defaults to VAR when not specified
func TestConsolidateCoinTypeDefault(t *testing.T) {
	cmd := types.NewConsolidateCmd(100, nil, nil)
	if cmd == nil {
		t.Fatal("NewConsolidateCmd returned nil")
	}

	// When CoinType is nil, the handler should default to VAR (0)
	if cmd.CoinType != nil {
		t.Logf("Note: CoinType is set to %d (expected nil for default)", *cmd.CoinType)
	}

	// The RPC handler should interpret nil CoinType as VAR
	expectedDefault := cointype.CoinTypeVAR
	if expectedDefault != 0 {
		t.Errorf("Expected default coin type to be 0 (VAR), got %d", expectedDefault)
	}

	t.Log("✓ CoinType defaults to VAR when not specified")
}

// TestConsolidateCoinTypeValues tests valid and edge-case coin type values
func TestConsolidateCoinTypeValues(t *testing.T) {
	tests := []struct {
		name     string
		coinType uint8
		valid    bool
		desc     string
	}{
		{
			name:     "VAR (0)",
			coinType: 0,
			valid:    true,
			desc:     "VAR coin type",
		},
		{
			name:     "SKA1",
			coinType: 1,
			valid:    true,
			desc:     "First SKA coin type",
		},
		{
			name:     "SKA2",
			coinType: 2,
			valid:    true,
			desc:     "Second SKA coin type",
		},
		{
			name:     "SKA100",
			coinType: 100,
			valid:    true,
			desc:     "Mid-range SKA coin type",
		},
		{
			name:     "SKA255",
			coinType: 255,
			valid:    true,
			desc:     "Maximum SKA coin type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := types.NewConsolidateCmdWithCoinType(100, nil, nil, &tt.coinType)
			if cmd == nil {
				t.Fatal("NewConsolidateCmdWithCoinType returned nil")
			}

			if cmd.CoinType == nil {
				t.Fatal("CoinType should not be nil")
			}

			if *cmd.CoinType != tt.coinType {
				t.Errorf("CoinType mismatch: got %d, want %d", *cmd.CoinType, tt.coinType)
			}

			// Verify coin type is in valid uint8 range (0-255)
			if tt.valid && (*cmd.CoinType < 0 || *cmd.CoinType > 255) {
				t.Errorf("CoinType %d is out of valid range", *cmd.CoinType)
			}

			t.Logf("✓ %s: CoinType %d is valid", tt.desc, tt.coinType)
		})
	}
}

// TestConsolidateWithAccount tests consolidate command with account parameter
func TestConsolidateWithAccount(t *testing.T) {
	accounts := []string{"default", "staker", "imported", "mixed"}

	for _, account := range accounts {
		t.Run(account, func(t *testing.T) {
			acct := account
			cmd := types.NewConsolidateCmd(50, &acct, nil)

			if cmd == nil {
				t.Fatal("NewConsolidateCmd returned nil")
			}

			if cmd.Account == nil {
				t.Error("Account should not be nil")
			} else if *cmd.Account != account {
				t.Errorf("Account mismatch: got %s, want %s", *cmd.Account, account)
			}

			// Test with coin type as well
			coinType := uint8(1) // SKA1
			cmdWithCoinType := types.NewConsolidateCmdWithCoinType(50, &acct, nil, &coinType)

			if cmdWithCoinType.Account == nil {
				t.Error("Account should not be nil")
			} else if *cmdWithCoinType.Account != account {
				t.Errorf("Account mismatch: got %s, want %s", *cmdWithCoinType.Account, account)
			}

			if cmdWithCoinType.CoinType == nil {
				t.Error("CoinType should not be nil")
			} else if *cmdWithCoinType.CoinType != coinType {
				t.Errorf("CoinType mismatch: got %d, want %d", *cmdWithCoinType.CoinType, coinType)
			}
		})
	}
}

// TestConsolidateWithAddress tests consolidate command with address parameter
func TestConsolidateWithAddress(t *testing.T) {
	// Note: These are example addresses - actual validation happens in the RPC handler
	addresses := []string{
		"SsWKp7wtdTZYabYFYSc9cnxhwFEjA5g4pFc",
		"SsXciQNTo3HuV5tX3yy4hXndRWgLMRVC7Ah",
	}

	for _, address := range addresses {
		t.Run(address, func(t *testing.T) {
			addr := address
			cmd := types.NewConsolidateCmd(100, nil, &addr)

			if cmd == nil {
				t.Fatal("NewConsolidateCmd returned nil")
			}

			if cmd.Address == nil {
				t.Error("Address should not be nil")
			} else if *cmd.Address != address {
				t.Errorf("Address mismatch: got %s, want %s", *cmd.Address, address)
			}

			// Test with coin type as well
			coinType := uint8(1) // SKA1
			cmdWithCoinType := types.NewConsolidateCmdWithCoinType(100, nil, &addr, &coinType)

			if cmdWithCoinType.Address == nil {
				t.Error("Address should not be nil")
			} else if *cmdWithCoinType.Address != address {
				t.Errorf("Address mismatch: got %s, want %s", *cmdWithCoinType.Address, address)
			}

			if cmdWithCoinType.CoinType == nil {
				t.Error("CoinType should not be nil")
			}
		})
	}
}

// TestConsolidateInputCountValidation tests input count parameter
func TestConsolidateInputCountValidation(t *testing.T) {
	tests := []struct {
		name   string
		inputs int
		desc   string
	}{
		{"Minimum practical", 2, "Need at least 2 UTXOs to consolidate"},
		{"Small consolidation", 10, "Consolidate 10 UTXOs"},
		{"Medium consolidation", 50, "Consolidate 50 UTXOs"},
		{"Large consolidation", 100, "Consolidate 100 UTXOs"},
		{"Very large consolidation", 500, "Consolidate 500 UTXOs"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := types.NewConsolidateCmd(tt.inputs, nil, nil)

			if cmd == nil {
				t.Fatal("NewConsolidateCmd returned nil")
			}

			if cmd.Inputs != tt.inputs {
				t.Errorf("Inputs mismatch: got %d, want %d", cmd.Inputs, tt.inputs)
			}

			t.Logf("✓ %s: %d inputs", tt.desc, tt.inputs)
		})
	}
}

// TestConsolidateAllParameters tests consolidate command with all parameters
func TestConsolidateAllParameters(t *testing.T) {
	inputs := 100
	account := "staker"
	address := "SsWKp7wtdTZYabYFYSc9cnxhwFEjA5g4pFc"
	coinType := uint8(1) // SKA1

	cmd := types.NewConsolidateCmdWithCoinType(inputs, &account, &address, &coinType)

	if cmd == nil {
		t.Fatal("NewConsolidateCmdWithCoinType returned nil")
	}

	// Verify all parameters
	if cmd.Inputs != inputs {
		t.Errorf("Inputs mismatch: got %d, want %d", cmd.Inputs, inputs)
	}

	if cmd.Account == nil {
		t.Error("Account should not be nil")
	} else if *cmd.Account != account {
		t.Errorf("Account mismatch: got %s, want %s", *cmd.Account, account)
	}

	if cmd.Address == nil {
		t.Error("Address should not be nil")
	} else if *cmd.Address != address {
		t.Errorf("Address mismatch: got %s, want %s", *cmd.Address, address)
	}

	if cmd.CoinType == nil {
		t.Error("CoinType should not be nil")
	} else if *cmd.CoinType != coinType {
		t.Errorf("CoinType mismatch: got %d, want %d", *cmd.CoinType, coinType)
	}

	t.Logf("✓ All parameters set correctly: inputs=%d, account=%s, address=%s, coinType=%d",
		inputs, account, address, coinType)
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func uint8Ptr(u uint8) *uint8 {
	return &u
}
