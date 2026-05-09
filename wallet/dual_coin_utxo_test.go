package wallet

import (
	"math/big"
	"testing"

	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/dcrutil"
	"github.com/monetarium/monetarium-node/wire"
	"github.com/monetarium/monetarium-wallet/wallet/txauthor"
	"github.com/monetarium/monetarium-wallet/wallet/txrules"
)

// TestDualCoinTxRules tests the dual-coin transaction rules and coin type detection
func TestDualCoinTxRules(t *testing.T) {
	// Test case 1: Verify that VAR outputs result in VAR coin type detection
	varOutputs := []*wire.TxOut{
		{Value: 100000000, CoinType: cointype.CoinTypeVAR}, // 1 VAR
	}

	coinType := txrules.GetCoinTypeFromOutputs(varOutputs)
	if coinType != cointype.CoinTypeVAR {
		t.Errorf("Expected VAR coin type (0), got %d", coinType)
	}

	// Test case 2: Verify that SKA outputs result in correct SKA coin type detection
	skaOutputs := []*wire.TxOut{
		{Value: 200000000, CoinType: cointype.CoinType(1)}, // 2 SKA1
	}

	coinType = txrules.GetCoinTypeFromOutputs(skaOutputs)
	if coinType != cointype.CoinType(1) {
		t.Errorf("Expected SKA1 coin type (1), got %d", coinType)
	}

	// Test case 3: Empty outputs should return VAR as default
	emptyOutputs := []*wire.TxOut{}

	coinType = txrules.GetCoinTypeFromOutputs(emptyOutputs)
	if coinType != cointype.CoinTypeVAR {
		t.Errorf("Expected VAR coin type (0) for empty outputs, got %d", coinType)
	}

	// Note: Mixed coin types in outputs are no longer allowed in transactions.
	// All outputs must have the same coin type after SSFee implementation.
}

// TestTxAuthorFeeHandling tests that txauthor properly handles fees for all coin types
func TestTxAuthorFeeHandling(t *testing.T) {
	// Create mock input source for VAR that provides sufficient funds
	varInputSource := func(target dcrutil.Amount, targetSKA cointype.SKAAmount) (*txauthor.InputDetail, error) {
		// Provide inputs with enough value to cover target + fees
		mockInput := &wire.TxIn{
			PreviousOutPoint: wire.OutPoint{Index: 0},
			ValueIn:          int64(target + 1000), // Extra for fees
		}
		return &txauthor.InputDetail{
			Amount:            target + 1000,
			Inputs:            []*wire.TxIn{mockInput},
			RedeemScriptSizes: []int{25}, // P2PKH script size
		}, nil
	}

	// Create mock input source for SKA that provides sufficient funds
	skaInputSource := func(target dcrutil.Amount, targetSKA cointype.SKAAmount) (*txauthor.InputDetail, error) {
		// Provide inputs with enough value to cover the big.Int SKA target + fees
		skaAmount := int64(100000000 + 10000) // Output amount + extra for fees
		mockInput := &wire.TxIn{
			PreviousOutPoint: wire.OutPoint{Index: 0},
			ValueIn:          skaAmount,
		}
		return &txauthor.InputDetail{
			Amount:            dcrutil.Amount(skaAmount),
			SKAAmount:         cointype.SKAAmountFromInt64(skaAmount),
			Inputs:            []*wire.TxIn{mockInput},
			RedeemScriptSizes: []int{25}, // P2PKH script size
		}, nil
	}

	// Create mock change source
	changeSource := &mockChangeSource{
		script:     make([]byte, 25), // P2PKH script
		scriptSize: 25,
	}

	relayFeePerKb := cointype.SKAAmountFromInt64(1000) // 1000 atoms per KB

	// Test VAR transaction - should include fees
	varOutputs := []*wire.TxOut{
		{Value: 100000000, CoinType: cointype.CoinTypeVAR},
	}

	varTx, err := txauthor.NewUnsignedTransaction(varOutputs, relayFeePerKb, varInputSource, changeSource, 100000, -1)
	if err != nil {
		t.Fatalf("Failed to create VAR transaction: %v", err)
	}

	// Verify VAR transaction has fee
	varInputTotal := dcrutil.Amount(varTx.Tx.TxIn[0].ValueIn)
	varOutputTotal := dcrutil.Amount(0)
	for _, out := range varTx.Tx.TxOut {
		varOutputTotal += dcrutil.Amount(out.Value)
	}
	varFee := varInputTotal - varOutputTotal
	if varFee <= 0 {
		t.Errorf("VAR transaction should have positive fee, got %d", varFee)
	}

	// Test SKA transaction - should also include fees (fixed in our changes)
	// SKA outputs use SKAValue, not Value
	skaOutputs := []*wire.TxOut{
		{Value: 0, SKAValue: big.NewInt(100000000), CoinType: cointype.CoinType(1)},
	}

	skaTx, err := txauthor.NewUnsignedTransaction(skaOutputs, relayFeePerKb, skaInputSource, changeSource, 100000, -1)
	if err != nil {
		t.Fatalf("Failed to create SKA transaction: %v", err)
	}

	// Verify SKA transaction has fee (this would have failed before our fix)
	skaInputTotal := dcrutil.Amount(skaTx.Tx.TxIn[0].ValueIn)
	skaOutputTotal := dcrutil.Amount(0)
	for _, out := range skaTx.Tx.TxOut {
		// SKA outputs store value in SKAValue
		if out.SKAValue != nil {
			skaOutputTotal += dcrutil.Amount(out.SKAValue.Int64())
		} else {
			skaOutputTotal += dcrutil.Amount(out.Value)
		}
	}
	skaFee := skaInputTotal - skaOutputTotal
	if skaFee <= 0 {
		t.Errorf("SKA transaction should have positive fee, got %d", skaFee)
	}

	// Verify both transactions have similar fee rates
	if skaFee != varFee {
		t.Logf("VAR fee: %d, SKA fee: %d (difference: %d)", varFee, skaFee, skaFee-varFee)
		// Allow small differences due to transaction size variations
		if abs(int64(skaFee-varFee)) > 100 {
			t.Errorf("Fee difference between VAR and SKA transactions too large: %d", abs(int64(skaFee-varFee)))
		}
	}
}

// mockChangeSource implements txauthor.ChangeSource for testing
type mockChangeSource struct {
	script     []byte
	scriptSize int
}

func (m *mockChangeSource) Script() ([]byte, uint16, error) {
	return m.script, wire.DefaultPkScriptVersion, nil
}

func (m *mockChangeSource) ScriptSize() int {
	return m.scriptSize
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
