// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestGetBalanceCmdWithCoinType tests JSON marshaling/unmarshaling of GetBalanceCmd with coin type
func TestGetBalanceCmdWithCoinType(t *testing.T) {
	tests := []struct {
		name     string
		cmd      *GetBalanceCmd
		wantJSON string
	}{
		{
			name: "GetBalance with VAR coin type",
			cmd: &GetBalanceCmd{
				Account:  stringPtr("default"),
				MinConf:  intPtr(1),
				CoinType: uint8Ptr(0),
			},
			wantJSON: `{"account":"default","minconf":1,"cointype":0}`,
		},
		{
			name: "GetBalance with SKA coin type",
			cmd: &GetBalanceCmd{
				Account:  stringPtr("*"),
				MinConf:  intPtr(6),
				CoinType: uint8Ptr(1),
			},
			wantJSON: `{"account":"*","minconf":6,"cointype":1}`,
		},
		{
			name: "GetBalance without coin type (backward compatible)",
			cmd: &GetBalanceCmd{
				Account: stringPtr("*"),
				MinConf: intPtr(1),
				// CoinType is nil
			},
			wantJSON: `{"account":"*","minconf":1}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			got, err := json.Marshal(tt.cmd)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}

			if string(got) != tt.wantJSON {
				t.Errorf("json.Marshal() = %s, want %s", string(got), tt.wantJSON)
			}

			// Test unmarshaling
			var cmd GetBalanceCmd
			if err := json.Unmarshal([]byte(tt.wantJSON), &cmd); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}

			if !reflect.DeepEqual(&cmd, tt.cmd) {
				t.Errorf("json.Unmarshal() got = %+v, want %+v", &cmd, tt.cmd)
			}
		})
	}
}

// TestListUnspentCmdWithCoinType tests JSON marshaling/unmarshaling of ListUnspentCmd with coin type
func TestListUnspentCmdWithCoinType(t *testing.T) {
	tests := []struct {
		name     string
		cmd      *ListUnspentCmd
		wantJSON string
	}{
		{
			name: "ListUnspent with VAR coin type",
			cmd: &ListUnspentCmd{
				MinConf:  intPtr(1),
				MaxConf:  intPtr(9999999),
				CoinType: uint8Ptr(0),
			},
			wantJSON: `{"minconf":1,"maxconf":9999999,"cointype":0}`,
		},
		{
			name: "ListUnspent with SKA coin type",
			cmd: &ListUnspentCmd{
				MinConf:  intPtr(6),
				MaxConf:  intPtr(100),
				CoinType: uint8Ptr(2),
			},
			wantJSON: `{"minconf":6,"maxconf":100,"cointype":2}`,
		},
		{
			name: "ListUnspent without coin type",
			cmd: &ListUnspentCmd{
				MinConf: intPtr(1),
				MaxConf: intPtr(9999999),
			},
			wantJSON: `{"minconf":1,"maxconf":9999999}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			got, err := json.Marshal(tt.cmd)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}

			if string(got) != tt.wantJSON {
				t.Errorf("json.Marshal() = %s, want %s", string(got), tt.wantJSON)
			}

			// Test unmarshaling
			var cmd ListUnspentCmd
			if err := json.Unmarshal([]byte(tt.wantJSON), &cmd); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}

			if !reflect.DeepEqual(&cmd, tt.cmd) {
				t.Errorf("json.Unmarshal() got = %+v, want %+v", &cmd, tt.cmd)
			}
		})
	}
}

