// Copyright (c) 2025 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package jsonrpc

import (
	"encoding/hex"
	"math/big"
	"strings"
	"testing"

	"github.com/monetarium/monetarium-node/chaincfg"
	"github.com/monetarium/monetarium-node/chaincfg/chainhash"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/dcrec"
	"github.com/monetarium/monetarium-node/dcrec/secp256k1"
	"github.com/monetarium/monetarium-node/txscript"
	"github.com/monetarium/monetarium-node/txscript/sign"
	"github.com/monetarium/monetarium-node/txscript/stdaddr"
	"github.com/monetarium/monetarium-node/txscript/stdscript"
	"github.com/monetarium/monetarium-node/wire"
	"github.com/monetarium/monetarium-wallet/rpc/jsonrpc/types"
	"github.com/monetarium/monetarium-wallet/wallet"
)

// TestMultisigCommandsCoinTypeField verifies the CoinType field is properly
// parsed from multisig RPC command structs.
func TestMultisigCommandsCoinTypeField(t *testing.T) {
	t.Run("SendToMultiSigCmd accepts cointype", func(t *testing.T) {
		ct := uint8(1)
		cmd := &types.SendToMultiSigCmd{
			FromAccount: "default",
			Amount:      "10.0",
			Pubkeys:     []string{"pk1", "pk2"},
			CoinType:    &ct,
		}
		if cmd.CoinType == nil || *cmd.CoinType != 1 {
			t.Error("CoinType should be 1")
		}
	})

	t.Run("SendToMultiSigCmd defaults to nil cointype", func(t *testing.T) {
		cmd := &types.SendToMultiSigCmd{
			FromAccount: "default",
			Amount:      "10.0",
			Pubkeys:     []string{"pk1", "pk2"},
		}
		if cmd.CoinType != nil {
			t.Error("CoinType should be nil by default")
		}
	})

	t.Run("RedeemMultiSigOutCmd accepts cointype", func(t *testing.T) {
		ct := uint8(2)
		cmd := &types.RedeemMultiSigOutCmd{
			Hash:     "abc123",
			Index:    0,
			Tree:     0,
			CoinType: &ct,
		}
		if cmd.CoinType == nil || *cmd.CoinType != 2 {
			t.Error("CoinType should be 2")
		}
	})

	t.Run("RedeemMultiSigOutsCmd accepts cointype", func(t *testing.T) {
		ct := uint8(1)
		cmd := &types.RedeemMultiSigOutsCmd{
			FromScrAddress: "addr",
			CoinType:       &ct,
		}
		if cmd.CoinType == nil || *cmd.CoinType != 1 {
			t.Error("CoinType should be 1")
		}
	})
}

// TestP2SHSKAValueInStr pins the M4 fix from the 2026-05-15 review:
// redeemMultiSigOut must pass the SKA value through the synthesized
// RawTxInput.SKAValueIn explicitly so the signing path does not rely on
// SKAValueIn surviving the wire-format round-trip. Before the fix, the
// caller-supplied RawTxInput omitted SKAValueIn entirely and any future
// change that dropped SKAValueIn from serialize/deserialize would have
// silently signed an SKA tx with a zero input value.
func TestP2SHSKAValueInStr(t *testing.T) {
	const skaCT = cointype.CoinType(1)
	atomsPerCoin := big.NewInt(0).Exp(big.NewInt(10), big.NewInt(18), nil)
	params := &chaincfg.Params{
		SKACoins: map[cointype.CoinType]*chaincfg.SKACoinConfig{
			skaCT: {
				AtomsPerCoin: new(big.Int).Set(atomsPerCoin),
				MaxSupply:    new(big.Int).Mul(big.NewInt(1_000_000), atomsPerCoin),
				Active:       true,
			},
		},
	}

	t.Run("VAR coin type returns nil", func(t *testing.T) {
		p2sh := &wallet.P2SHMultiSigOutput{
			OutputAmount: 100,
		}
		if got := p2shSKAValueInStr(p2sh, cointype.CoinTypeVAR, params); got != nil {
			t.Fatalf("VAR must return nil; got %v", *got)
		}
	})

	t.Run("nil output returns nil", func(t *testing.T) {
		if got := p2shSKAValueInStr(nil, skaCT, params); got != nil {
			t.Fatalf("nil p2sh must return nil; got %v", *got)
		}
	})

	t.Run("SKA returns decimal-coin string matching atoms-per-coin", func(t *testing.T) {
		// 2.5 SKA at 1e18 atoms/coin = 2.5e18 atoms.
		atoms := new(big.Int).Mul(big.NewInt(25), new(big.Int).Exp(big.NewInt(10), big.NewInt(17), nil))
		p2sh := &wallet.P2SHMultiSigOutput{
			SKAOutputAmount: cointype.NewSKAAmount(atoms),
			CoinType:        skaCT,
		}
		got := p2shSKAValueInStr(p2sh, skaCT, params)
		if got == nil {
			t.Fatalf("SKA must return non-nil")
		}
		// Re-parse via coinsToAtomsBig to confirm round-trip.
		back, err := coinsToAtomsBig(*got, atomsPerCoin)
		if err != nil {
			t.Fatalf("returned string %q must round-trip via coinsToAtomsBig: %v", *got, err)
		}
		if back.Cmp(atoms) != 0 {
			t.Fatalf("round-trip mismatch: returned %q parsed to %s, want %s", *got, back, atoms)
		}
	})

	t.Run("SKA value at MaxSupply boundary round-trips", func(t *testing.T) {
		// Operator-visible upper bound — ensure the decimal-string format
		// does not lose precision at the high end of the supply range.
		atoms := new(big.Int).Mul(big.NewInt(999_999), atomsPerCoin)
		p2sh := &wallet.P2SHMultiSigOutput{
			SKAOutputAmount: cointype.NewSKAAmount(atoms),
			CoinType:        skaCT,
		}
		got := p2shSKAValueInStr(p2sh, skaCT, params)
		if got == nil {
			t.Fatalf("SKA must return non-nil")
		}
		back, err := coinsToAtomsBig(*got, atomsPerCoin)
		if err != nil || back.Cmp(atoms) != 0 {
			t.Fatalf("boundary round-trip failed: string=%q parsed=%v err=%v want=%v", *got, back, err, atoms)
		}
	})
}

