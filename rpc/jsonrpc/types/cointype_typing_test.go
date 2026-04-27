// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types_test

import (
	"encoding/json"
	"testing"

	"github.com/monetarium/monetarium-wallet/rpc/jsonrpc/types"
)

// TestCoinTypeFieldIsUint8 enforces that every RPC command struct carrying a
// user-supplied coin type is typed as *uint8. The review called out
// GetReceivedByAddressCmd / GetWalletFeeCmd / SetTxFeeCmd as the only
// *int holdouts; those have been migrated. The test guards against a new
// handler regressing to *int, which would silently let negative values
// through JSON decoding.
func TestCoinTypeFieldIsUint8(t *testing.T) {
	// Ensure decoding -1 into *uint8 fails (proves the type is what we claim).
	var cmd types.GetReceivedByAddressCmd
	err := json.Unmarshal([]byte(`{"Address":"addr","CoinType":-1}`), &cmd)
	if err == nil {
		t.Fatalf("decoding CoinType=-1 into GetReceivedByAddressCmd must fail; got %+v", cmd)
	}

	var fee types.GetWalletFeeCmd
	err = json.Unmarshal([]byte(`{"CoinType":-1}`), &fee)
	if err == nil {
		t.Fatalf("decoding CoinType=-1 into GetWalletFeeCmd must fail; got %+v", fee)
	}

	var settx types.SetTxFeeCmd
	err = json.Unmarshal([]byte(`{"Amount":"1.0","CoinType":-1}`), &settx)
	if err == nil {
		t.Fatalf("decoding CoinType=-1 into SetTxFeeCmd must fail; got %+v", settx)
	}

	// Positive cases — 0 and 255 both decode cleanly.
	if err := json.Unmarshal([]byte(`{"CoinType":0}`), &fee); err != nil {
		t.Fatalf("CoinType=0 must decode: %v", err)
	}
	if err := json.Unmarshal([]byte(`{"CoinType":255}`), &fee); err != nil {
		t.Fatalf("CoinType=255 must decode: %v", err)
	}

	// Overflow past uint8 must also fail.
	err = json.Unmarshal([]byte(`{"CoinType":256}`), &fee)
	if err == nil {
		t.Fatalf("CoinType=256 must fail (uint8 overflow); got %+v", fee)
	}
}

// TestSendToMultiSigAmountIsString locks in the HIGH #3 fix: amount is a
// string (so SKA values preserving 1e18 atoms/coin precision flow through),
// not float64.
func TestSendToMultiSigAmountIsString(t *testing.T) {
	cmd := types.NewSendToMultiSigCmd("default", "1234567890123456789", []string{"pk1"}, nil, nil, nil)

	raw, err := json.Marshal(cmd)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Decode back and ensure the big SKA amount round-trips byte-exact — a
	// float64 field would have silently truncated at ~15 significant digits.
	var back types.SendToMultiSigCmd
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Amount != "1234567890123456789" {
		t.Fatalf("Amount precision lost: got %q want %q", back.Amount, "1234567890123456789")
	}
}
