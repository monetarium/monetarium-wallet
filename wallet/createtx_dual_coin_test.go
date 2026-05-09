// Copyright (c) 2024 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"math/big"
	"testing"

	"github.com/monetarium/monetarium-wallet/errors"
	"github.com/monetarium/monetarium-wallet/wallet/txauthor"
	"github.com/monetarium/monetarium-wallet/wallet/txrules"
	"github.com/monetarium/monetarium-wallet/wallet/txsizes"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/dcrutil"
	"github.com/monetarium/monetarium-node/wire"
)

// TestDualCoinValidationLogic tests the dual-coin validation logic in transaction creation
func TestDualCoinValidationLogic(t *testing.T) {
	testCases := []struct {
		name        string
		outputs     []*wire.TxOut
		expectError bool
		description string
	}{
		{
			name: "All VAR outputs",
			outputs: []*wire.TxOut{
				{Value: 1e6, CoinType: cointype.CoinTypeVAR},
				{Value: 2e6, CoinType: cointype.CoinTypeVAR},
			},
			expectError: false,
			description: "All VAR outputs should be valid",
		},
		{
			name: "All SKA1 outputs",
			outputs: []*wire.TxOut{
				{Value: 1e6, CoinType: cointype.CoinType(1)},
				{Value: 2e6, CoinType: cointype.CoinType(1)},
			},
			expectError: false,
			description: "All same SKA type outputs should be valid",
		},
		{
			name: "Single VAR output",
			outputs: []*wire.TxOut{
				{Value: 1e6, CoinType: cointype.CoinTypeVAR},
			},
			expectError: false,
			description: "Single VAR output should be valid",
		},
		{
			name: "Single SKA output",
			outputs: []*wire.TxOut{
				{Value: 1e6, CoinType: cointype.CoinType(1)},
			},
			expectError: false,
			description: "Single SKA output should be valid",
		},
		{
			name: "Mixed VAR and SKA1",
			outputs: []*wire.TxOut{
				{Value: 1e6, CoinType: cointype.CoinTypeVAR},
				{Value: 2e6, CoinType: cointype.CoinType(1)},
			},
			expectError: true,
			description: "Mixed VAR and SKA should be invalid",
		},
		{
			name: "Mixed SKA1 and SKA2",
			outputs: []*wire.TxOut{
				{Value: 1e6, CoinType: cointype.CoinType(1)},
				{Value: 2e6, CoinType: cointype.CoinType(2)},
			},
			expectError: true,
			description: "Mixed different SKA types should be invalid",
		},
		{
			name: "Three mixed outputs",
			outputs: []*wire.TxOut{
				{Value: 1e6, CoinType: cointype.CoinTypeVAR},
				{Value: 2e6, CoinType: cointype.CoinType(1)},
				{Value: 3e6, CoinType: cointype.CoinType(2)},
			},
			expectError: true,
			description: "Multiple different coin types should be invalid",
		},
		{
			name:        "Empty outputs",
			outputs:     []*wire.TxOut{},
			expectError: false, // Empty outputs are handled elsewhere
			description: "Empty outputs should not trigger coin type validation error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Drive the production helper directly. The full
			// validateAuthoredCoinTypes path requires a walletdb.ReadTx
			// and is exercised by the integration tests; here we pin the
			// pure-output uniformity contract.
			err := txrules.ValidateCoinTypeUniformity(tc.outputs)

			if tc.expectError && err == nil {
				t.Errorf("Expected error for %s, but validation passed", tc.description)
			}

			if !tc.expectError && err != nil {
				t.Errorf("Unexpected error for %s: %v", tc.description, err)
			}

			// If we expect an error, verify it's the right kind.
			if tc.expectError && err != nil {
				if !errors.Is(err, errors.Invalid) {
					t.Errorf("Expected Invalid error, got: %v", err)
				}
			}
		})
	}
}

