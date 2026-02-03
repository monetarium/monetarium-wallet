// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2016-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package txauthor provides transaction creation code for wallets.
package txauthor

import (
	"github.com/monetarium/monetarium-wallet/errors"
	"github.com/monetarium/monetarium-wallet/wallet/txrules"
	"github.com/monetarium/monetarium-wallet/wallet/txsizes"
	"github.com/monetarium/monetarium-node/chaincfg"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/crypto/rand"
	"github.com/monetarium/monetarium-node/dcrutil"
	"github.com/monetarium/monetarium-node/txscript"
	"github.com/monetarium/monetarium-node/txscript/sign"
	"github.com/monetarium/monetarium-node/wire"
)

const (
	// generatedTxVersion is the version of the transaction being generated.
	// It is defined as a constant here rather than using the wire.TxVersion
	// constant since a change in the transaction version will potentially
	// require changes to the generated transaction.  Thus, using the wire
	// constant for the generated transaction version could allow creation
	// of invalid transactions for the updated version.
	generatedTxVersion = 1
)

// InputDetail provides a detailed summary of transaction inputs
// referencing spendable outputs. This consists of the total spendable
// amount, the generated inputs, the redeem scripts and the full redeem
// script sizes.
type InputDetail struct {
	Amount            dcrutil.Amount
	SKAAmount         cointype.SKAAmount // For SKA coins that exceed int64
	Inputs            []*wire.TxIn
	Scripts           [][]byte
	RedeemScriptSizes []int
}

// InputSource provides transaction inputs referencing spendable outputs to
// construct a transaction outputting some target amount.  If the target amount
// can not be satisified, this can be signaled by returning a total amount less
// than the target or by returning a more detailed error.
type InputSource func(target dcrutil.Amount) (detail *InputDetail, err error)

// AuthoredTx holds the state of a newly-created transaction and the change
// output (if one was added).
type AuthoredTx struct {
	Tx                           *wire.MsgTx
	PrevScripts                  [][]byte
	TotalInput                   dcrutil.Amount
	SKATotalInput                cointype.SKAAmount // For SKA coins that exceed int64
	ChangeIndex                  int                // negative if no change
	EstimatedSignedSerializeSize int
}

// ChangeSource provides change output scripts and versions for
// transaction creation.
type ChangeSource interface {
	Script() (script []byte, version uint16, err error)
	ScriptSize() int
}

func sumOutputValues(outputs []*wire.TxOut) (totalOutput dcrutil.Amount) {
	for _, txOut := range outputs {
		totalOutput += dcrutil.Amount(txOut.Value)
	}
	return totalOutput
}

// sumSKAOutputValues sums the SKAValue fields from transaction outputs.
// This is used for SKA transactions where amounts exceed int64.
func sumSKAOutputValues(outputs []*wire.TxOut) cointype.SKAAmount {
	total := cointype.Zero()
	for _, txOut := range outputs {
		if txOut.SKAValue != nil {
			total = total.Add(cointype.NewSKAAmount(txOut.SKAValue))
		}
	}
	return total
}

