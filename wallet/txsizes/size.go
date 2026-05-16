// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2016-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txsizes

import (
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/wire"
)

// Worst case script and input/output size estimates.
const (
	// RedeemP2PKSigScriptSize is the worst case (largest) serialize size
	// of a transaction input script that redeems a compressed P2PK output.
	// It is calculated as:
	//
	//   - OP_DATA_73
	//   - 72 bytes DER signature + 1 byte sighash
	RedeemP2PKSigScriptSize = 1 + 73

	// RedeemP2PKHSigScriptSize is the worst case (largest) serialize size
	// of a transaction input script that redeems a compressed P2PKH output.
	// It is calculated as:
	//
	//   - OP_DATA_73
	//   - 72 bytes DER signature + 1 byte sighash
	//   - OP_DATA_33
	//   - 33 bytes serialized compressed pubkey
	RedeemP2PKHSigScriptSize = 1 + 73 + 1 + 33

	// RedeemP2SHSigScriptSize is the worst case (largest) serialize size
	// of a transaction input script that redeems a P2SH-wrapped P2PK
	// output (single-signature, single-pubkey redeem script). It is NOT
	// correct for redeeming a P2SH-wrapped N-of-M multisig — use
	// RedeemP2SHMultiSigSigScriptSize for that case. It is calculated as:
	//
	//  - OP_DATA_73
	//  - 73-byte signature
	//  - OP_DATA_35
	//  - OP_DATA_33
	//  - 33 bytes serialized compressed pubkey
	//  - OP_CHECKSIG
	RedeemP2SHSigScriptSize = 1 + 73 + 1 + 1 + 33 + 1

	// RedeemP2PKHInputSize is the worst case (largest) serialize size of a
	// transaction input redeeming a compressed P2PKH output.  It is
	// calculated as:
	//
	//   - 32 bytes previous tx
	//   - 4 bytes output index
	//   - 1 byte tree
	//   - 8 bytes amount
	//   - 4 bytes block height
	//   - 4 bytes block index
	//   - 1 byte compact int encoding value 107
	//   - 107 bytes signature script
	//   - 4 bytes sequence
	RedeemP2PKHInputSize = 32 + 4 + 1 + 8 + 4 + 4 + 1 + RedeemP2PKHSigScriptSize + 4

	// P2PKHPkScriptSize is the size of a transaction output script that
	// pays to a compressed pubkey hash.  It is calculated as:
	//
	//   - OP_DUP
	//   - OP_HASH160
	//   - OP_DATA_20
	//   - 20 bytes pubkey hash
	//   - OP_EQUALVERIFY
	//   - OP_CHECKSIG
	P2PKHPkScriptSize = 1 + 1 + 1 + 20 + 1 + 1

	// P2PKHPkTreasruryScriptSize is the size of a transaction output
	// script that pays stake change to a compressed pubkey hash.  This is
	// used when a user sends coins to the treasury via OP_TADD.  It is
	// calculated as:
	//
	//   - OP_SSTXCHANGE
	//   - OP_DUP
	//   - OP_HASH160
	//   - OP_DATA_20
	//   - 20 bytes pubkey hash
	//   - OP_EQUALVERIFY
	//   - OP_CHECKSIG
	P2PKHPkTreasruryScriptSize = 1 + 1 + 1 + 1 + 20 + 1 + 1

	// P2SHPkScriptSize is the size of a transaction output script that
	// pays to a script hash.  It is calculated as:
	//
	//   - OP_HASH160
	//   - OP_DATA_20
	//   - 20 bytes script hash
	//   - OP_EQUAL
	P2SHPkScriptSize = 1 + 1 + 20 + 1

	// TicketCommitmentScriptSize is the size of a ticket purchase commitment
	// script. It is calculated as:
	//
	//   - OP_RETURN
	//   - OP_DATA_30
	//   - 20 bytes P2SH/P2PKH
	//   - 8 byte amount
	//   - 2 byte fee range limits
	TicketCommitmentScriptSize = 1 + 1 + 20 + 8 + 2

	// P2PKHOutputSize is the serialize size of a transaction output with a
	// P2PKH output script.  It is calculated as:
	//
	//   - 8 bytes output value
	//   - 1 byte coin type (dual-coin support)
	//   - 2 bytes version
	//   - 1 byte compact int encoding value 25
	//   - 25 bytes P2PKH output script
	P2PKHOutputSize = 8 + 1 + 2 + 1 + 25

	// TSPENDInputSize
	//
	//   - OP_DATA_73
	//   - 73 bytes signature
	//   - OP_DATA_33
	//   - 33 bytes serialized compressed pubkey
	//   - 1 byte OP_TSPEND
	TSPENDInputSize = 1 + 73 + 1 + 33 + 1
)

