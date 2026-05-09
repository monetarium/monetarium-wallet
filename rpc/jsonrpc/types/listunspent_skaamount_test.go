// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestListUnspentResultVARShape asserts that for a VAR UTXO the JSON encoder
// emits `amount` as a decimal coin string. The dual-field skaamount is gone:
// after the unification, both VAR and SKA share a single string-typed Amount
// field.
func TestListUnspentResultVARShape(t *testing.T) {
	r := &ListUnspentResult{
		TxID:     "deadbeef",
		Amount:   "1.5",
		CoinType: 0,
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, `"amount":"1.5"`) {
		t.Errorf("expected amount=\"1.5\" in JSON; got %s", s)
	}
	if strings.Contains(s, "skaamount") {
		t.Errorf("legacy skaamount field must not appear in JSON; got %s", s)
	}
}

// TestListUnspentResultSKAShape asserts that for an SKA UTXO the JSON encoder
// emits a populated decimal-string `amount` field that preserves SKA precision.
func TestListUnspentResultSKAShape(t *testing.T) {
	r := &ListUnspentResult{
		TxID:     "deadbeef",
		Amount:   "12345.6789",
		CoinType: 1,
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, `"amount":"12345.6789"`) {
		t.Errorf("expected amount=\"12345.6789\" in JSON; got %s", s)
	}
	if strings.Contains(s, "skaamount") {
		t.Errorf("legacy skaamount field must not appear in JSON; got %s", s)
	}
}

// TestListUnspentResultRoundTrip asserts that the unified-string shape
// survives an unmarshal/marshal cycle without precision loss.
func TestListUnspentResultRoundTrip(t *testing.T) {
	src := &ListUnspentResult{
		TxID:     "abc",
		Amount:   "1000000000000000000", // would lose precision as float64
		CoinType: 2,
	}
	b, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var dst ListUnspentResult
	if err := json.Unmarshal(b, &dst); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if dst.Amount != src.Amount {
		t.Errorf("Amount round-trip: got %q, want %q", dst.Amount, src.Amount)
	}
}