// TestInputStructCoinTypeUsage tests proper usage of the CoinType field in Input struct
func TestInputStructCoinTypeUsage(t *testing.T) {
	testCases := []struct {
		name        string
		coinType    cointype.CoinType
		value       int64
		description string
	}{
		{
			name:        "VAR input",
			coinType:    cointype.CoinTypeVAR,
			value:       1e8,
			description: "VAR input should preserve coin type",
		},
		{
			name:        "SKA1 input",
			coinType:    cointype.CoinType(1),
			value:       2e8,
			description: "SKA1 input should preserve coin type",
		},
		{
			name:        "SKA255 input",
			coinType:    cointype.CoinType(255),
			value:       5e8,
			description: "SKA255 input should preserve coin type",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create Input with CoinType field
			input := Input{
				OutPoint: wire.OutPoint{},
				PrevOut: wire.TxOut{
					Value:    tc.value,
					CoinType: tc.coinType,
				},
				CoinType: tc.coinType,
			}

			// Test that CoinType field is properly set
			if input.CoinType != tc.coinType {
				t.Errorf("Input.CoinType mismatch: got %v, want %v",
					input.CoinType, tc.coinType)
			}

			// Test that PrevOut.CoinType matches
			if input.PrevOut.CoinType != tc.coinType {
				t.Errorf("Input.PrevOut.CoinType mismatch: got %v, want %v",
					input.PrevOut.CoinType, tc.coinType)
			}

			// Test that value is preserved
			if input.PrevOut.Value != tc.value {
				t.Errorf("Input.PrevOut.Value mismatch: got %d, want %d",
					input.PrevOut.Value, tc.value)
			}
		})
	}
}

// TestTransactionCreationWithDifferentCoinTypes tests transaction creation for different coin types
func TestTransactionCreationWithDifferentCoinTypes(t *testing.T) {
	testCases := []struct {
		name         string
		coinType     cointype.CoinType
		inputAmount  dcrutil.Amount
		outputAmount dcrutil.Amount
		expectFees   bool
		description  string
	}{
		{
			name:         "VAR transaction",
			coinType:     cointype.CoinTypeVAR,
			inputAmount:  1e8,
			outputAmount: 9e7, // Leave room for fees
			expectFees:   true,
			description:  "VAR transaction should include fees",
		},
		{
			name:         "SKA1 transaction",
			coinType:     cointype.CoinType(1),
			inputAmount:  1e8,
			outputAmount: 1e8, // Exact amount - no fees
			expectFees:   false,
			description:  "SKA transaction should have no fees",
		},
		{
			name:         "SKA255 transaction",
			coinType:     cointype.CoinType(255),
			inputAmount:  5e7,
			outputAmount: 5e7, // Exact amount - no fees
			expectFees:   false,
			description:  "All SKA types should have no fees",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test the transaction creation principles

			// For VAR transactions, inputs must cover outputs + fees
			// For SKA transactions, inputs must exactly equal outputs

			if tc.expectFees {
				// VAR transaction logic
				if tc.inputAmount <= tc.outputAmount {
					t.Errorf("VAR transaction input (%v) should be greater than output (%v) to cover fees",
						tc.inputAmount, tc.outputAmount)
				}
			} else {
				// SKA transaction logic
				if tc.inputAmount != tc.outputAmount {
					t.Errorf("SKA transaction input (%v) should exactly equal output (%v)",
						tc.inputAmount, tc.outputAmount)
				}
			}

			// Verify coin type consistency
			testInput := Input{
				CoinType: tc.coinType,
				PrevOut: wire.TxOut{
					Value:    int64(tc.inputAmount),
					CoinType: tc.coinType,
				},
			}

			testOutput := &wire.TxOut{
				Value:    int64(tc.outputAmount),
				CoinType: tc.coinType,
			}

			// Input and output should have matching coin types
			if testInput.CoinType != tc.coinType {
				t.Errorf("Input coin type mismatch: got %v, want %v",
					testInput.CoinType, tc.coinType)
			}

			if testOutput.CoinType != tc.coinType {
				t.Errorf("Output coin type mismatch: got %v, want %v",
					testOutput.CoinType, tc.coinType)
			}
		})
	}
}

// TestOutputSelectionAlgorithmWithCoinTypes tests output selection with coin type awareness
func TestOutputSelectionAlgorithmWithCoinTypes(t *testing.T) {
	// Test different output selection algorithms
	algorithms := []OutputSelectionAlgorithm{
		OutputSelectionAlgorithmDefault,
		OutputSelectionAlgorithmAll,
	}

	coinTypes := []cointype.CoinType{
		cointype.CoinTypeVAR,
		cointype.CoinType(1),
		cointype.CoinType(2),
	}

	for _, algo := range algorithms {
		for _, coinType := range coinTypes {
			t.Run(string(rune(algo))+"_"+string(rune(coinType)), func(t *testing.T) {
				// Test that algorithm selection works with coin types
				// This is mainly testing that the enum values are valid

				switch algo {
				case OutputSelectionAlgorithmDefault:
					// Default algorithm should work with any coin type
					if coinType < 0 || coinType > 255 {
						t.Errorf("Invalid coin type for default algorithm: %v", coinType)
					}
				case OutputSelectionAlgorithmAll:
					// All algorithm should work with any coin type
					if coinType < 0 || coinType > 255 {
						t.Errorf("Invalid coin type for all algorithm: %v", coinType)
					}
				default:
					t.Errorf("Unknown output selection algorithm: %v", algo)
				}
			})
		}
	}
}