// NewUnsignedTransaction creates an unsigned transaction paying to one or more
// non-change outputs.  An appropriate transaction fee is included based on the
// transaction size.
//
// Transaction inputs are chosen from repeated calls to fetchInputs with
// increasing targets amounts.
//
// If any remaining output value can be returned to the wallet via a change
// output without violating mempool dust rules, a P2PKH change output is
// appended to the transaction outputs.  Since the change output may not be
// necessary, fetchChange is called zero or one times to generate this script.
// This function must return a P2PKH script or smaller, otherwise fee estimation
// will be incorrect.
//
// If successful, the transaction, total input value spent, and all previous
// output scripts are returned.  If the input source was unable to provide
// enough input value to pay for every output any necessary fees, an
// InputSourceError is returned.
func NewUnsignedTransaction(outputs []*wire.TxOut, relayFeePerKb cointype.SKAAmount,
	fetchInputs InputSource, fetchChange ChangeSource, maxTxSize int) (*AuthoredTx, error) {

	const op errors.Op = "txauthor.NewUnsignedTransaction"

	// Determine if this is an SKA transaction
	isSKA := len(outputs) > 0 && outputs[0].CoinType.IsSKA()

	// For SKA, use big.Int amounts; for VAR, use int64
	targetAmount := sumOutputValues(outputs)
	targetSKAAmount := cointype.Zero()
	if isSKA {
		targetSKAAmount = sumSKAOutputValues(outputs)
	}

	scriptSizes := []int{txsizes.RedeemP2PKHSigScriptSize}
	changeScript, changeScriptVersion, err := fetchChange.Script()
	if err != nil {
		return nil, errors.E(op, err)
	}
	changeScriptSize := fetchChange.ScriptSize()
	var maxSignedSize int
	if isSKA {
		maxSignedSize = txsizes.EstimateSerializeSizeSKA(scriptSizes, outputs, changeScriptSize)
	} else {
		maxSignedSize = txsizes.EstimateSerializeSize(scriptSizes, outputs, changeScriptSize)
	}

	// Calculate initial fee for transaction size estimation using SKAAmount (big.Int)
	// SKA emission transactions have zero fees, all other transactions use normal fees
	targetFeeSKA := txrules.FeeForSerializeSizeSKA(relayFeePerKb, maxSignedSize)

	// Check if this is an SKA emission transaction (need to create temp tx to check)
	tempTx := &wire.MsgTx{
		SerType: wire.TxSerializeFull,
		Version: generatedTxVersion,
		TxOut:   outputs,
	}
	if wire.IsSKAEmissionTransaction(tempTx) {
		targetFeeSKA = cointype.Zero() // SKA emission transactions have zero fees
	}

	// Convert to dcrutil.Amount for VAR compatibility (VAR fees always fit in int64)
	targetFee := dcrutil.Amount(0)
	if !isSKA {
		targetFeeInt64, _ := targetFeeSKA.Int64()
		targetFee = dcrutil.Amount(targetFeeInt64)
	}

	for {
		// For SKA, pass target=0 to collect all available UTXOs since we can't
		// pass big.Int through the int64 parameter. Then check with big.Int.
		var inputTarget dcrutil.Amount
		if isSKA {
			inputTarget = 0 // Get all available SKA UTXOs
		} else {
			inputTarget = targetAmount + targetFee
		}

		inputDetail, err := fetchInputs(inputTarget)
		if err != nil {
			return nil, errors.E(op, err)
		}

		// Check if we have sufficient balance
		if isSKA {
			// For SKA, compare using big.Int (SKAAmount)
			targetWithFee := targetSKAAmount.Add(targetFeeSKA)
			if inputDetail.SKAAmount.Cmp(targetWithFee) < 0 {
				return nil, errors.E(op, errors.InsufficientBalance)
			}
		} else {
			if inputDetail.Amount < targetAmount+targetFee {
				return nil, errors.E(op, errors.InsufficientBalance)
			}
		}

		scriptSizes := make([]int, 0, len(inputDetail.RedeemScriptSizes))
		scriptSizes = append(scriptSizes, inputDetail.RedeemScriptSizes...)

		if isSKA {
			maxSignedSize = txsizes.EstimateSerializeSizeSKA(scriptSizes, outputs, changeScriptSize)
		} else {
			maxSignedSize = txsizes.EstimateSerializeSize(scriptSizes, outputs, changeScriptSize)
		}

		// Calculate fee based on actual transaction size using SKAAmount (big.Int)
		// Check if this is an SKA emission transaction for final fee calculation
		tempTxWithInputs := &wire.MsgTx{
			SerType: wire.TxSerializeFull,
			Version: generatedTxVersion,
			TxIn:    inputDetail.Inputs,
			TxOut:   outputs,
		}
		maxRequiredFeeSKA := txrules.FeeForSerializeSizeSKA(relayFeePerKb, maxSignedSize)
		if wire.IsSKAEmissionTransaction(tempTxWithInputs) {
			maxRequiredFeeSKA = cointype.Zero() // SKA emission transactions have zero fees
		}

		// Convert SKAAmount fee to dcrutil.Amount for VAR transactions
		maxRequiredFeeInt64, _ := maxRequiredFeeSKA.Int64()
		maxRequiredFee := dcrutil.Amount(maxRequiredFeeInt64)

		// Check remaining amount covers fees
		if isSKA {
			remainingSKA := inputDetail.SKAAmount.Sub(targetSKAAmount)
			if remainingSKA.Cmp(maxRequiredFeeSKA) < 0 {
				targetFeeSKA = maxRequiredFeeSKA
				continue
			}
		} else {
			remainingAmount := inputDetail.Amount - targetAmount
			if remainingAmount < maxRequiredFee {
				targetFee = maxRequiredFee
				continue
			}
		}

		if maxSignedSize > maxTxSize {
			return nil, errors.E(errors.Invalid, "signed tx size exceeds allowed maximum")
		}

		unsignedTransaction := &wire.MsgTx{
			SerType:  wire.TxSerializeFull,
			Version:  generatedTxVersion,
			TxIn:     inputDetail.Inputs,
			TxOut:    outputs,
			LockTime: 0,
			Expiry:   0,
		}
		changeIndex := -1

		// Calculate change amount based on coin type
		var changeAmount dcrutil.Amount
		var changeSKAAmount cointype.SKAAmount
		if isSKA {
			// SKA: use SKAAmount (big.Int) for full precision
			changeSKAAmount = inputDetail.SKAAmount.Sub(targetSKAAmount).Sub(maxRequiredFeeSKA)
		} else {
			// VAR: use dcrutil.Amount (int64)
			changeAmount = inputDetail.Amount - targetAmount - maxRequiredFee
		}

		// Check if change output should be added
		var hasChange bool
		if isSKA {
			// For SKA, minimum 30 atoms to avoid dust (28 atoms is minimal transfer fee)
			hasChange = !changeSKAAmount.IsNegative() && changeSKAAmount.BigInt().Cmp(cointype.MinSKADustAmount) >= 0
		} else {
			// For VAR, use dust check with fee rate converted to dcrutil.Amount
			dustFeeRateInt64, _ := relayFeePerKb.Int64()
			hasChange = changeAmount != 0 && !txrules.IsDustAmount(changeAmount, changeScriptSize, dcrutil.Amount(dustFeeRateInt64))
		}

		if hasChange {
			if len(changeScript) > txscript.MaxScriptElementSize {
				return nil, errors.E(errors.Invalid, "script size exceed maximum bytes "+
					"pushable to the stack")
			}

			// Set the coin type for the change output to match the transaction
			var changeCoinType cointype.CoinType = cointype.CoinTypeVAR // Default to VAR
			if len(outputs) > 0 {
				changeCoinType = outputs[0].CoinType
			}

			change := &wire.TxOut{
				Version:  changeScriptVersion,
				PkScript: changeScript,
				CoinType: changeCoinType,
			}

			// Set value based on coin type
			if isSKA {
				change.Value = 0 // SKA uses SKAValue, not Value
				change.SKAValue = changeSKAAmount.BigInt()
			} else {
				change.Value = int64(changeAmount)
			}

			l := len(outputs)
			unsignedTransaction.TxOut = append(outputs[:l:l], change)
			changeIndex = l
		} else {
			if isSKA {
				maxSignedSize = txsizes.EstimateSerializeSizeSKA(scriptSizes,
					unsignedTransaction.TxOut, 0)
			} else {
				maxSignedSize = txsizes.EstimateSerializeSize(scriptSizes,
					unsignedTransaction.TxOut, 0)
			}
		}
		return &AuthoredTx{
			Tx:                           unsignedTransaction,
			PrevScripts:                  inputDetail.Scripts,
			TotalInput:                   inputDetail.Amount,
			SKATotalInput:                inputDetail.SKAAmount,
			ChangeIndex:                  changeIndex,
			EstimatedSignedSerializeSize: maxSignedSize,
		}, nil
	}
}

