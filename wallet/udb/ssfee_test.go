// Copyright (c) 2024 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package udb

import (
	"testing"

	"github.com/monetarium/monetarium-node/chaincfg/chainhash"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/txscript"
	"github.com/monetarium/monetarium-node/wire"
)

// createMockSSFeeTx creates a mock SSFee transaction for testing.
// Creates null-input SSFee (creates new UTXO).
func createMockSSFeeTx(ct cointype.CoinType, numOutputs int, amount int64, markerType string) *wire.MsgTx {
	return createMockSSFeeTxEx(ct, numOutputs, amount, markerType, nil, 0)
}

// createMockSSFeeTxEx creates a mock SSFee transaction with optional UTXO augmentation.
// If prevOutpoint is provided, creates an augmented SSFee that spends that UTXO.
// If prevOutpoint is nil, creates a null-input SSFee (like coinbase).
//
// Parameters:
//   - ct: Coin type (VAR or SKA)
//   - numOutputs: Number of reward outputs
//   - amount: Value per output
//   - markerType: "MF" for miner fee or "SF" for staker fee
//   - prevOutpoint: Optional previous output to spend (for augmentation)
//   - prevValue: Value of previous output (ignored if prevOutpoint is nil)
func createMockSSFeeTxEx(ct cointype.CoinType, numOutputs int, amount int64, markerType string,
	prevOutpoint *wire.OutPoint, prevValue int64) *wire.MsgTx {

	tx := wire.NewMsgTx()
	tx.Version = 3

	// Add input: either null (creates new) or real UTXO (augments existing)
	if prevOutpoint != nil {
		// Augmented SSFee - use real UTXO as input
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: *prevOutpoint,
			SignatureScript:  []byte{}, // Empty signature for SSFee
			ValueIn:          prevValue,
		})
	} else {
		// Null input SSFee (like coinbase) - creates new UTXO
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{},
				Index: wire.MaxPrevOutIndex,
			},
			SignatureScript: []byte{}, // Empty signature for SSFee
		})
	}

	// Add reward outputs
	for i := 0; i < numOutputs; i++ {
		tx.AddTxOut(&wire.TxOut{
			Value:    amount,
			CoinType: ct,
			PkScript: make([]byte, 25), // Dummy P2PKH script
		})
	}

	// Add OP_RETURN with marker
	var marker []byte
	if markerType == "MF" {
		marker = []byte{0x4D, 0x46} // "MF"
	} else {
		marker = []byte{0x53, 0x46} // "SF"
	}

	opReturnScript := []byte{
		txscript.OP_RETURN, // 0x6a
		0x06,               // OP_DATA_6
	}
	opReturnScript = append(opReturnScript, marker...)
	opReturnScript = append(opReturnScript, []byte{0x00, 0x00, 0x00, 0x00}...) // height

	tx.AddTxOut(&wire.TxOut{
		Value:    0,
		PkScript: opReturnScript,
		CoinType: ct,
	})

	return tx
}

func TestIsSSFeeTx(t *testing.T) {
	tests := []struct {
		name     string
		tx       *wire.MsgTx
		expected bool
	}{
		{
			name:     "SSFee MF transaction",
			tx:       createMockSSFeeTx(cointype.CoinType(1), 3, 1000, "MF"),
			expected: true,
		},
		{
			name:     "SSFee SF transaction",
			tx:       createMockSSFeeTx(cointype.CoinType(1), 3, 1000, "SF"),
			expected: true,
		},
		{
			name: "Regular transaction",
			tx: &wire.MsgTx{
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: wire.OutPoint{
						Hash:  chainhash.Hash{1, 2, 3},
						Index: 0,
					},
				}},
				TxOut: []*wire.TxOut{{
					Value:    1000,
					CoinType: cointype.CoinTypeVAR,
				}},
			},
			expected: false,
		},
		{
			name: "Coinbase (not SSFee)",
			tx: &wire.MsgTx{
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: wire.OutPoint{
						Hash:  chainhash.Hash{},
						Index: wire.MaxPrevOutIndex,
					},
					SignatureScript: []byte{0x00, 0x01}, // Coinbase signature
				}},
				TxOut: []*wire.TxOut{{
					Value:    5000000000,
					CoinType: cointype.CoinTypeVAR,
				}},
			},
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := isSSFeeTx(test.tx)
			if result != test.expected {
				t.Errorf("%s: isSSFeeTx() = %v, want %v", test.name, result, test.expected)
			}
		})
	}
}