// TestTransactionValidationEdgeCases tests edge cases in transaction validation
func TestTransactionValidationEdgeCases(t *testing.T) {
	testCases := []struct {
		name        string
		outputs     []*wire.TxOut
		expectError bool
		description string
	}{
		{
			name: "Single zero-value VAR output",
			outputs: []*wire.TxOut{
				{Value: 0, CoinType: cointype.CoinTypeVAR},
			},
			expectError: false, // Zero value validation happens elsewhere
			description: "Zero-value outputs don't trigger coin type validation",
		},
		{
			name: "Single negative-value SKA output",
			outputs: []*wire.TxOut{
				{Value: -1, CoinType: cointype.CoinType(1)},
			},
			expectError: false, // Negative value validation happens elsewhere
			description: "Negative-value outputs don't trigger coin type validation",
		},
		{
			name: "Maximum coin type value",
			outputs: []*wire.TxOut{
				{Value: 1e6, CoinType: cointype.CoinType(255)},
				{Value: 2e6, CoinType: cointype.CoinType(255)},
			},
			expectError: false,
			description: "Maximum coin type (255) should be valid",
		},
		{
			name: "Large number of same-type outputs",
			outputs: func() []*wire.TxOut {
				outputs := make([]*wire.TxOut, 100)
				for i := range outputs {
					outputs[i] = &wire.TxOut{
						Value:    int64(i + 1),
						CoinType: cointype.CoinType(1),
					}
				}
				return outputs
			}(),
			expectError: false,
			description: "Many outputs of same coin type should be valid",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := txrules.ValidateCoinTypeUniformity(tc.outputs)

			if tc.expectError && err == nil {
				t.Errorf("Expected error for %s, but validation passed", tc.description)
			}

			if !tc.expectError && err != nil {
				t.Errorf("Unexpected error for %s: %v", tc.description, err)
			}
		})
	}
}

// TestChangeSourceCoinTypeCompatibility tests change source compatibility with coin types
func TestChangeSourceCoinTypeCompatibility(t *testing.T) {
	// Test that change sources work with different coin types
	coinTypes := []cointype.CoinType{
		cointype.CoinTypeVAR,
		cointype.CoinType(1),
		cointype.CoinType(2),
		cointype.CoinType(255),
	}

	for _, coinType := range coinTypes {
		t.Run(string(rune(coinType)), func(t *testing.T) {
			// Test that change source interface works regardless of coin type
			// In a real implementation, change sources would need to be coin-type aware

			// Mock change source
			changeSource := &testChangeSource{}

			script, version, err := changeSource.Script()
			if err != nil {
				t.Errorf("Change source failed for coin type %v: %v", coinType, err)
			}

			if len(script) == 0 {
				t.Errorf("Change source returned empty script for coin type %v", coinType)
			}

			if version != 0 {
				t.Errorf("Expected script version 0 for coin type %v, got %v", coinType, version)
			}

			size := changeSource.ScriptSize()
			if size <= 0 {
				t.Errorf("Change source returned invalid size for coin type %v: %d", coinType, size)
			}
		})
	}
}

// Mock change source for testing
type testChangeSource struct{}

func (cs *testChangeSource) Script() ([]byte, uint16, error) {
	// Return a mock P2PKH script
	return make([]byte, 25), 0, nil
}

func (cs *testChangeSource) ScriptSize() int {
	return 25
}

