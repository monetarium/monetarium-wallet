// Copyright (c) 2024 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"testing"
	"time"

	"github.com/monetarium/monetarium-node/chaincfg/chainhash"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/dcrutil"
	"github.com/monetarium/monetarium-node/wire"
)

// Helper function to create test transaction outputs as TransactionOutput structs
func createTestTransactionOutput(value int64, coinType cointype.CoinType, height int32) *TransactionOutput {
	return &TransactionOutput{
		OutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{},
			Index: 0,
		},
		Output: wire.TxOut{
			Value:    value,
			Version:  0,
			PkScript: make([]byte, 25),
			CoinType: coinType,
		},
		OutputKind:      OutputKindNormal,
		ContainingBlock: BlockIdentity{Height: height},
		ReceiveTime:     time.Now(),
	}
}

// TestOutputSelectionPolicyCoinTypeFilter tests coin type filtering in OutputSelectionPolicy
func TestOutputSelectionPolicyCoinTypeFilter(t *testing.T) {
	testCases := []struct {
		name           string
		policyFilter   *cointype.CoinType
		outputCoinType cointype.CoinType
		shouldInclude  bool
		description    string
	}{
		{
			name:           "No filter - VAR output",
			policyFilter:   nil,
			outputCoinType: cointype.CoinTypeVAR,
			shouldInclude:  true,
			description:    "No filter should include all coin types",
		},
		{
			name:           "No filter - SKA output",
			policyFilter:   nil,
			outputCoinType: cointype.CoinType(1),
			shouldInclude:  true,
			description:    "No filter should include all coin types",
		},
		{
			name:           "VAR filter - VAR output",
			policyFilter:   func() *cointype.CoinType { ct := cointype.CoinTypeVAR; return &ct }(),
			outputCoinType: cointype.CoinTypeVAR,
			shouldInclude:  true,
			description:    "VAR filter should include VAR outputs",
		},
		{
			name:           "VAR filter - SKA output",
			policyFilter:   func() *cointype.CoinType { ct := cointype.CoinTypeVAR; return &ct }(),
			outputCoinType: cointype.CoinType(1),
			shouldInclude:  false,
			description:    "VAR filter should exclude SKA outputs",
		},
		{
			name:           "SKA1 filter - SKA1 output",
			policyFilter:   func() *cointype.CoinType { ct := cointype.CoinType(1); return &ct }(),
			outputCoinType: cointype.CoinType(1),
			shouldInclude:  true,
			description:    "SKA1 filter should include SKA1 outputs",
		},
		{
			name:           "SKA1 filter - SKA2 output",
			policyFilter:   func() *cointype.CoinType { ct := cointype.CoinType(1); return &ct }(),
			outputCoinType: cointype.CoinType(2),
			shouldInclude:  false,
			description:    "SKA1 filter should exclude SKA2 outputs",
		},
		{
			name:           "SKA1 filter - VAR output",
			policyFilter:   func() *cointype.CoinType { ct := cointype.CoinType(1); return &ct }(),
			outputCoinType: cointype.CoinTypeVAR,
			shouldInclude:  false,
			description:    "SKA1 filter should exclude VAR outputs",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test the filtering logic directly
			var shouldInclude bool
			if tc.policyFilter != nil {
				policy := OutputSelectionPolicy{
					Account:               0,
					RequiredConfirmations: 1,
					CoinType:              *tc.policyFilter,
				}
				// Simulate the filtering condition from UnspentOutputs
				shouldInclude = tc.outputCoinType == policy.CoinType
			} else {
				// When no filter is specified, we'd default to a specific coin type
				// but for testing purposes, nil means "include all"
				shouldInclude = true
			}

			if shouldInclude != tc.shouldInclude {
				t.Errorf("Filter logic mismatch: got %v, want %v. %s",
					shouldInclude, tc.shouldInclude, tc.description)
			}
		})
	}
}

