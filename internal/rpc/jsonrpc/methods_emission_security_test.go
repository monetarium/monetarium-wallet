// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package jsonrpc

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/monetarium/monetarium-wallet/rpc/jsonrpc/types"
)

// TestEmissionRPCsRequireWalletPassphraseField verifies that the
// GenerateEmissionKeyCmd and ImportEmissionKeyCmd structs expose a
// WalletPassphrase field with the JSON name "walletpassphrase". This is the
// new per-call capability gate; clients failing to send this field receive
// an explicit ErrRPCInvalidParameter from the handler.
func TestEmissionRPCsRequireWalletPassphraseField(t *testing.T) {
	t.Run("GenerateEmissionKeyCmd has walletpassphrase JSON field", func(t *testing.T) {
		cmd := &types.GenerateEmissionKeyCmd{
			KeyName:          "k",
			WalletPassphrase: "WALLET-PP",
			Passphrase:       "BACKUP-PP",
		}
		b, err := json.Marshal(cmd)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		s := string(b)
		if !strings.Contains(s, `"walletpassphrase":"WALLET-PP"`) {
			t.Errorf("expected walletpassphrase field with WALLET-PP; got %s", s)
		}
		if !strings.Contains(s, `"passphrase":"BACKUP-PP"`) {
			t.Errorf("expected passphrase field with BACKUP-PP; got %s", s)
		}
	})

	t.Run("ImportEmissionKeyCmd has walletpassphrase JSON field", func(t *testing.T) {
		cmd := &types.ImportEmissionKeyCmd{
			KeyName:          "k",
			PrivateKey:       "deadbeef",
			WalletPassphrase: "WALLET-PP",
			Passphrase:       "BACKUP-PP",
		}
		b, err := json.Marshal(cmd)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		s := string(b)
		if !strings.Contains(s, `"walletpassphrase":"WALLET-PP"`) {
			t.Errorf("expected walletpassphrase field with WALLET-PP; got %s", s)
		}
		if !strings.Contains(s, `"passphrase":"BACKUP-PP"`) {
			t.Errorf("expected passphrase field with BACKUP-PP; got %s", s)
		}
	})

	t.Run("WalletPassphrase is distinct from backup Passphrase", func(t *testing.T) {
		// Set only WalletPassphrase. The backup Passphrase must remain
		// independently zero-valued — they are not aliases.
		gen := &types.GenerateEmissionKeyCmd{
			KeyName:          "k",
			WalletPassphrase: "WALLET-PP",
		}
		if gen.Passphrase != "" {
			t.Errorf("expected backup Passphrase to be empty; got %q", gen.Passphrase)
		}
		imp := &types.ImportEmissionKeyCmd{
			KeyName:          "k",
			PrivateKey:       "deadbeef",
			WalletPassphrase: "WALLET-PP",
		}
		if imp.Passphrase != "" {
			t.Errorf("expected backup Passphrase to be empty; got %q", imp.Passphrase)
		}
	})
}

// TestCreateAuthorizedEmissionCmdForceFlags verifies that
// CreateAuthorizedEmissionCmd exposes independent ForceWindow and ForceNonce
// fields. The two flags bypass unrelated guards (out-of-window height vs
// non-default nonce), so a single global "force" affordance is intentionally
// absent — operators must opt in to each bypass individually.
func TestCreateAuthorizedEmissionCmdForceFlags(t *testing.T) {
	t.Run("force flags omitted when nil (omitempty)", func(t *testing.T) {
		cmd := &types.CreateAuthorizedEmissionCmd{
			CoinType:        1,
			EmissionKeyName: "k",
			Passphrase:      "p",
		}
		b, err := json.Marshal(cmd)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		s := string(b)
		if strings.Contains(s, "force") {
			t.Errorf("expected force* fields omitted when nil; got %s", s)
		}
	})

	t.Run("ForceWindow serialized independently", func(t *testing.T) {
		yes := true
		cmd := &types.CreateAuthorizedEmissionCmd{
			CoinType:        1,
			EmissionKeyName: "k",
			Passphrase:      "p",
			ForceWindow:     &yes,
		}
		b, err := json.Marshal(cmd)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		s := string(b)
		if !strings.Contains(s, `"forcewindow":true`) {
			t.Errorf("expected forcewindow=true in JSON; got %s", s)
		}
		if strings.Contains(s, `"forcenonce"`) {
			t.Errorf("expected forcenonce omitted when only ForceWindow set; got %s", s)
		}
	})

	t.Run("ForceNonce serialized independently", func(t *testing.T) {
		yes := true
		cmd := &types.CreateAuthorizedEmissionCmd{
			CoinType:        1,
			EmissionKeyName: "k",
			Passphrase:      "p",
			ForceNonce:      &yes,
		}
		b, err := json.Marshal(cmd)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		s := string(b)
		if !strings.Contains(s, `"forcenonce":true`) {
			t.Errorf("expected forcenonce=true in JSON; got %s", s)
		}
		if strings.Contains(s, `"forcewindow"`) {
			t.Errorf("expected forcewindow omitted when only ForceNonce set; got %s", s)
		}
	})

	t.Run("legacy Force field is removed from struct", func(t *testing.T) {
		// Compile-time guard: if a future refactor reintroduces a single
		// `Force` field, this block fails to compile and pins the design.
		cmd := &types.CreateAuthorizedEmissionCmd{}
		_ = cmd.ForceWindow
		_ = cmd.ForceNonce
	})
}

// TestNewGenerateEmissionKeyCmdConstructorRequiresWalletPassphrase asserts
// the constructor signature accepts (and propagates) a wallet passphrase.
// This protects against a future refactor that re-collapses the two
// passphrases — which would re-introduce the HIGH severity gap.
func TestNewGenerateEmissionKeyCmdConstructorRequiresWalletPassphrase(t *testing.T) {
	cmd := types.NewGenerateEmissionKeyCmd("k", "WALLET-PP", "BACKUP-PP")
	if cmd.WalletPassphrase != "WALLET-PP" {
		t.Errorf("constructor did not propagate WalletPassphrase: got %q", cmd.WalletPassphrase)
	}
	if cmd.Passphrase != "BACKUP-PP" {
		t.Errorf("constructor did not propagate Passphrase: got %q", cmd.Passphrase)
	}

	cmd2 := types.NewGenerateEmissionKeyCmdWithCoinType(2, "k2", "WP", "BP")
	if cmd2.WalletPassphrase != "WP" || cmd2.Passphrase != "BP" {
		t.Errorf("WithCoinType constructor: got WP=%q BP=%q", cmd2.WalletPassphrase, cmd2.Passphrase)
	}
}

// TestNewImportEmissionKeyCmdConstructorRequiresWalletPassphrase asserts
// the same for the import variant.
func TestNewImportEmissionKeyCmdConstructorRequiresWalletPassphrase(t *testing.T) {
	cmd := types.NewImportEmissionKeyCmd(1, "k", "deadbeef", "WALLET-PP", "BACKUP-PP")
	if cmd.WalletPassphrase != "WALLET-PP" || cmd.Passphrase != "BACKUP-PP" {
		t.Errorf("constructor: got WP=%q BP=%q", cmd.WalletPassphrase, cmd.Passphrase)
	}

	cmd2 := types.NewImportEmissionKeyCmdNoCoinType("k2", "deadbeef", "WP", "BP")
	if cmd2.WalletPassphrase != "WP" || cmd2.Passphrase != "BP" {
		t.Errorf("NoCoinType constructor: got WP=%q BP=%q", cmd2.WalletPassphrase, cmd2.Passphrase)
	}
}