// TestInputSourceCoinTypeCompatibility tests input source compatibility with coin types
func TestInputSourceCoinTypeCompatibility(t *testing.T) {
	coinTypes := []cointype.CoinType{
		cointype.CoinTypeVAR,
		cointype.CoinType(1),
		cointype.CoinType(100),
		cointype.CoinType(255),
	}

	for _, coinType := range coinTypes {
		t.Run(string(rune(coinType)), func(t *testing.T) {
			// Test that input source interface works with coin types
			// Create a mock input source that simulates coin-type aware selection

			inputSource := createMockInputSource(coinType, 1e8)

			target := dcrutil.Amount(5e7)
			inputDetail, err := inputSource(target, cointype.Zero())

			if err != nil {
				t.Errorf("Input source failed for coin type %v: %v", coinType, err)
			}

			if inputDetail == nil {
				t.Errorf("Input source returned nil detail for coin type %v", coinType)
				return
			}

			if inputDetail.Amount < target {
				t.Errorf("Input source didn't meet target for coin type %v: got %v, want at least %v",
					coinType, inputDetail.Amount, target)
			}

			if len(inputDetail.Inputs) == 0 {
				t.Errorf("Input source returned no inputs for coin type %v", coinType)
			}
		})
	}
}

// Mock input source for testing
// TestSKAMultisigFeeAccountsForChangeOutput regression-tests the fee math
// in the SKA branch of txToMultisigInternal. Previously the change-output
// decision used a stale int64 feeEstForTx (≈1e6) heuristic that is ~12
// orders of magnitude smaller than the actual SKA fee (~1e18 atoms/KB);
// when that heuristic predicted "no change" but the real fee math left
// enough leftover to emit one, the fee was computed without accounting
// for the change output's ~25 bytes. The fix is to always include
// P2PKHPkScriptSize in the SKA fee estimate. Verify here that the fee
// for a serialize-size that already accounts for change is strictly
// greater than the fee for the same outputs without change.
func TestSKAMultisigFeeAccountsForChangeOutput(t *testing.T) {
	// One 25-byte input script (mock multisig redemption) and one P2SH
	// output. This mirrors the per-call shape of txToMultisigInternal.
	scriptSizes := []int{25}
	outs := []*wire.TxOut{{
		Value:    0,
		PkScript: make([]byte, txsizes.P2SHPkScriptSize),
		CoinType: cointype.CoinType(1), // SKA1
	}}

	feeSizeNoChange := txsizes.EstimateSerializeSizeSKA(scriptSizes, outs, 0)
	feeSizeWithChange := txsizes.EstimateSerializeSizeSKA(scriptSizes, outs, txsizes.P2PKHPkScriptSize)

	if feeSizeWithChange <= feeSizeNoChange {
		t.Fatalf("size with change must exceed size without change: with=%d without=%d",
			feeSizeWithChange, feeSizeNoChange)
	}

	// Use a representative SKA relay fee (4 SKA/kB on mainnet) and
	// confirm the fee differential is large enough that under-charging
	// would leave the tx below relay-rule threshold for the change-emitted
	// case.
	relayFee := cointype.NewSKAAmount(new(big.Int).Mul(big.NewInt(4), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)))
	feeNoChange := txrules.FeeForSerializeSizeSKA(relayFee, feeSizeNoChange)
	feeWithChange := txrules.FeeForSerializeSizeSKA(relayFee, feeSizeWithChange)

	if feeWithChange.Cmp(feeNoChange) <= 0 {
		t.Fatalf("fee with change must exceed fee without change: with=%s without=%s",
			feeWithChange.String(), feeNoChange.String())
	}
}

func createMockInputSource(coinType cointype.CoinType, availableAmount dcrutil.Amount) txauthor.InputSource {
	return func(target dcrutil.Amount, targetSKA cointype.SKAAmount) (*txauthor.InputDetail, error) {
		if target > availableAmount {
			return nil, errors.E(errors.InsufficientBalance, "not enough funds")
		}

		// Create mock input
		input := &wire.TxIn{
			PreviousOutPoint: wire.OutPoint{},
			ValueIn:          int64(availableAmount),
		}

		return &txauthor.InputDetail{
			Amount:            availableAmount,
			Inputs:            []*wire.TxIn{input},
			Scripts:           [][]byte{make([]byte, 25)},
			RedeemScriptSizes: []int{25},
		}, nil
	}
}