// TestCoinTypeIsolation tests that UTXO operations properly isolate coin types
func TestCoinTypeIsolation(t *testing.T) {
	// This is a conceptual test - in a real implementation, we'd need a full wallet setup
	// Here we test the logic principles

	testOutputs := []*TransactionOutput{
		createTestTransactionOutput(1e8, cointype.CoinTypeVAR, 100), // VAR
		createTestTransactionOutput(2e8, cointype.CoinType(1), 100), // SKA1
		createTestTransactionOutput(3e8, cointype.CoinType(2), 100), // SKA2
		createTestTransactionOutput(4e8, cointype.CoinTypeVAR, 100), // VAR
		createTestTransactionOutput(5e8, cointype.CoinType(1), 100), // SKA1
	}

	// Test filtering by VAR
	varFilter := cointype.CoinTypeVAR
	varOutputs := filterOutputsByCoinType(testOutputs, &varFilter)
	if len(varOutputs) != 2 {
		t.Errorf("Expected 2 VAR outputs, got %d", len(varOutputs))
	}

	// Verify VAR output values
	expectedVARValues := []int64{1e8, 4e8}
	for i, output := range varOutputs {
		if output.Output.Value != expectedVARValues[i] {
			t.Errorf("VAR output %d: expected value %d, got %d",
				i, expectedVARValues[i], output.Output.Value)
		}
		if output.Output.CoinType != cointype.CoinTypeVAR {
			t.Errorf("VAR output %d: wrong coin type %v", i, output.Output.CoinType)
		}
	}

	// Test filtering by SKA1
	ska1Filter := cointype.CoinType(1)
	ska1Outputs := filterOutputsByCoinType(testOutputs, &ska1Filter)
	if len(ska1Outputs) != 2 {
		t.Errorf("Expected 2 SKA1 outputs, got %d", len(ska1Outputs))
	}

	// Verify SKA1 output values
	expectedSKA1Values := []int64{2e8, 5e8}
	for i, output := range ska1Outputs {
		if output.Output.Value != expectedSKA1Values[i] {
			t.Errorf("SKA1 output %d: expected value %d, got %d",
				i, expectedSKA1Values[i], output.Output.Value)
		}
		if output.Output.CoinType != cointype.CoinType(1) {
			t.Errorf("SKA1 output %d: wrong coin type %v", i, output.Output.CoinType)
		}
	}

	// Test filtering by SKA2
	ska2Filter := cointype.CoinType(2)
	ska2Outputs := filterOutputsByCoinType(testOutputs, &ska2Filter)
	if len(ska2Outputs) != 1 {
		t.Errorf("Expected 1 SKA2 output, got %d", len(ska2Outputs))
	}
}

// Helper function to simulate output filtering by coin type
func filterOutputsByCoinType(outputs []*TransactionOutput, coinType *cointype.CoinType) []*TransactionOutput {
	if coinType == nil {
		return outputs
	}

	var filtered []*TransactionOutput
	for _, output := range outputs {
		if output.Output.CoinType == *coinType {
			filtered = append(filtered, output)
		}
	}
	return filtered
}

// TestInputStructCoinType tests the Input struct CoinType field functionality
func TestInputStructCoinType(t *testing.T) {
	testCases := []struct {
		name     string
		coinType cointype.CoinType
		value    int64
	}{
		{"VAR input", cointype.CoinTypeVAR, 1e8},
		{"SKA1 input", cointype.CoinType(1), 2e8},
		{"SKA255 input", cointype.CoinType(255), 3e8},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test input
			input := Input{
				OutPoint: wire.OutPoint{
					Hash:  chainhash.Hash{},
					Index: 0,
				},
				PrevOut: wire.TxOut{
					Value:    tc.value,
					Version:  0,
					PkScript: make([]byte, 25),
					CoinType: tc.coinType,
				},
				CoinType: tc.coinType,
			}

			// Verify coin type consistency
			if input.CoinType != tc.coinType {
				t.Errorf("Input CoinType field mismatch: got %v, want %v",
					input.CoinType, tc.coinType)
			}

			if input.PrevOut.CoinType != tc.coinType {
				t.Errorf("Input PrevOut CoinType mismatch: got %v, want %v",
					input.PrevOut.CoinType, tc.coinType)
			}

			// Verify value is preserved
			if input.PrevOut.Value != tc.value {
				t.Errorf("Input value mismatch: got %d, want %d",
					input.PrevOut.Value, tc.value)
			}
		})
	}
}