func TestGetSSFeeType(t *testing.T) {
	tests := []struct {
		name     string
		tx       *wire.MsgTx
		expected string
	}{
		{
			name:     "Miner Fee (MF)",
			tx:       createMockSSFeeTx(cointype.CoinType(1), 3, 1000, "MF"),
			expected: "MF",
		},
		{
			name:     "Staker Fee (SF)",
			tx:       createMockSSFeeTx(cointype.CoinType(1), 3, 1000, "SF"),
			expected: "SF",
		},
		{
			name: "Not SSFee",
			tx: &wire.MsgTx{
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: wire.OutPoint{
						Hash:  chainhash.Hash{1},
						Index: 0,
					},
				}},
				TxOut: []*wire.TxOut{{Value: 1000}},
			},
			expected: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := getSSFeeType(test.tx)
			if result != test.expected {
				t.Errorf("%s: getSSFeeType() = %q, want %q", test.name, result, test.expected)
			}
		})
	}
}

func TestIsSSFeeMinerTx(t *testing.T) {
	tests := []struct {
		name     string
		tx       *wire.MsgTx
		expected bool
	}{
		{
			name:     "Miner Fee transaction",
			tx:       createMockSSFeeTx(cointype.CoinType(1), 3, 1000, "MF"),
			expected: true,
		},
		{
			name:     "Staker Fee transaction",
			tx:       createMockSSFeeTx(cointype.CoinType(1), 3, 1000, "SF"),
			expected: false,
		},
		{
			name: "Regular transaction",
			tx: &wire.MsgTx{
				Version: 1,
				TxIn:    []*wire.TxIn{{PreviousOutPoint: wire.OutPoint{Hash: chainhash.Hash{1}, Index: 0}}},
				TxOut:   []*wire.TxOut{{Value: 1000}},
			},
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := isSSFeeMinerTx(test.tx)
			if result != test.expected {
				t.Errorf("%s: isSSFeeMinerTx() = %v, want %v", test.name, result, test.expected)
			}
		})
	}
}