// RedeemP2SHMultiSigSigScriptSize returns the worst-case serialize size of a
// transaction input sigScript that redeems a P2SH-wrapped N-of-M multisig
// output. requiredSigs is M (the number of signatures needed); redeemScriptLen
// is the byte length of the opcode-serialized multisig redeem script
// (`OP_M <push33><pubkey>... OP_N OP_CHECKMULTISIG`, i.e. 34*N + 3 bytes).
//
// Layout for a fully-signed P2SH-multisig (Decred fork omits the OP_0
// CHECKMULTISIG dummy — see txscript/sign/sign.go:signMultiSig):
//
//   - requiredSigs × (OP_DATA_73 + 73 signature bytes)   // 74 bytes per sig
//   - <push opcode> redeemScript                          // 1-3 bytes + script
//
// Each signature is the worst-case 72-byte DER ECDSA signature plus a 1-byte
// sighash flag, pushed with OP_DATA_73 (1 opcode byte). The redeem-script
// push opcode is OP_DATA_N for <=75 bytes and OP_PUSHDATA{1,2,4} otherwise.
func RedeemP2SHMultiSigSigScriptSize(requiredSigs, redeemScriptLen int) int {
	var pushPrefix int
	switch {
	case redeemScriptLen <= 75:
		pushPrefix = 1 // OP_DATA_N (single opcode encoding length)
	case redeemScriptLen <= 255:
		pushPrefix = 2 // OP_PUSHDATA1 + 1-byte length
	case redeemScriptLen <= 65535:
		pushPrefix = 3 // OP_PUSHDATA2 + 2-byte length
	default:
		pushPrefix = 5 // OP_PUSHDATA4 + 4-byte length
	}
	return requiredSigs*(1+73) + pushPrefix + redeemScriptLen
}

func sumOutputSerializeSizes(outputs []*wire.TxOut) (serializeSize int) {
	for _, txOut := range outputs {
		serializeSize += txOut.SerializeSize()
	}
	return serializeSize
}

// EstimateSerializeSize returns a worst case serialize size estimate for a
// signed VAR transaction that spends a number of outputs and contains each
// transaction output from txOuts. The estimated size is incremented for an
// additional change output if changeScriptSize is greater than 0. Passing 0
// does not add a change output.
//
// VAR-only: callers building SKA transactions must use
// EstimateSerializeSizeForCoinType (or the explicit SKA variant) so the
// witness-bytes accounting picks the correct SKAValueIn worst-case width.
func EstimateSerializeSize(scriptSizes []int, txOuts []*wire.TxOut, changeScriptSize int) int {
	return estimateSerializeSizeInternal(scriptSizes, txOuts, changeScriptSize, false)
}

// EstimateSerializeSizeSKA returns a worst case serialize size estimate for a
// signed SKA transaction. SKA outputs have a slightly different wire format
// (1-byte length prefix for the value), so change output estimation differs.
func EstimateSerializeSizeSKA(scriptSizes []int, txOuts []*wire.TxOut, changeScriptSize int) int {
	return estimateSerializeSizeInternal(scriptSizes, txOuts, changeScriptSize, true)
}