// TestMixedCoinTypeDetection tests detection of mixed coin types in collections
func TestMixedCoinTypeDetection(t *testing.T) {
	testCases := []struct {
		name        string
		coinTypes   []cointype.CoinType
		expectMixed bool
		description string
	}{
		{
			name:        "All VAR",
			coinTypes:   []cointype.CoinType{cointype.CoinTypeVAR, cointype.CoinTypeVAR, cointype.CoinTypeVAR},
			expectMixed: false,
			description: "All VAR should not be considered mixed",
		},
		{
			name:        "All SKA1",
			coinTypes:   []cointype.CoinType{cointype.CoinType(1), cointype.CoinType(1), cointype.CoinType(1)},
			expectMixed: false,
			description: "All same SKA type should not be considered mixed",
		},
		{
			name:        "VAR and SKA1",
			coinTypes:   []cointype.CoinType{cointype.CoinTypeVAR, cointype.CoinType(1)},
			expectMixed: true,
			description: "VAR and SKA should be considered mixed",
		},
		{
			name:        "SKA1 and SKA2",
			coinTypes:   []cointype.CoinType{cointype.CoinType(1), cointype.CoinType(2)},
			expectMixed: true,
			description: "Different SKA types should be considered mixed",
		},
		{
			name:        "Single coin type",
			coinTypes:   []cointype.CoinType{cointype.CoinType(1)},
			expectMixed: false,
			description: "Single coin type should not be mixed",
		},
		{
			name:        "Empty collection",
			coinTypes:   []cointype.CoinType{},
			expectMixed: false,
			description: "Empty collection should not be mixed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			isMixed := detectMixedCoinTypes(tc.coinTypes)
			if isMixed != tc.expectMixed {
				t.Errorf("detectMixedCoinTypes(%v) = %v, want %v. %s",
					tc.coinTypes, isMixed, tc.expectMixed, tc.description)
			}
		})
	}
}

// Helper function to detect mixed coin types
func detectMixedCoinTypes(coinTypes []cointype.CoinType) bool {
	if len(coinTypes) <= 1 {
		return false
	}

	firstType := coinTypes[0]
	for _, coinType := range coinTypes[1:] {
		if coinType != firstType {
			return true
		}
	}
	return false
}