// TestAugmentedSSFeeCreation tests that the helper can create both null-input
// and augmented SSFee transactions correctly.
func TestAugmentedSSFeeCreation(t *testing.T) {
	coinType := cointype.CoinType(1) // SKA1

	t.Run("Null input SSFee", func(t *testing.T) {
		tx := createMockSSFeeTx(coinType, 1, 1000, "SF")

		// Verify null input
		if len(tx.TxIn) != 1 {
			t.Fatalf("Expected 1 input, got %d", len(tx.TxIn))
		}
		if tx.TxIn[0].PreviousOutPoint.Index != wire.MaxPrevOutIndex {
			t.Errorf("Expected null input (MaxPrevOutIndex), got index %d",
				tx.TxIn[0].PreviousOutPoint.Index)
		}

		// Verify it's identified as SSFee
		if getSSFeeType(tx) != "SF" {
			t.Error("Transaction not identified as SSFee SF")
		}
	})

	t.Run("Augmented SSFee", func(t *testing.T) {
		// Create a previous outpoint to augment
		prevHash, _ := chainhash.NewHashFromStr("1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")
		prevOutpoint := &wire.OutPoint{
			Hash:  *prevHash,
			Index: 0,
			Tree:  wire.TxTreeStake,
		}
		prevValue := int64(5000)
		feeAmount := int64(1000)

		// Create augmented SSFee
		tx := createMockSSFeeTxEx(coinType, 1, prevValue+feeAmount, "SF", prevOutpoint, prevValue)

		// Verify real input (not null)
		if len(tx.TxIn) != 1 {
			t.Fatalf("Expected 1 input, got %d", len(tx.TxIn))
		}
		if tx.TxIn[0].PreviousOutPoint.Index == wire.MaxPrevOutIndex {
			t.Error("Expected real input, got null input")
		}
		if tx.TxIn[0].PreviousOutPoint.Hash != *prevHash {
			t.Errorf("Input hash mismatch: got %v, want %v",
				tx.TxIn[0].PreviousOutPoint.Hash, *prevHash)
		}
		if tx.TxIn[0].ValueIn != prevValue {
			t.Errorf("Input value mismatch: got %d, want %d",
				tx.TxIn[0].ValueIn, prevValue)
		}

		// Verify output value = input + fee
		// (excluding OP_RETURN which has value 0)
		if len(tx.TxOut) < 2 {
			t.Fatalf("Expected at least 2 outputs, got %d", len(tx.TxOut))
		}
		outputValue := tx.TxOut[0].Value
		expectedOutput := prevValue + feeAmount
		if outputValue != expectedOutput {
			t.Errorf("Output value mismatch: got %d, want %d (input %d + fee %d)",
				outputValue, expectedOutput, prevValue, feeAmount)
		}

		// Verify it's still identified as SSFee
		if getSSFeeType(tx) != "SF" {
			t.Error("Augmented transaction not identified as SSFee SF")
		}
	})

	t.Run("Augmented miner SSFee", func(t *testing.T) {
		prevHash, _ := chainhash.NewHashFromStr("abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")
		prevOutpoint := &wire.OutPoint{
			Hash:  *prevHash,
			Index: 1,
			Tree:  wire.TxTreeStake,
		}
		prevValue := int64(3000)
		feeAmount := int64(500)

		// Create augmented miner SSFee
		tx := createMockSSFeeTxEx(coinType, 1, prevValue+feeAmount, "MF", prevOutpoint, prevValue)

		// Verify it's augmented (real input)
		if tx.TxIn[0].PreviousOutPoint.Index == wire.MaxPrevOutIndex {
			t.Error("Expected real input for augmented miner SSFee")
		}

		// Verify it's identified as miner fee
		if getSSFeeType(tx) != "MF" {
			t.Error("Augmented miner SSFee not identified correctly")
		}

		// Verify treated as coinbase-like (miner fees)
		if !isSSFeeMinerTx(tx) {
			t.Error("Augmented miner SSFee not identified as miner transaction")
		}
	})
}

// TestAugmentedSSFeeValueCalculation verifies the fee calculation logic
// for augmented SSFee transactions.
func TestAugmentedSSFeeValueCalculation(t *testing.T) {
	tests := []struct {
		name       string
		prevValue  int64
		feeAmount  int64
		wantOutput int64
	}{
		{
			name:       "Small fee accumulation",
			prevValue:  1000,
			feeAmount:  100,
			wantOutput: 1100,
		},
		{
			name:       "Large fee accumulation",
			prevValue:  5000000,
			feeAmount:  1000000,
			wantOutput: 6000000,
		},
		{
			name:       "Tiny dust consolidation",
			prevValue:  300, // 0.000003 SKA
			feeAmount:  300,
			wantOutput: 600,
		},
		{
			name:       "Zero previous value (first fee)",
			prevValue:  0,
			feeAmount:  500,
			wantOutput: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prevHash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
			prevOutpoint := &wire.OutPoint{
				Hash:  *prevHash,
				Index: 0,
				Tree:  wire.TxTreeStake,
			}

			tx := createMockSSFeeTxEx(cointype.CoinType(1), 1, tt.wantOutput, "SF",
				prevOutpoint, tt.prevValue)

			// Get output value (first non-OP_RETURN output)
			var outputValue int64
			for _, out := range tx.TxOut {
				if len(out.PkScript) > 0 && out.PkScript[0] == txscript.OP_RETURN {
					continue
				}
				outputValue = out.Value
				break
			}

			if outputValue != tt.wantOutput {
				t.Errorf("Output value = %d, want %d", outputValue, tt.wantOutput)
			}

			// Verify fee calculation: output - input = fee
			calculatedFee := outputValue - tt.prevValue
			if calculatedFee != tt.feeAmount {
				t.Errorf("Calculated fee = %d, want %d", calculatedFee, tt.feeAmount)
			}
		})
	}
}
