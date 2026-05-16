// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"context"
	"math/big"

	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-wallet/errors"
	"github.com/monetarium/monetarium-wallet/wallet/txrules"
	"github.com/monetarium/monetarium-wallet/wallet/txsizes"
	"github.com/monetarium/monetarium-wallet/wallet/udb"
	"github.com/monetarium/monetarium-wallet/wallet/walletdb"
	"github.com/monetarium/monetarium-node/txscript/stdaddr"
	"github.com/monetarium/monetarium-node/wire"
)

// FetchP2SHMultiSigOutput fetches information regarding a wallet's P2SH
// multi-signature output.
func (w *Wallet) FetchP2SHMultiSigOutput(ctx context.Context, outPoint *wire.OutPoint) (*P2SHMultiSigOutput, error) {
	const op errors.Op = "wallet.FetchP2SHMultiSigOutput"

	var (
		mso          *udb.MultisigOut
		redeemScript []byte
	)
	err := walletdb.View(ctx, w.db, func(tx walletdb.ReadTx) error {
		addrmgrNs := tx.ReadBucket(waddrmgrNamespaceKey)
		txmgrNs := tx.ReadBucket(wtxmgrNamespaceKey)
		var err error

		mso, err = w.txStore.GetMultisigOutput(txmgrNs, outPoint)
		if err != nil {
			return err
		}

		addr, _ := stdaddr.NewAddressScriptHashV0FromHash(mso.ScriptHash[:], w.chainParams)
		redeemScript, err = w.manager.RedeemScript(addrmgrNs, addr)
		return err
	})
	if err != nil {
		return nil, errors.E(op, err)
	}

	p2shAddr, err := stdaddr.NewAddressScriptHashV0FromHash(
		mso.ScriptHash[:], w.chainParams)
	if err != nil {
		return nil, err
	}

	multiSigOutput := P2SHMultiSigOutput{
		OutPoint:        *mso.OutPoint,
		OutputAmount:    mso.Amount,
		SKAOutputAmount: mso.SKAAmount,
		CoinType:        mso.CoinType,
		ContainingBlock: BlockIdentity{
			Hash:   mso.BlockHash,
			Height: int32(mso.BlockHeight),
		},
		P2SHAddress:  p2shAddr,
		RedeemScript: redeemScript,
		M:            mso.M,
		N:            mso.N,
		Redeemer:     nil,
	}

	if mso.Spent {
		multiSigOutput.Redeemer = &OutputRedeemer{
			TxHash:     mso.SpentBy,
			InputIndex: mso.SpentByIndex,
		}
	}

	return &multiSigOutput, nil
}

// PrepareRedeemMultiSigOutTxOutput estimates the tx value for a MultiSigOutTx
// output and adds it to msgTx. For SKA outputs (ct.IsSKA()) the fee and amount
// are computed with big.Int arithmetic via FeeForSerializeSizeSKA /
// RelayFeeForCoinType, and the output carries SKAValue with Value=0. For VAR
// the legacy int64 path is used.
func (w *Wallet) PrepareRedeemMultiSigOutTxOutput(ctx context.Context, msgTx *wire.MsgTx, p2shOutput *P2SHMultiSigOutput, pkScript *[]byte, ct cointype.CoinType) error {
	const op errors.Op = "wallet.PrepareRedeemMultiSigOutTxOutput"

	// The P2SH output being redeemed is a multisig (asserted by the
	// stdscript.STMultiSig check in the caller), so size each input's
	// worst-case sigScript with the multisig-aware helper. The legacy
	// RedeemP2SHSigScriptSize models a P2SH-wrapped P2PK and undercounts
	// real multisig sigScripts by 70-150 bytes — see size.go.
	sigScriptSize := txsizes.RedeemP2SHMultiSigSigScriptSize(
		int(p2shOutput.M), len(p2shOutput.RedeemScript))
	scriptSizes := make([]int, 0, len(msgTx.TxIn))
	for range msgTx.TxIn {
		scriptSizes = append(scriptSizes, sigScriptSize)
	}

	txOut := wire.NewTxOut(0, *pkScript)
	txOut.CoinType = ct

	if ct.IsSKA() {
		feeSize := txsizes.EstimateSerializeSizeSKA(scriptSizes, []*wire.TxOut{txOut}, 0)
		relayFee := w.RelayFeeForCoinType(ctx, ct)
		if relayFee.IsZero() {
			return errors.E(op, errors.Invalid, errors.Errorf(
				"no relay fee configured for coin type %d; cannot redeem multisig output", ct))
		}
		feeEst := txrules.FeeForSerializeSizeSKA(relayFee, feeSize)
		if p2shOutput.SKAOutputAmount.Cmp(feeEst) <= 0 {
			return errors.E(op, errors.Errorf("estimated SKA fee %v is at or above output value %v",
				feeEst, p2shOutput.SKAOutputAmount))
		}
		toReceive := p2shOutput.SKAOutputAmount.Sub(feeEst)
		if toReceive.BigInt().Cmp(cointype.MinSKADustAmount) < 0 {
			return errors.E(op, errors.Policy, errors.Errorf(
				"SKA redemption output %v below dust threshold %v atoms",
				toReceive, cointype.MinSKADustAmount))
		}
		txOut.Value = 0
		// Defense in depth: SKAAmount.BigInt() already returns a fresh
		// *big.Int per its current contract, so this copy is redundant
		// today. Kept so the wire TxOut stays safe if BigInt()'s
		// contract is ever weakened to alias the inner pointer.
		txOut.SKAValue = new(big.Int).Set(toReceive.BigInt())
		msgTx.AddTxOut(txOut)
		return nil
	}

	// VAR path.
	relayFee := w.RelayFee()
	feeSize := txsizes.EstimateSerializeSize(scriptSizes, []*wire.TxOut{txOut}, 0)
	feeEst := txrules.FeeForSerializeSize(relayFee, feeSize)
	if feeEst >= p2shOutput.OutputAmount {
		return errors.E(op, errors.Errorf("estimated fee %v is above output value %v",
			feeEst, p2shOutput.OutputAmount))
	}
	toReceive := p2shOutput.OutputAmount - feeEst
	if txrules.IsDustAmount(toReceive, len(*pkScript), relayFee) {
		return errors.E(op, errors.Policy, errors.Errorf(
			"VAR redemption output %v is dust at relay fee %v",
			toReceive, relayFee))
	}
	txOut.Value = int64(toReceive)
	msgTx.AddTxOut(txOut)
	return nil
}