// EstimateSerializeSizeForCoinType is the coin-type-parameterised wrapper
// over EstimateSerializeSize / EstimateSerializeSizeSKA. Prefer this entry
// point so a single dispatch on ct routes to the correct witness/output
// width — calling the VAR variant on a slice that contains SKA outputs
// silently under-estimates the witness bytes.
func EstimateSerializeSizeForCoinType(ct cointype.CoinType, scriptSizes []int, txOuts []*wire.TxOut, changeScriptSize int) int {
	return estimateSerializeSizeInternal(scriptSizes, txOuts, changeScriptSize, ct.IsSKA())
}

func estimateSerializeSizeInternal(scriptSizes []int, txOuts []*wire.TxOut, changeScriptSize int, isSKA bool) int {
	inputCount := len(scriptSizes)
	outputCount := len(txOuts)
	changeSize := 0
	if changeScriptSize != 0 {
		if isSKA {
			changeSize = EstimateOutputSizeSKA(changeScriptSize)
		} else {
			changeSize = EstimateOutputSize(changeScriptSize)
		}
		outputCount++
	}

	// Calculate size for TxSerializeFull format (prefix + witness)
	// This matches the format used in wire.MsgTx.SerializeSize() for TxSerializeFull

	// Base: Version 4 bytes + LockTime 4 bytes + Expiry 4 bytes = 12 bytes
	// Plus varint sizes for input count (x2) and output count
	baseSize := 12 + wire.VarIntSerializeSize(uint64(inputCount)) +
		wire.VarIntSerializeSize(uint64(inputCount)) +
		wire.VarIntSerializeSize(uint64(outputCount))

	// Calculate prefix input sizes (without witness data)
	prefixInputsSize := 0
	for range scriptSizes {
		prefixInputsSize += EstimateInputPrefixSize()
	}

	// Calculate witness input sizes (signature scripts)
	// V13 format: [ValueIn:8][SKAValueInLen:1][SKAValueIn:N][BlockHeight:4][BlockIndex:4][SigScript:var]
	// For SKA inputs, SKAValueIn can be up to 16 bytes (worst case)
	witnessInputsSize := 0
	for _, scriptSize := range scriptSizes {
		if isSKA {
			witnessInputsSize += EstimateInputWitnessSizeSKA(scriptSize)
		} else {
			witnessInputsSize += EstimateInputWitnessSize(scriptSize)
		}
	}

	// Calculate output sizes (includes CoinType field for dual-coin)
	outputsSize := sumOutputSerializeSizes(txOuts) + changeSize

	return baseSize + prefixInputsSize + witnessInputsSize + outputsSize
}

// EstimateSerializeSizeFromScriptSizes returns a worst case serialize size
// estimate for a signed transaction that spends len(inputSizes) previous
// outputs and pays to len(outputSizes) outputs with scripts of the provided
// worst-case sizes. The estimated size is incremented for an additional
// change output if changeScriptSize is greater than 0. Passing 0 does not
// add a change output.
func EstimateSerializeSizeFromScriptSizes(inputSizes []int, outputSizes []int, changeScriptSize int) int {
	// Generate and sum up the estimated sizes of the inputs.
	txInsSize := 0
	for _, inputSize := range inputSizes {
		txInsSize += EstimateInputSize(inputSize)
	}

	// Generate and sum up the estimated sizes of the outputs.
	txOutsSize := 0
	for _, outputSize := range outputSizes {
		txOutsSize += EstimateOutputSize(outputSize)
	}

	inputCount := len(inputSizes)
	outputCount := len(outputSizes)
	changeSize := 0
	if changeScriptSize > 0 {
		changeSize = EstimateOutputSize(changeScriptSize)
		outputCount++
	}

	// 12 additional bytes are for version, locktime and expiry.
	return 12 + (2 * wire.VarIntSerializeSize(uint64(inputCount))) +
		wire.VarIntSerializeSize(uint64(outputCount)) +
		txInsSize + txOutsSize + changeSize
}