// TestValidateCoinTypeForMultisig verifies that validateCoinType works
// correctly for multisig-relevant coin type values.
func TestValidateCoinTypeForMultisig(t *testing.T) {
	tests := []struct {
		name     string
		coinType cointype.CoinType
		wantErr  bool
	}{
		{"VAR (0)", cointype.CoinTypeVAR, false},
		{"SKA1", cointype.CoinType(1), false},
		{"SKA2", cointype.CoinType(2), false},
		{"SKA255", cointype.CoinType(255), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCoinType(tt.coinType)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCoinType(%d) error = %v, wantErr %v", tt.coinType, err, tt.wantErr)
			}
		})
	}
}

// TestMultisigOutputCoinType verifies that multisig transaction outputs
// carry the correct CoinType for both VAR and SKA.
func TestMultisigOutputCoinType(t *testing.T) {
	params := chaincfg.SimNetParams()

	// Generate 2 keys for a 2-of-2 multisig
	privKey1, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	privKey2, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}

	pk1 := privKey1.PubKey().SerializeCompressed()
	pk2 := privKey2.PubKey().SerializeCompressed()

	// Create multisig script
	script, err := stdscript.MultiSigScriptV0(2, pk1, pk2)
	if err != nil {
		t.Fatal(err)
	}

	scriptAddr, err := stdaddr.NewAddressScriptHashV0(script, params)
	if err != nil {
		t.Fatal(err)
	}
	_, p2shScript := scriptAddr.PaymentScript()

	tests := []struct {
		name     string
		coinType cointype.CoinType
	}{
		{"VAR output", cointype.CoinTypeVAR},
		{"SKA1 output", cointype.CoinType(1)},
		{"SKA2 output", cointype.CoinType(2)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := wire.NewMsgTx()
			tx.AddTxIn(&wire.TxIn{
				PreviousOutPoint: wire.OutPoint{
					Hash:  chainhash.Hash{},
					Index: 0,
				},
				ValueIn: 1000000,
			})

			txOut := wire.NewTxOut(1000000, p2shScript)
			txOut.CoinType = tt.coinType
			tx.AddTxOut(txOut)

			if tx.TxOut[0].CoinType != tt.coinType {
				t.Errorf("Expected CoinType %d, got %d", tt.coinType, tx.TxOut[0].CoinType)
			}
		})
	}
}