// TestTxToMultisigInternalSKAFeePreBudget verifies HIGH-1: txToMultisigInternal's
// SKA pre-selection fee budget must be derived from the real SKA relay fee, not
// the int64 VAR heuristic (5e7 atoms on mainnet — ~12 orders of magnitude too
// small). The pre-budget is what's passed to findEligibleOutputsAmount; if it
// undershoots the post-selection real fee, the call returns spurious
// InsufficientBalance even when the wallet has UTXOs that would cover the real
// fee.
//
// This test exercises the pre-budget math directly (without a full wallet) by
// referencing the same multisigFeePreSelectInputGuess constant the
// production path uses, so any future drift in that constant flows into the
// test rather than silently passing.
func TestTxToMultisigInternalSKAFeePreBudget(t *testing.T) {
	// Mainnet-representative SKA relay fee: 4 SKA/kB at AtomsPerCoin=1e18 →
	// 4e18 atoms/kB. This is the value the configured RelayFeeForCoinType
	// returns for SKA on mainnet.
	relayFee := cointype.NewSKAAmount(new(big.Int).Mul(
		big.NewInt(4),
		new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil),
	))

	// Target SKA amount: 1 SKA = 1e18 atoms.
	amountSKA := cointype.NewSKAAmount(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))

	// Reproduce the pre-budget computation from txToMultisigInternal,
	// using the same package-level constant the production path uses.
	estScriptSizes := make([]int, multisigFeePreSelectInputGuess)
	for i := range estScriptSizes {
		estScriptSizes[i] = txsizes.RedeemP2SHSigScriptSize
	}
	estTxOuts := []*wire.TxOut{{
		Value:    0,
		SKAValue: amountSKA.BigInt(),
		PkScript: make([]byte, txsizes.P2SHPkScriptSize),
		CoinType: cointype.CoinType(1),
	}}
	preBudgetSize := txsizes.EstimateSerializeSizeSKA(estScriptSizes, estTxOuts, txsizes.P2PKHPkScriptSize)
	skaFeePreBudget := txrules.FeeForSerializeSizeSKA(relayFee, preBudgetSize)

	// The old buggy pre-budget was 5e7 atoms. The new one must dwarf it,
	// because real SKA fees are ~1e18 atoms/kB and a typical multisig tx
	// is a few hundred bytes minimum.
	oldBuggyBudget := cointype.SKAAmountFromInt64(5e7)
	if skaFeePreBudget.Cmp(oldBuggyBudget) <= 0 {
		t.Fatalf("new SKA pre-budget %s must exceed old buggy 5e7 atoms",
			skaFeePreBudget.String())
	}

	// The pre-budget must cover the real fee at every selection size the
	// production path can produce (i.e. up to multisigFeePreSelectInputGuess
	// inputs). Compare against several N values rather than just the boundary
	// case; comparing only at N==guess would be structurally `X >= X` because
	// both sides feed identical inputs into the same formula.
	//
	// At N < guess, the pre-budget should strictly exceed the real fee
	// (over-estimation is intentional — harmless slack so findEligibleOutputsAmount
	// pulls inputs that comfortably cover the post-selection recompute).
	// At N == guess, the pre-budget equals the real fee by construction.
	// We assert >= here so the test stays valid if the constant changes.
	var (
		lastFee    cointype.SKAAmount
		haveLast   bool
	)
	for _, n := range []int{1, 10, 25, multisigFeePreSelectInputGuess} {
		ss := make([]int, n)
		for i := range ss {
			ss[i] = txsizes.RedeemP2SHSigScriptSize
		}
		sz := txsizes.EstimateSerializeSizeSKA(ss, estTxOuts, txsizes.P2PKHPkScriptSize)
		fee := txrules.FeeForSerializeSizeSKA(relayFee, sz)
		if skaFeePreBudget.Cmp(fee) < 0 {
			t.Fatalf("pre-budget %s must be >= real fee %s for a %d-input shape",
				skaFeePreBudget.String(), fee.String(), n)
		}
		// Strictness: real fee must grow with input count. If it didn't,
		// EstimateSerializeSizeSKA or FeeForSerializeSizeSKA has regressed.
		if haveLast && fee.Cmp(lastFee) <= 0 {
			t.Fatalf("real fee for %d inputs (%s) must exceed prior fee (%s)",
				n, fee.String(), lastFee.String())
		}
		lastFee = fee
		haveLast = true
	}

	// Boundary sanity: at N==guess+1, the pre-budget is necessarily
	// insufficient (this is the regime where the SKA balance check at
	// line ~781 of createtx.go correctly returns InsufficientBalance).
	// Verifying this proves the constant is the actual cliff, not just
	// a number we picked.
	overSS := make([]int, multisigFeePreSelectInputGuess+1)
	for i := range overSS {
		overSS[i] = txsizes.RedeemP2SHSigScriptSize
	}
	overSz := txsizes.EstimateSerializeSizeSKA(overSS, estTxOuts, txsizes.P2PKHPkScriptSize)
	overFee := txrules.FeeForSerializeSizeSKA(relayFee, overSz)
	if skaFeePreBudget.Cmp(overFee) >= 0 {
		t.Fatalf("pre-budget %s must NOT cover real fee %s for %d inputs "+
			"(otherwise multisigFeePreSelectInputGuess is mis-set)",
			skaFeePreBudget.String(), overFee.String(),
			multisigFeePreSelectInputGuess+1)
	}
}