// RandomizeOutputPosition randomizes the position of a transaction's output by
// swapping it with a random output.  The new index is returned.  This should be
// done before signing.
func RandomizeOutputPosition(outputs []*wire.TxOut, index int) int {
	r := rand.Int32N(int32(len(outputs)))
	outputs[r], outputs[index] = outputs[index], outputs[r]
	return int(r)
}

// RandomizeChangePosition randomizes the position of an authored transaction's
// change output.  This should be done before signing.
func (tx *AuthoredTx) RandomizeChangePosition() {
	tx.ChangeIndex = RandomizeOutputPosition(tx.Tx.TxOut, tx.ChangeIndex)
}

// SecretsSource provides private keys and redeem scripts necessary for
// constructing transaction input signatures.  Secrets are looked up by the
// corresponding Address for the previous output script.  Addresses for lookup
// are created using the source's blockchain parameters and means a single
// SecretsSource can only manage secrets for a single chain.
//
// TODO: Rewrite this interface to look up private keys and redeem scripts for
// pubkeys, pubkey hashes, script hashes, etc. as separate interface methods.
// This would remove the ChainParams requirement of the interface and could
// avoid unnecessary conversions from previous output scripts to Addresses.
// This can not be done without modifications to the txscript package.
type SecretsSource interface {
	sign.KeyDB
	sign.ScriptDB
	ChainParams() *chaincfg.Params
}

// AddAllInputScripts modifies transaction a transaction by adding inputs
// scripts for each input.  Previous output scripts being redeemed by each input
// are passed in prevPkScripts and the slice length must match the number of
// inputs.  Private keys and redeem scripts are looked up using a SecretsSource
// based on the previous output script.
func AddAllInputScripts(tx *wire.MsgTx, prevPkScripts [][]byte, secrets SecretsSource) error {
	inputs := tx.TxIn
	chainParams := secrets.ChainParams()

	if len(inputs) != len(prevPkScripts) {
		return errors.New("tx.TxIn and prevPkScripts slices must " +
			"have equal length")
	}

	for i := range inputs {
		pkScript := prevPkScripts[i]
		sigScript := inputs[i].SignatureScript
		script, err := sign.SignTxOutput(chainParams, tx, i,
			pkScript, txscript.SigHashAll, secrets, secrets,
			sigScript, true) // Yes treasury
		if err != nil {
			return err
		}
		inputs[i].SignatureScript = script
	}

	return nil
}

// AddAllInputScripts modifies an authored transaction by adding inputs scripts
// for each input of an authored transaction.  Private keys and redeem scripts
// are looked up using a SecretsSource based on the previous output script.
func (tx *AuthoredTx) AddAllInputScripts(secrets SecretsSource) error {
	return AddAllInputScripts(tx.Tx, tx.PrevScripts, secrets)
}