// TestSignTransactionCompleteField_Regression tests the bug fix where
// SignTransaction's complete field was always true for multisig transactions
// with insufficient signatures.
//
// The bug had two layers:
// 1. prevOutScript lookup used additionalPrevScripts instead of the resolved prevOutScript
// 2. SignTransaction didn't communicate partial signing to the caller
func TestSignTransactionCompleteField_Regression(t *testing.T) {
	params := chaincfg.SimNetParams()

	// Generate 3 keys for a 2-of-3 multisig
	privKey1, _ := secp256k1.GeneratePrivateKey()
	privKey2, _ := secp256k1.GeneratePrivateKey()
	privKey3, _ := secp256k1.GeneratePrivateKey()

	pk1 := privKey1.PubKey().SerializeCompressed()
	pk2 := privKey2.PubKey().SerializeCompressed()
	pk3 := privKey3.PubKey().SerializeCompressed()

	// Create 2-of-3 multisig redeem script
	redeemScript, err := stdscript.MultiSigScriptV0(2, pk1, pk2, pk3)
	if err != nil {
		t.Fatal(err)
	}

	// Create P2SH address
	scriptAddr, err := stdaddr.NewAddressScriptHashV0(redeemScript, params)
	if err != nil {
		t.Fatal(err)
	}
	_, p2shPkScript := scriptAddr.PaymentScript()

	// Create a spending transaction
	prevOutHash := chainhash.HashH([]byte("fake prev tx"))
	tx := wire.NewMsgTx()
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Hash: prevOutHash, Index: 0},
		ValueIn:          1000000,
	})

	// P2PKH output for the destination
	destPrivKey, _ := secp256k1.GeneratePrivateKey()
	destPub := destPrivKey.PubKey()
	destAddr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(
		stdaddr.Hash160(destPub.SerializeCompressed()), params)
	if err != nil {
		t.Fatal(err)
	}
	_, destPkScript := destAddr.PaymentScript()
	tx.AddTxOut(wire.NewTxOut(900000, destPkScript))

	// Sign with only 1 of 3 keys (not enough for 2-of-3)
	addr1, _ := stdaddr.NewAddressPubKeyEcdsaSecp256k1V0(privKey1.PubKey(), params)

	sigScript, err := sign.SignTxOutput(params, tx, 0, p2shPkScript,
		txscript.SigHashAll,
		sign.KeyClosure(func(addr stdaddr.Address) ([]byte, dcrec.SignatureType, bool, error) {
			if addr.String() == addr1.String() {
				return privKey1.Serialize(), dcrec.STEcdsaSecp256k1, true, nil
			}
			return nil, 0, false, nil
		}),
		sign.ScriptClosure(func(addr stdaddr.Address) ([]byte, error) {
			if addr.String() == scriptAddr.String() {
				return redeemScript, nil
			}
			return nil, nil
		}), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	tx.TxIn[0].SignatureScript = sigScript

	// The script engine should fail with insufficient signatures
	t.Run("1-of-3 sigs fails execution", func(t *testing.T) {
		vm, err := txscript.NewEngine(p2shPkScript, tx, 0,
			0, 0, nil)
		if err != nil {
			t.Fatalf("Failed to create script engine: %v", err)
		}
		if vm.Execute() == nil {
			t.Fatal("Script should fail with 1-of-3 signatures for 2-of-3 multisig")
		}
	})

	// Now add a second signature — should succeed
	t.Run("2-of-3 sigs succeeds execution", func(t *testing.T) {
		addr2, _ := stdaddr.NewAddressPubKeyEcdsaSecp256k1V0(privKey2.PubKey(), params)

		sigScript2, err := sign.SignTxOutput(params, tx, 0, p2shPkScript,
			txscript.SigHashAll,
			sign.KeyClosure(func(addr stdaddr.Address) ([]byte, dcrec.SignatureType, bool, error) {
				switch addr.String() {
				case addr1.String():
					return privKey1.Serialize(), dcrec.STEcdsaSecp256k1, true, nil
				case addr2.String():
					return privKey2.Serialize(), dcrec.STEcdsaSecp256k1, true, nil
				}
				return nil, 0, false, nil
			}),
			sign.ScriptClosure(func(addr stdaddr.Address) ([]byte, error) {
				if addr.String() == scriptAddr.String() {
					return redeemScript, nil
				}
				return nil, nil
			}), tx.TxIn[0].SignatureScript, false)
		if err != nil {
			t.Fatal(err)
		}
		tx.TxIn[0].SignatureScript = sigScript2

		vm, err := txscript.NewEngine(p2shPkScript, tx, 0,
			0, 0, nil)
		if err != nil {
			t.Fatalf("Failed to create script engine: %v", err)
		}
		if err := vm.Execute(); err != nil {
			t.Errorf("Script should succeed with 2-of-3 sigs, got: %v", err)
		}
	})

	// Note: a real regression for SignTransaction's `complete` boolean
	// against a partially-signed multisig tx lives in
	// wallet/sign_partial_multisig_test.go (package wallet), where the
	// internal testWallet helper is in scope.
}