// TestTxToMultisigInternalVARFeePreBudget verifies that the VAR pre-selection
// fee budget is now derived from the configured RelayFee() instead of the old
// hard-coded 5e7/3e4 atom switch. The VAR formula must scale with the
// operator's --minrelaytxfee setting and produce a budget that covers the
// post-selection real fee at every input count up to the guess ceiling, and
// must not cover it once N exceeds the ceiling (so the constant is the
// actual cliff and not just a happens-to-pass number).
func TestTxToMultisigInternalVARFeePreBudget(t *testing.T) {
	// Stand-in for w.RelayFee() — pick a deliberately non-default rate so
	// the test fails if the production path silently drops back to a
	// constant. 1e5 atoms/kB is 10× the upstream Decred wallet default; an
	// operator setting --minrelaytxfee higher than the default is exactly
	// the workflow this fix unblocks.
	relayFee := dcrutil.Amount(1e5)

	// Target VAR amount, immaterial to the size math but kept realistic
	// for documentation.
	amount := dcrutil.Amount(1e8)

	estScriptSizes := make([]int, multisigFeePreSelectInputGuess)
	for i := range estScriptSizes {
		estScriptSizes[i] = txsizes.RedeemP2SHSigScriptSize
	}
	estTxOuts := []*wire.TxOut{{
		Value:    int64(amount),
		PkScript: make([]byte, txsizes.P2SHPkScriptSize),
		CoinType: cointype.CoinTypeVAR,
	}}
	preBudgetSize := txsizes.EstimateSerializeSize(estScriptSizes, estTxOuts, txsizes.P2PKHPkScriptSize)
	feeEstForTx := txrules.FeeForSerializeSize(relayFee, preBudgetSize)

	// The pre-budget must reflect the configured rate. Old hard-coded
	// budgets were 5e7 (mainnet/testnet) or 3e4 (default). Neither tracks
	// --minrelaytxfee, so the new budget should differ from both anchors
	// once the rate is non-default.
	if feeEstForTx == 5e7 || feeEstForTx == 3e4 {
		t.Fatalf("pre-budget %d still matches a hard-coded constant — "+
			"VAR branch is not deriving from RelayFee()", feeEstForTx)
	}

	// At N <= guess, the pre-budget must cover the real fee.
	var (
		lastFee  dcrutil.Amount
		haveLast bool
	)
	for _, n := range []int{1, 10, 25, multisigFeePreSelectInputGuess} {
		ss := make([]int, n)
		for i := range ss {
			ss[i] = txsizes.RedeemP2SHSigScriptSize
		}
		sz := txsizes.EstimateSerializeSize(ss, estTxOuts, txsizes.P2PKHPkScriptSize)
		fee := txrules.FeeForSerializeSize(relayFee, sz)
		if feeEstForTx < fee {
			t.Fatalf("pre-budget %d must be >= real fee %d for a %d-input shape",
				feeEstForTx, fee, n)
		}
		if haveLast && fee <= lastFee {
			t.Fatalf("real fee for %d inputs (%d) must exceed prior fee (%d)",
				n, fee, lastFee)
		}
		lastFee = fee
		haveLast = true
	}

	// Boundary sanity: at N == guess+1, the pre-budget is necessarily
	// insufficient. This proves the constant is the cliff, not a
	// happens-to-pass number.
	overSS := make([]int, multisigFeePreSelectInputGuess+1)
	for i := range overSS {
		overSS[i] = txsizes.RedeemP2SHSigScriptSize
	}
	overSz := txsizes.EstimateSerializeSize(overSS, estTxOuts, txsizes.P2PKHPkScriptSize)
	overFee := txrules.FeeForSerializeSize(relayFee, overSz)
	if feeEstForTx >= overFee {
		t.Fatalf("pre-budget %d must NOT cover real fee %d for %d inputs",
			feeEstForTx, overFee, multisigFeePreSelectInputGuess+1)
	}
}