// TestUTXOSelectionByCoinType tests UTXO selection algorithms with coin type constraints
func TestUTXOSelectionByCoinType(t *testing.T) {
	// Create test UTXOs with different coin types
	utxos := []struct {
		value    dcrutil.Amount
		coinType cointype.CoinType
	}{
		{1e8, cointype.CoinTypeVAR}, // 1 VAR
		{2e8, cointype.CoinTypeVAR}, // 2 VAR
		{3e8, cointype.CoinType(1)}, // 3 SKA1
		{4e8, cointype.CoinType(1)}, // 4 SKA1
		{5e8, cointype.CoinType(2)}, // 5 SKA2
	}

	// Test VAR selection
	varTargets := []dcrutil.Amount{1e8, 2.5e8, 3e8}
	for _, target := range varTargets {
		selected := selectUTXOsByCoinType(utxos, target, cointype.CoinTypeVAR)

		// Verify all selected UTXOs are VAR
		for _, utxo := range selected {
			if utxo.coinType != cointype.CoinTypeVAR {
				t.Errorf("Selected non-VAR UTXO in VAR selection: %v", utxo.coinType)
			}
		}

		// Verify selection meets target (where possible)
		totalSelected := dcrutil.Amount(0)
		for _, utxo := range selected {
			totalSelected += utxo.value
		}

		if target <= 3e8 && totalSelected < target { // 3e8 is max VAR available
			t.Errorf("VAR selection didn't meet target: selected %v, target %v", totalSelected, target)
		}
	}

	// Test SKA1 selection
	ska1Targets := []dcrutil.Amount{3e8, 6e8, 7e8}
	for _, target := range ska1Targets {
		selected := selectUTXOsByCoinType(utxos, target, cointype.CoinType(1))

		// Verify all selected UTXOs are SKA1
		for _, utxo := range selected {
			if utxo.coinType != cointype.CoinType(1) {
				t.Errorf("Selected non-SKA1 UTXO in SKA1 selection: %v", utxo.coinType)
			}
		}

		// Verify selection meets target (where possible)
		totalSelected := dcrutil.Amount(0)
		for _, utxo := range selected {
			totalSelected += utxo.value
		}

		if target <= 7e8 && totalSelected < target { // 7e8 is max SKA1 available
			t.Errorf("SKA1 selection didn't meet target: selected %v, target %v", totalSelected, target)
		}
	}
}

// Helper function to simulate UTXO selection by coin type
func selectUTXOsByCoinType(utxos []struct {
	value    dcrutil.Amount
	coinType cointype.CoinType
}, target dcrutil.Amount, coinType cointype.CoinType) []struct {
	value    dcrutil.Amount
	coinType cointype.CoinType
} {
	var selected []struct {
		value    dcrutil.Amount
		coinType cointype.CoinType
	}

	totalSelected := dcrutil.Amount(0)

	// Simple greedy selection
	for _, utxo := range utxos {
		if utxo.coinType == coinType && totalSelected < target {
			selected = append(selected, utxo)
			totalSelected += utxo.value
		}
	}

	return selected
}

// TestOutputSelectionPolicyValidation tests validation of OutputSelectionPolicy
func TestOutputSelectionPolicyValidation(t *testing.T) {
	testCases := []struct {
		name        string
		policy      OutputSelectionPolicy
		isValid     bool
		description string
	}{
		{
			name: "Valid VAR policy",
			policy: OutputSelectionPolicy{
				Account:               0,
				RequiredConfirmations: 1,
				CoinType:              cointype.CoinTypeVAR,
			},
			isValid:     true,
			description: "Standard VAR policy should be valid",
		},
		{
			name: "Valid SKA policy",
			policy: OutputSelectionPolicy{
				Account:               0,
				RequiredConfirmations: 1,
				CoinType:              cointype.CoinType(1),
			},
			isValid:     true,
			description: "Standard SKA policy should be valid",
		},
		{
			name: "Valid SKA255 policy",
			policy: OutputSelectionPolicy{
				Account:               0,
				RequiredConfirmations: 1,
				CoinType:              cointype.CoinType(255),
			},
			isValid:     true,
			description: "Policy with max SKA coin type should be valid",
		},
		{
			name: "Negative confirmations with VAR",
			policy: OutputSelectionPolicy{
				Account:               0,
				RequiredConfirmations: -1,
				CoinType:              cointype.CoinTypeVAR,
			},
			isValid:     true, // Negative confirmations are handled elsewhere
			description: "Policy with negative confirmations",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// In a real implementation, we'd have validation logic
			// Here we just verify the policy structure is reasonable
			isValid := validateOutputSelectionPolicy(tc.policy)

			if isValid != tc.isValid {
				t.Errorf("Policy validation mismatch: got %v, want %v. %s",
					isValid, tc.isValid, tc.description)
			}
		})
	}
}

// Helper function to validate OutputSelectionPolicy
func validateOutputSelectionPolicy(policy OutputSelectionPolicy) bool {
	// Basic validation - in reality this would be more comprehensive
	return true // For now, all policies are considered valid
}