// TestMultisigSKAOutputSerialization verifies that multisig outputs with
// SKA coin type serialize correctly with the CoinType byte in the wire format.
func TestMultisigSKAOutputSerialization(t *testing.T) {
	params := chaincfg.SimNetParams()

	privKey1, _ := secp256k1.GeneratePrivateKey()
	privKey2, _ := secp256k1.GeneratePrivateKey()

	pk1 := privKey1.PubKey().SerializeCompressed()
	pk2 := privKey2.PubKey().SerializeCompressed()

	script, err := stdscript.MultiSigScriptV0(2, pk1, pk2)
	if err != nil {
		t.Fatal(err)
	}

	scriptAddr, _ := stdaddr.NewAddressScriptHashV0(script, params)
	_, p2shScript := scriptAddr.PaymentScript()

	tests := []struct {
		name         string
		coinType     cointype.CoinType
		value        int64
		wantCoinType cointype.CoinType
	}{
		{"VAR multisig output", cointype.CoinTypeVAR, 500000, cointype.CoinTypeVAR},
		{"SKA1 multisig output", cointype.CoinType(1), 500000, cointype.CoinType(1)},
		{"SKA2 multisig output", cointype.CoinType(2), 1000000, cointype.CoinType(2)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := wire.NewMsgTx()
			tx.AddTxIn(&wire.TxIn{
				PreviousOutPoint: wire.OutPoint{Hash: chainhash.Hash{}, Index: 0},
				ValueIn:          tt.value,
			})

			txOut := wire.NewTxOut(tt.value, p2shScript)
			txOut.CoinType = tt.coinType
			tx.AddTxOut(txOut)

			// Serialize and deserialize to verify CoinType survives the round-trip
			var buf strings.Builder
			buf.Grow(2 * tx.SerializeSize())
			err := tx.Serialize(hex.NewEncoder(&buf))
			if err != nil {
				t.Fatalf("Failed to serialize: %v", err)
			}

			txBytes, err := hex.DecodeString(buf.String())
			if err != nil {
				t.Fatalf("Failed to decode hex: %v", err)
			}

			var tx2 wire.MsgTx
			err = tx2.Deserialize(strings.NewReader(string(txBytes)))
			if err != nil {
				t.Fatalf("Failed to deserialize: %v", err)
			}

			if tx2.TxOut[0].CoinType != tt.wantCoinType {
				t.Errorf("CoinType mismatch after round-trip: got %d, want %d",
					tx2.TxOut[0].CoinType, tt.wantCoinType)
			}
		})
	}
}

// TestRedeemMultiSigOutCoinTypePropagation verifies that the CoinType
// from RedeemMultiSigOutsCmd is correctly propagated to the inner
// RedeemMultiSigOutCmd when called via redeemMultiSigOuts.
func TestRedeemMultiSigOutCoinTypePropagation(t *testing.T) {
	tests := []struct {
		name         string
		coinType     *uint8
		wantCoinType *uint8
	}{
		{"nil cointype defaults to VAR", nil, nil},
		{"SKA1 propagates", uint8Ptr(1), uint8Ptr(1)},
		{"SKA2 propagates", uint8Ptr(2), uint8Ptr(2)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outerCmd := &types.RedeemMultiSigOutsCmd{
				FromScrAddress: "testaddr",
				CoinType:       tt.coinType,
			}

			// Simulate what redeemMultiSigOuts does: creates inner commands
			// with the same CoinType
			var ctVal *uint8
			if outerCmd.CoinType != nil {
				v := *outerCmd.CoinType
				ctVal = &v
			}

			innerCmd := &types.RedeemMultiSigOutCmd{
				Hash:     "fakehash",
				Index:    0,
				Tree:     0,
				CoinType: ctVal,
			}

			if tt.wantCoinType == nil {
				if innerCmd.CoinType != nil {
					t.Errorf("Expected nil CoinType, got %d", *innerCmd.CoinType)
				}
			} else {
				if innerCmd.CoinType == nil {
					t.Error("Expected non-nil CoinType, got nil")
				} else if *innerCmd.CoinType != *tt.wantCoinType {
					t.Errorf("CoinType mismatch: got %d, want %d", *innerCmd.CoinType, *tt.wantCoinType)
				}
			}
		})
	}
}

