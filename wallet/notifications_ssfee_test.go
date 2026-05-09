// Copyright (c) 2024 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"testing"

	"github.com/monetarium/monetarium-node/blockchain/stake"
	"github.com/monetarium/monetarium-node/chaincfg/chainhash"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/txscript"
	"github.com/monetarium/monetarium-node/wire"
)

// TestTxTransactionTypeSSFee tests that SSFee transactions are correctly identified in notifications
func TestTxTransactionTypeSSFee(t *testing.T) {
	t.Parallel()

	// Create a valid SSFee transaction
	ssFeeTx := createMockSSFeeTx(cointype.CoinType(1), 3, 1000)

	// Test that it's identified as SSFee type
	txType := TxTransactionType(ssFeeTx)
	if txType != TransactionTypeSSFee {
		t.Errorf("TxTransactionType(ssFeeTx) = %v, want TransactionTypeSSFee", txType)
	}

	// Test that stake.IsSSFee also recognizes it
	if !stake.IsSSFee(ssFeeTx) {
		t.Error("stake.IsSSFee(ssFeeTx) = false, want true")
	}

	// Test that it's NOT identified as other types
	if stake.IsSStx(ssFeeTx) {
		t.Error("SSFee incorrectly identified as SStx")
	}
	if stake.IsSSGen(ssFeeTx) {
		t.Error("SSFee incorrectly identified as SSGen")
	}
	if stake.IsSSRtx(ssFeeTx) {
		t.Error("SSFee incorrectly identified as SSRtx")
	}
}

// TestSSFeeTransactionNotifications tests various notification scenarios for SSFee transactions
func TestSSFeeTransactionNotifications(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		createTx    func() *wire.MsgTx
		wantType    TransactionType
		description string
	}{
		{
			name: "SSFee SKA1 transaction",
			createTx: func() *wire.MsgTx {
				return createMockSSFeeTx(cointype.CoinType(1), 3, 1000)
			},
			wantType:    TransactionTypeSSFee,
			description: "SKA1 SSFee should be identified correctly",
		},
		{
			name: "SSFee SKA2 transaction",
			createTx: func() *wire.MsgTx {
				return createMockSSFeeTx(cointype.CoinType(2), 4, 2000) // Max 4 reward outputs
			},
			wantType:    TransactionTypeSSFee,
			description: "SKA2 SSFee should be identified correctly",
		},
		{
			name: "SSFee with single output",
			createTx: func() *wire.MsgTx {
				return createMockSSFeeTx(cointype.CoinType(3), 1, 5000)
			},
			wantType:    TransactionTypeSSFee,
			description: "SSFee with single voter output should be valid",
		},
		{
			name: "SSFee with max outputs",
			createTx: func() *wire.MsgTx {
				return createMockSSFeeTx(cointype.CoinType(1), 4, 500) // Max 4 reward outputs
			},
			wantType:    TransactionTypeSSFee,
			description: "SSFee with max voter outputs should be valid",
		},
		{
			name: "Regular transaction (not SSFee)",
			createTx: func() *wire.MsgTx {
				tx := wire.NewMsgTx()
				tx.Version = 1
				tx.AddTxIn(&wire.TxIn{
					PreviousOutPoint: wire.OutPoint{
						Hash:  chainhash.Hash{1, 2, 3},
						Index: 0,
					},
				})
				tx.AddTxOut(&wire.TxOut{
					Value:    1000,
					CoinType: cointype.CoinTypeVAR,
					PkScript: make([]byte, 25),
				})
				return tx
			},
			wantType:    TransactionTypeRegular,
			description: "Regular transaction should not be identified as SSFee",
		},
		{
			name: "Coinbase transaction",
			createTx: func() *wire.MsgTx {
				tx := wire.NewMsgTx()
				tx.Version = 1
				tx.AddTxIn(&wire.TxIn{
					PreviousOutPoint: wire.OutPoint{
						Hash:  chainhash.Hash{},
						Index: wire.MaxPrevOutIndex,
					},
					SignatureScript: []byte{0x00, 0x00}, // Coinbase signature
				})
				tx.AddTxOut(&wire.TxOut{
					Value:    50000000,
					CoinType: cointype.CoinTypeVAR,
				})
				return tx
			},
			wantType:    TransactionTypeCoinbase,
			description: "Coinbase should not be identified as SSFee",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			tx := test.createTx()
			txType := TxTransactionType(tx)

			if txType != test.wantType {
				t.Errorf("%s: got type %v, want %v", test.description, txType, test.wantType)
			}

			// Additional check for SSFee transactions
			if test.wantType == TransactionTypeSSFee {
				if !stake.IsSSFee(tx) {
					t.Errorf("%s: stake.IsSSFee() = false, want true", test.description)
				}
			} else {
				if stake.IsSSFee(tx) {
					t.Errorf("%s: stake.IsSSFee() = true, want false", test.description)
				}
			}
		})
	}
}

