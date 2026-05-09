// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package jsonrpc

import (
	"testing"

	"github.com/monetarium/monetarium-wallet/rpc/jsonrpc/types"
)

// TestSweepAccountCmdAcceptsCoinType pins the MED-5 fix from the 2026-05-04
// review: SweepAccountCmd must carry an optional CoinType so SKA sweeps can
// be expressed via JSON-RPC. Default (nil) preserves the prior VAR-only
// behavior; setting it routes the handler to
// NewUnsignedSweepTransactionForCoinType for the requested SKA cointype.
func TestSweepAccountCmdAcceptsCoinType(t *testing.T) {
	t.Run("default cointype is nil", func(t *testing.T) {
		cmd := &types.SweepAccountCmd{
			SourceAccount:      "default",
			DestinationAddress: "SsWKp7wtdTZYabYFYSc9cnxhwFEjA5g4pFc",
		}
		if cmd.CoinType != nil {
			t.Errorf("default CoinType = %v, want nil", *cmd.CoinType)
		}
	})

	t.Run("SKA cointype is preserved", func(t *testing.T) {
		ct := uint8(1)
		cmd := &types.SweepAccountCmd{
			SourceAccount:      "default",
			DestinationAddress: "SsWKp7wtdTZYabYFYSc9cnxhwFEjA5g4pFc",
			CoinType:           &ct,
		}
		if cmd.CoinType == nil || *cmd.CoinType != 1 {
			t.Errorf("CoinType should be 1, got %v", cmd.CoinType)
		}
	})

	t.Run("SKA255 cointype is preserved", func(t *testing.T) {
		ct := uint8(255)
		cmd := &types.SweepAccountCmd{
			SourceAccount:      "default",
			DestinationAddress: "SsWKp7wtdTZYabYFYSc9cnxhwFEjA5g4pFc",
			CoinType:           &ct,
		}
		if cmd.CoinType == nil || *cmd.CoinType != 255 {
			t.Errorf("CoinType should be 255, got %v", cmd.CoinType)
		}
	})
}