// EstimateInputSize returns the worst case serialize size estimate for a tx input
//   - 32 bytes previous tx
//   - 4 bytes output index
//   - 1 byte tree
//   - 8 bytes amount (ValueIn)
//   - 1 byte SKAValueInLen (V13: always present, 0 for VAR inputs)
//   - 4 bytes block height
//   - 4 bytes block index
//   - the compact int representation of the script size
//   - the supplied script size
//   - 4 bytes sequence
func EstimateInputSize(scriptSize int) int {
	return 32 + 4 + 1 + 8 + 1 + 4 + 4 + wire.VarIntSerializeSize(uint64(scriptSize)) + scriptSize + 4
}

// EstimateOutputSize returns the worst case serialize size estimate for a tx output
//   - 8 bytes amount
//   - 1 byte coin type (dual-coin support)
//   - 2 bytes version
//   - the compact int representation of the script size
//   - the supplied script size
func EstimateOutputSize(scriptSize int) int {
	return 8 + 1 + 2 + wire.VarIntSerializeSize(uint64(scriptSize)) + scriptSize
}

// MaxSKAValueBytes is the worst-case length in bytes of a serialized SKA
// value (big.Int atoms). The wire format encodes any value up to 255 bytes,
// but for fee estimation we use this bound — chain-params validation rejects
// any SKA coin whose MaxSupply exceeds it, so the assumption is safe.
//
// SKA1 currently configured at 900T coins × 1e18 atoms/coin ≈ 14 bytes; 16
// gives ~2 bytes of headroom for future configurations within the same order
// of magnitude. Coins that need more must update this constant and the
// chain-params validator together.
const MaxSKAValueBytes = 16

// EstimateOutputSizeSKA returns the serialize size estimate for an SKA tx output.
// SKA outputs have a different format from VAR:
//   - 1 byte coin type
//   - 1 byte value length prefix
//   - N bytes value (up to MaxSKAValueBytes)
//   - 2 bytes version
//   - the compact int representation of the script size
//   - the supplied script size
func EstimateOutputSizeSKA(scriptSize int) int {
	return 1 + 1 + MaxSKAValueBytes + 2 + wire.VarIntSerializeSize(uint64(scriptSize)) + scriptSize
}

// EstimateInputPrefixSize returns the serialize size estimate for a tx input prefix
//   - 32 bytes previous tx
//   - 4 bytes output index
//   - 1 byte tree
//   - 4 bytes sequence
func EstimateInputPrefixSize() int {
	return 32 + 4 + 1 + 4
}

// EstimateInputWitnessSize returns the serialize size estimate for a tx input witness
// V13 format: [ValueIn:8][SKAValueInLen:1][SKAValueIn:N][BlockHeight:4][BlockIndex:4][SigScript:var]
//   - 8 bytes amount (ValueIn for fraud proofs)
//   - 1 byte SKAValueInLen (V13: always present, 0 for VAR inputs)
//   - 4 bytes block height
//   - 4 bytes block index
//   - the compact int representation of the script size
//   - the supplied script size
func EstimateInputWitnessSize(scriptSize int) int {
	// V13 format includes SKAValueInLen (1 byte), which is 0 for VAR inputs
	return 8 + 1 + 4 + 4 + wire.VarIntSerializeSize(uint64(scriptSize)) + scriptSize
}

// EstimateInputWitnessSizeSKA returns the serialize size estimate for an SKA tx input witness.
// SKA inputs include SKAValueIn which can be up to MaxSKAValueBytes for large
// amounts. The bound is enforced by chain-params validation at wallet open.
func EstimateInputWitnessSizeSKA(scriptSize int) int {
	// V13 SKA format: ValueIn(8) + SKAValueInLen(1) + SKAValueIn(MaxSKAValueBytes) + BlockHeight(4) + BlockIndex(4) + VarInt + SigScript
	return 8 + 1 + MaxSKAValueBytes + 4 + 4 + wire.VarIntSerializeSize(uint64(scriptSize)) + scriptSize
}