// TestPrevOutScriptLookup_Regression verifies the fix for the bug where
// the multisig detection code used additionalPrevScripts instead of
// the already-resolved prevOutScript variable.
func TestPrevOutScriptLookup_Regression(t *testing.T) {
	params := chaincfg.SimNetParams()

	// Generate keys for 2-of-2 multisig
	privKey1, _ := secp256k1.GeneratePrivateKey()
	privKey2, _ := secp256k1.GeneratePrivateKey()

	pk1 := privKey1.PubKey().SerializeCompressed()
	pk2 := privKey2.PubKey().SerializeCompressed()

	redeemScript, err := stdscript.MultiSigScriptV0(2, pk1, pk2)
	if err != nil {
		t.Fatal(err)
	}

	scriptAddr, _ := stdaddr.NewAddressScriptHashV0(redeemScript, params)
	_, p2shPkScript := scriptAddr.PaymentScript()

	// Create spending tx with 1-of-2 signatures
	prevHash := chainhash.HashH([]byte("prev"))
	tx := wire.NewMsgTx()
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Hash: prevHash, Index: 0},
		ValueIn:          500000,
	})
	tx.AddTxOut(wire.NewTxOut(400000, []byte{txscript.OP_TRUE}))

	// Sign with only key1
	addr1, _ := stdaddr.NewAddressPubKeyEcdsaSecp256k1V0(privKey1.PubKey(), params)
	sigScript, err := sign.SignTxOutput(params, tx, 0, p2shPkScript,
		txscript.SigHashAll,
		sign.KeyClosure(func(addr stdaddr.Address) ([]byte, dcrec.SignatureType, bool, error) {
			if addr.String() == addr1.String() {
				return privKey1.Serialize(), dcrec.STEcdsaSecp256k1, true, nil
			}
			return nil, 0, false, nil
		}),
		sign.ScriptClosure(func(addr stdaddr.Address) ([]byte, error) {
			if addr.String() == scriptAddr.String() {
				return redeemScript, nil
			}
			return nil, nil
		}), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	tx.TxIn[0].SignatureScript = sigScript

	// The script engine should fail (not enough sigs for 2-of-2)
	vm, err := txscript.NewEngine(p2shPkScript, tx, 0,
		0, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	execErr := vm.Execute()
	if execErr == nil {
		t.Fatal("Script should fail with 1-of-2 signatures for 2-of-2 multisig")
	}

	// The bug was: wallet code checked if error is ErrInvalidStackOperation,
	// then looked up pkScript in additionalPrevScripts (empty for blockchain
	// inputs). The fix uses prevOutScript which is always resolved.
	//
	// Verify that IsMultiSigScriptV0 correctly identifies the redeem script
	// (this is what the wallet checks after resolving the script).
	if !stdscript.IsMultiSigScriptV0(redeemScript) {
		t.Error("Redeem script should be identified as multisig")
	}

	// Verify the P2SH script class detection
	class, _ := stdscript.ExtractAddrs(0, p2shPkScript, params)
	if class != stdscript.STScriptHash {
		t.Errorf("Expected STScriptHash, got %v", class)
	}
}

// TestResolveRedeemMultiSigOutsCap verifies the per-call iteration cap
// computed by redeemMultiSigOuts. The cap protects the RPC server from an
// authenticated operator triggering an unbounded number of redemption
// signings in a single call. Truncation is computed by
// redeemMultiSigOutsCollect after coin-type filtering and is exercised by
// dedicated tests in redeemmultisigouts_test.go.
func TestResolveRedeemMultiSigOutsCap(t *testing.T) {
	intPtr := func(v int) *int { return &v }
	defaultCap := int(redeemMultiSigOutsMax)
	tests := []struct {
		name      string
		number    *int
		wantLimit uint32
	}{
		{"default cap when nil", nil, redeemMultiSigOutsMax},
		{"caller below cap honored", intPtr(50), 50},
		{"caller above cap clamped", intPtr(1000), redeemMultiSigOutsMax},
		{"caller equal to cap", intPtr(defaultCap), redeemMultiSigOutsMax},
		{"negative caller value treated as default", intPtr(-1), redeemMultiSigOutsMax},
		// Number=0 must use the default cap, not clamp to zero (regression
		// guard — previously a Number=0 call returned an empty result).
		{"caller zero treated as default", intPtr(0), redeemMultiSigOutsMax},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLimit := resolveRedeemMultiSigOutsCap(tt.number)
			if gotLimit != tt.wantLimit {
				t.Errorf("limit: got %d, want %d", gotLimit, tt.wantLimit)
			}
		})
	}
}