// TestSendToAddressCmdWithCoinType tests JSON marshaling/unmarshaling of SendToAddressCmd with coin type
func TestSendToAddressCmdWithCoinType(t *testing.T) {
	tests := []struct {
		name     string
		cmd      *SendToAddressCmd
		wantJSON string
	}{
		{
			name: "SendToAddress with VAR coin type",
			cmd: &SendToAddressCmd{
				Address:  "SsWKp7wtdTZYabYFYSc9cnxhwFEjA5g4pFc",
				Amount:   "1.5",
				CoinType: uint8Ptr(0),
			},
			wantJSON: `{"address":"SsWKp7wtdTZYabYFYSc9cnxhwFEjA5g4pFc","amount":"1.5","cointype":0}`,
		},
		{
			name: "SendToAddress with SKA coin type",
			cmd: &SendToAddressCmd{
				Address:  "SsWKp7wtdTZYabYFYSc9cnxhwFEjA5g4pFc",
				Amount:   "0.25",
				CoinType: uint8Ptr(1),
			},
			wantJSON: `{"address":"SsWKp7wtdTZYabYFYSc9cnxhwFEjA5g4pFc","amount":"0.25","cointype":1}`,
		},
		{
			name: "SendToAddress without coin type",
			cmd: &SendToAddressCmd{
				Address: "SsWKp7wtdTZYabYFYSc9cnxhwFEjA5g4pFc",
				Amount:  "2.0",
			},
			wantJSON: `{"address":"SsWKp7wtdTZYabYFYSc9cnxhwFEjA5g4pFc","amount":"2.0"}`,
		},
		{
			name: "SendToAddress with subtractfeefromamount=true and SKA coin type",
			cmd: &SendToAddressCmd{
				Address:               "SsWKp7wtdTZYabYFYSc9cnxhwFEjA5g4pFc",
				Amount:                "1.5",
				CoinType:              uint8Ptr(1),
				SubtractFeeFromAmount: subFeeBoolPtr(true),
			},
			wantJSON: `{"address":"SsWKp7wtdTZYabYFYSc9cnxhwFEjA5g4pFc","amount":"1.5","cointype":1,"subtractfeefromamount":true}`,
		},
		{
			name: "SendToAddress with subtractfeefromamount=false explicit",
			cmd: &SendToAddressCmd{
				Address:               "SsWKp7wtdTZYabYFYSc9cnxhwFEjA5g4pFc",
				Amount:                "0.5",
				SubtractFeeFromAmount: subFeeBoolPtr(false),
			},
			wantJSON: `{"address":"SsWKp7wtdTZYabYFYSc9cnxhwFEjA5g4pFc","amount":"0.5","subtractfeefromamount":false}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			got, err := json.Marshal(tt.cmd)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}

			if string(got) != tt.wantJSON {
				t.Errorf("json.Marshal() = %s, want %s", string(got), tt.wantJSON)
			}

			// Test unmarshaling
			var cmd SendToAddressCmd
			if err := json.Unmarshal([]byte(tt.wantJSON), &cmd); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}

			if !reflect.DeepEqual(&cmd, tt.cmd) {
				t.Errorf("json.Unmarshal() got = %+v, want %+v", &cmd, tt.cmd)
			}
		})
	}
}

// TestListUnspentResultWithCoinType tests the extended ListUnspentResult with CoinType field
func TestListUnspentResultWithCoinType(t *testing.T) {
	result := &ListUnspentResult{
		TxID:          "1111111111111111111111111111111111111111111111111111111111111111",
		Vout:          0,
		Tree:          0,
		Address:       "SsWKp7wtdTZYabYFYSc9cnxhwFEjA5g4pFc",
		Account:       "default",
		Amount:        "1.5",
		Confirmations: 10,
		Spendable:     true,
		CoinType:      1, // SKA1
	}

	expectedJSON := `{"txid":"1111111111111111111111111111111111111111111111111111111111111111","vout":0,"tree":0,"txtype":0,"address":"SsWKp7wtdTZYabYFYSc9cnxhwFEjA5g4pFc","account":"default","scriptPubKey":"","amount":"1.5","confirmations":10,"spendable":true,"cointype":1}`

	// Test marshaling
	got, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	if string(got) != expectedJSON {
		t.Errorf("json.Marshal() = %s, want %s", string(got), expectedJSON)
	}

	// Test unmarshaling
	var unmarshaled ListUnspentResult
	if err := json.Unmarshal([]byte(expectedJSON), &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if !reflect.DeepEqual(&unmarshaled, result) {
		t.Errorf("json.Unmarshal() got = %+v, want %+v", &unmarshaled, result)
	}
}

// TestCommandConstructors tests the command constructor functions with CoinType
func TestCommandConstructors(t *testing.T) {
	t.Run("NewGetBalanceCmd with CoinType", func(t *testing.T) {
		cmd := NewGetBalanceCmd(stringPtr("default"), intPtr(6))
		// Manually set CoinType to test the structure
		cmd.CoinType = uint8Ptr(1)

		if *cmd.Account != "default" {
			t.Errorf("Account = %s, want default", *cmd.Account)
		}
		if *cmd.MinConf != 6 {
			t.Errorf("MinConf = %d, want 6", *cmd.MinConf)
		}
		if *cmd.CoinType != 1 {
			t.Errorf("CoinType = %d, want 1", *cmd.CoinType)
		}
	})

	t.Run("NewListUnspentCmd with CoinType", func(t *testing.T) {
		addresses := []string{"addr1", "addr2"}
		cmd := NewListUnspentCmd(intPtr(1), intPtr(100), &addresses)
		// Manually set CoinType to test the structure
		cmd.CoinType = uint8Ptr(2)
		cmd.Account = stringPtr("default")

		if *cmd.MinConf != 1 {
			t.Errorf("MinConf = %d, want 1", *cmd.MinConf)
		}
		if *cmd.MaxConf != 100 {
			t.Errorf("MaxConf = %d, want 100", *cmd.MaxConf)
		}
		if *cmd.Account != "default" {
			t.Errorf("Account = %s, want default", *cmd.Account)
		}
		if *cmd.CoinType != 2 {
			t.Errorf("CoinType = %d, want 2", *cmd.CoinType)
		}
	})

	t.Run("NewSendToAddressCmd with CoinType", func(t *testing.T) {
		cmd := NewSendToAddressCmd("SsWKp7wtdTZYabYFYSc9cnxhwFEjA5g4pFc", "1.5", nil, nil)
		// Manually set CoinType to test the structure
		cmd.CoinType = uint8Ptr(1)

		if cmd.Address != "SsWKp7wtdTZYabYFYSc9cnxhwFEjA5g4pFc" {
			t.Errorf("Address = %s, want SsWKp7wtdTZYabYFYSc9cnxhwFEjA5g4pFc", cmd.Address)
		}
		if cmd.Amount != "1.5" {
			t.Errorf("Amount = %s, want 1.5", cmd.Amount)
		}
		if *cmd.CoinType != 1 {
			t.Errorf("CoinType = %d, want 1", *cmd.CoinType)
		}
	})
}

// TestSpendOutputsCmdWithCoinType pins the JSON wire format for the
// SKA extension to spendoutputs: optional CoinType field and AddressAmountPair
// Amount as a unified decimal coin string for both VAR and SKA.
func TestSpendOutputsCmdWithCoinType(t *testing.T) {
	t.Run("VAR (no CoinType, decimal-string amount)", func(t *testing.T) {
		cmd := &SpendOutputsCmd{
			Account:           "default",
			PreviousOutpoints: []string{"abc:0"},
			Outputs: []AddressAmountPair{
				{Address: "addr1", Amount: "1.5"},
			},
		}
		got, err := json.Marshal(cmd)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		want := `{"Account":"default","PreviousOutpoints":["abc:0"],"Outputs":[{"address":"addr1","amount":"1.5"}]}`
		if string(got) != want {
			t.Errorf("Marshal: got %s, want %s", got, want)
		}

		var round SpendOutputsCmd
		if err := json.Unmarshal(got, &round); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if round.CoinType != nil {
			t.Errorf("CoinType: want nil, got %v", *round.CoinType)
		}
		if round.Outputs[0].Amount != "1.5" {
			t.Errorf("Amount: want \"1.5\", got %q", round.Outputs[0].Amount)
		}
	})

	t.Run("SKA with CoinType and decimal-string amount", func(t *testing.T) {
		cmd := &SpendOutputsCmd{
			Account:           "default",
			PreviousOutpoints: []string{"abc:0"},
			Outputs: []AddressAmountPair{
				{Address: "addr1", Amount: "50000000000.123"},
			},
			CoinType: uint8Ptr(1),
		}
		got, err := json.Marshal(cmd)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		var round SpendOutputsCmd
		if err := json.Unmarshal(got, &round); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if round.CoinType == nil || *round.CoinType != 1 {
			t.Errorf("CoinType: want 1, got %v", round.CoinType)
		}
		if round.Outputs[0].Amount != "50000000000.123" {
			t.Errorf("Amount: want \"50000000000.123\", got %q", round.Outputs[0].Amount)
		}
	})

	t.Run("Unmarshal rejects legacy JSON-number amount", func(t *testing.T) {
		// Post-unification, Amount is a string. JSON numbers must fail to
		// unmarshal — this is the breaking change documented on the type.
		varJSON := `{"Account":"default","PreviousOutpoints":["abc:0"],"Outputs":[{"address":"addr1","amount":2.0}]}`
		var varCmd SpendOutputsCmd
		if err := json.Unmarshal([]byte(varJSON), &varCmd); err == nil {
			t.Errorf("Unmarshal of float Amount: want error (string field), got nil")
		}
	})

	roundtripDeepEqual := func(t *testing.T, cmd *SpendOutputsCmd) {
		t.Helper()
		bs, err := json.Marshal(cmd)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		var got SpendOutputsCmd
		if err := json.Unmarshal(bs, &got); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if !reflect.DeepEqual(&got, cmd) {
			t.Errorf("round-trip mismatch: got %+v, want %+v", &got, cmd)
		}
	}
	t.Run("DeepEqual round-trip VAR", func(t *testing.T) {
		roundtripDeepEqual(t, &SpendOutputsCmd{
			Account:           "default",
			PreviousOutpoints: []string{"abc:0"},
			Outputs:           []AddressAmountPair{{Address: "addr1", Amount: "1.5"}},
		})
	})
	t.Run("DeepEqual round-trip SKA", func(t *testing.T) {
		roundtripDeepEqual(t, &SpendOutputsCmd{
			Account:           "default",
			PreviousOutpoints: []string{"abc:0"},
			Outputs:           []AddressAmountPair{{Address: "addr1", Amount: "1.5"}},
			CoinType:          uint8Ptr(1),
		})
	})
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}

func uint8Ptr(u uint8) *uint8 {
	return &u
}

func subFeeBoolPtr(b bool) *bool {
	return &b
}