// TestSSFeeNotificationEdgeCases tests edge cases for SSFee transaction notifications
func TestSSFeeNotificationEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		createTx  func() *wire.MsgTx
		wantType  TransactionType
		wantValid bool
		reason    string
	}{
		{
			name: "SSFee with version 3",
			createTx: func() *wire.MsgTx {
				tx := createMockSSFeeTx(cointype.CoinType(1), 3, 1000)
				tx.Version = 3
				return tx
			},
			wantType:  TransactionTypeSSFee,
			wantValid: true,
			reason:    "Version 3 is valid for SSFee",
		},
		{
			name: "SSFee with version 4",
			createTx: func() *wire.MsgTx {
				tx := createMockSSFeeTx(cointype.CoinType(1), 3, 1000)
				tx.Version = 4
				return tx
			},
			wantType:  TransactionTypeSSFee,
			wantValid: true,
			reason:    "Version 4 should also be valid for SSFee",
		},
		{
			name: "Invalid SSFee with version 2",
			createTx: func() *wire.MsgTx {
				tx := createMockSSFeeTx(cointype.CoinType(1), 3, 1000)
				tx.Version = 2 // Too low for SSFee
				return tx
			},
			wantType:  TransactionTypeCoinbase, // Will fail SSFee check, then detected as coinbase due to null input
			wantValid: false,
			reason:    "Version 2 is too low for SSFee",
		},
		{
			name: "SSFee with zero value outputs",
			createTx: func() *wire.MsgTx {
				tx := wire.NewMsgTx()
				tx.Version = 3
				tx.AddTxIn(&wire.TxIn{
					PreviousOutPoint: wire.OutPoint{
						Hash:  chainhash.Hash{},
						Index: wire.MaxPrevOutIndex,
					},
				})
				// Add zero-value outputs (edge case)
				for i := 0; i < 3; i++ {
					tx.AddTxOut(&wire.TxOut{
						Value:    0,
						CoinType: cointype.CoinType(1),
						PkScript: make([]byte, 25),
					})
				}
				// Add proper SSFee OP_RETURN with SF marker
				opReturnScript := []byte{
					txscript.OP_RETURN, // 0x6a
					0x06,               // OP_DATA_6
					'S', 'F',           // "SF" marker
					0x00, 0x00, 0x00, 0x00, // height
				}
				tx.AddTxOut(&wire.TxOut{
					Value:    0,
					PkScript: opReturnScript,
					CoinType: cointype.CoinType(1), // Must match other outputs
				})
				return tx
			},
			wantType:  TransactionTypeSSFee,
			wantValid: true,
			reason:    "SSFee with zero-value outputs might occur in edge cases",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			tx := test.createTx()
			txType := TxTransactionType(tx)

			if txType != test.wantType {
				t.Errorf("%s: got type %v, want %v", test.reason, txType, test.wantType)
			}

			// Check if it's recognized as SSFee by stake package
			isSSFee := stake.IsSSFee(tx)
			if test.wantType == TransactionTypeSSFee && !isSSFee {
				t.Errorf("%s: stake.IsSSFee() = false, want true", test.reason)
			}

			// Validate if marked as valid
			if test.wantValid {
				err := stake.CheckSSFee(tx)
				if err != nil && test.wantType == TransactionTypeSSFee {
					t.Errorf("%s: CheckSSFee() failed: %v", test.reason, err)
				}
			}
		})
	}
}

// TestSSFeeDetermineTxType tests that DetermineTxType correctly identifies SSFee
func TestSSFeeDetermineTxType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		tx       *wire.MsgTx
		wantType stake.TxType
	}{
		{
			name:     "SSFee transaction",
			tx:       createMockSSFeeTx(cointype.CoinType(1), 3, 1000),
			wantType: stake.TxTypeSSFee,
		},
		{
			name: "Regular transaction",
			tx: &wire.MsgTx{
				Version: 1,
				TxIn:    []*wire.TxIn{{PreviousOutPoint: wire.OutPoint{Hash: chainhash.Hash{1}, Index: 0}}},
				TxOut:   []*wire.TxOut{{Value: 1000, CoinType: cointype.CoinTypeVAR}},
			},
			wantType: stake.TxTypeRegular,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			txType := stake.DetermineTxType(test.tx)
			if txType != test.wantType {
				t.Errorf("DetermineTxType() = %v, want %v", txType, test.wantType)
			}
		})
	}
}

// TestSSFeeOutputDiscovery tests that SSFee outputs are properly discovered
func TestSSFeeOutputDiscovery(t *testing.T) {
	t.Parallel()

	// Create a simple P2PKH script for testing
	pkScript := []byte{
		0x76, 0xa9, 0x14, // OP_DUP OP_HASH160 <push 20 bytes>
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
		0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
		0x10, 0x11, 0x12, 0x13, // 20 bytes of hash
		0x88, 0xac, // OP_EQUALVERIFY OP_CHECKSIG
	}

	// Create SSFee transaction with outputs to our address
	tx := wire.NewMsgTx()
	tx.Version = 3
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{},
			Index: wire.MaxPrevOutIndex,
		},
	})

	// Add multiple outputs to the same address (simulating multiple voters getting rewards)
	for i := 0; i < 3; i++ {
		tx.AddTxOut(&wire.TxOut{
			Value:    1000,
			CoinType: cointype.CoinType(1),
			PkScript: pkScript,
		})
	}

	// Add OP_RETURN with proper SF marker
	opReturnScript := []byte{
		txscript.OP_RETURN, // 0x6a
		0x06,               // OP_DATA_6
		'S', 'F',           // "SF" marker
		0x00, 0x00, 0x00, 0x00, // height
	}
	tx.AddTxOut(&wire.TxOut{
		Value:    0,
		PkScript: opReturnScript,
		CoinType: cointype.CoinType(1),
	})

	// Verify the transaction is recognized as SSFee
	if !stake.IsSSFee(tx) {
		t.Fatal("Transaction not recognized as SSFee")
	}

	// Check that outputs can be extracted
	outputCount := 0
	for i, out := range tx.TxOut {
		if i < len(tx.TxOut)-1 { // Skip OP_RETURN
			if out.CoinType != cointype.CoinType(1) {
				t.Errorf("Output %d has wrong coin type: %v", i, out.CoinType)
			}
			if out.Value != 1000 {
				t.Errorf("Output %d has wrong value: %d", i, out.Value)
			}
			outputCount++
		}
	}

	if outputCount != 3 {
		t.Errorf("Expected 3 reward outputs, got %d", outputCount)
	}
}
