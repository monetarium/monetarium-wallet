// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestCreateAuthorizedEmissionResultWarningWireFormat pins the additive
// backwards-compatibility contract for the Warning field added in v1.3.7
// (see the M3 finding in the 2026-05-15 code review).
//
// The field is operator-visible: it surfaces a local one-shot-guard persistence
// failure on the forceNonce path so scripted callers can detect the broken
// guard and avoid auto-retrying without forcenonce=true. On the fresh-nonce
// path the handler refuses outright rather than returning a tx with a warning.
//
// The omitempty contract here matters — pre-v1.3.7 clients deserialize the
// result via field-name matching and must not see a spurious "warning": ""
// pair when no warning is set.
func TestCreateAuthorizedEmissionResultWarningWireFormat(t *testing.T) {
	t.Run("Warning omitted when empty", func(t *testing.T) {
		r := CreateAuthorizedEmissionResult{
			Transaction:     "deadbeef",
			TransactionHash: "abc123",
			Nonce:           7,
			TotalAmount:     "100",
			CoinType:        1,
			// Warning intentionally unset.
		}
		b, err := json.Marshal(&r)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if strings.Contains(string(b), "warning") {
			t.Fatalf("empty Warning must be omitted from wire format; got %s", string(b))
		}
	})

	t.Run("Warning serialized when set", func(t *testing.T) {
		r := CreateAuthorizedEmissionResult{
			Transaction:     "deadbeef",
			TransactionHash: "abc123",
			Nonce:           7,
			TotalAmount:     "100",
			CoinType:        1,
			Warning:         "local one-shot guard persistence failed",
		}
		b, err := json.Marshal(&r)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("round-trip unmarshal: %v", err)
		}
		w, ok := got["warning"].(string)
		if !ok {
			t.Fatalf("warning field missing or not a string in %s", string(b))
		}
		if !strings.Contains(w, "local one-shot guard") {
			t.Fatalf("warning content unexpected: %q", w)
		}
	})

	t.Run("Older client wire format (no warning) deserializes cleanly", func(t *testing.T) {
		// A v1.3.6 server response (no warning field) must deserialize into the
		// new struct with Warning == "". This is the additive-field contract.
		older := `{"transaction":"deadbeef","transactionhash":"abc123","nonce":7,"totalamount":"100","cointype":1}`
		var r CreateAuthorizedEmissionResult
		if err := json.Unmarshal([]byte(older), &r); err != nil {
			t.Fatalf("unmarshal older wire format: %v", err)
		}
		if r.Warning != "" {
			t.Fatalf("Warning must default to empty string; got %q", r.Warning)
		}
		if r.Transaction != "deadbeef" || r.Nonce != 7 {
			t.Fatalf("other fields decoded incorrectly: %+v", r)
		}
	})
}
