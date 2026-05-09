// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"bytes"
	"context"
	"math/big"
	"sync"

	"github.com/monetarium/monetarium-wallet/errors"
	"github.com/monetarium/monetarium-wallet/internal/compat"
	"github.com/monetarium/monetarium-wallet/wallet/udb"
	"github.com/monetarium/monetarium-wallet/wallet/walletdb"
	"github.com/monetarium/monetarium-node/blockchain/stake"
	"github.com/monetarium/monetarium-node/chaincfg/chainhash"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/dcrutil"
	"github.com/monetarium/monetarium-node/hdkeychain"
	"github.com/monetarium/monetarium-node/txscript/stdaddr"
	"github.com/monetarium/monetarium-node/txscript/stdscript"
	"github.com/monetarium/monetarium-node/wire"
)

// TODO: It would be good to send errors during notification creation to the rpc
// server instead of just logging them here so the client is aware that wallet
// isn't working correctly and notifications are missing.

// TODO: Anything dealing with accounts here is expensive because the database
// is not organized correctly for true account support, but do the slow thing
// instead of the easy thing since the db can be fixed later, and we want the
// api correct now.

// NotificationServer is a server that interested clients may hook into to
// receive notifications of changes in a wallet.  A client is created for each
// registered notification.  Clients are guaranteed to receive messages in the
// order wallet created them, but there is no guaranteed synchronization between
// different clients.
type NotificationServer struct {
	transactions []chan *TransactionNotifications
	// Coalesce transaction notifications since wallet previously did not add
	// mined txs together.  Now it does and this can be rewritten.
	currentTxNtfn             *TransactionNotifications
	accountClients            []chan *AccountNotification
	tipChangedClients         []chan *MainTipChangedNotification
	confClients               []*ConfirmationNotificationsClient
	removedTransactionClients []chan *RemovedTransactionNotification
	mu                        sync.Mutex // Only protects registered clients
	wallet                    *Wallet    // smells like hacks
}

func newNotificationServer(wallet *Wallet) *NotificationServer {
	return &NotificationServer{
		wallet: wallet,
	}
}

func lookupInputAccount(dbtx walletdb.ReadTx, w *Wallet, details *udb.TxDetails, deb udb.DebitRecord) uint32 {
	addrmgrNs := dbtx.ReadBucket(waddrmgrNamespaceKey)
	txmgrNs := dbtx.ReadBucket(wtxmgrNamespaceKey)

	// TODO: Debits should record which account(s?) they
	// debit from so this doesn't need to be looked up.
	prevOP := &details.MsgTx.TxIn[deb.Index].PreviousOutPoint
	prev, err := w.txStore.TxDetails(txmgrNs, &prevOP.Hash)
	if err != nil {
		log.Errorf("Cannot query previous transaction details for %v: %v", prevOP.Hash, err)
		return 0
	}
	if prev == nil {
		log.Errorf("Missing previous transaction %v", prevOP.Hash)
		return 0
	}
	prevOut := prev.MsgTx.TxOut[prevOP.Index]
	_, addrs := stdscript.ExtractAddrs(prevOut.Version, prevOut.PkScript, w.chainParams)
	if len(addrs) == 0 {
		return 0
	}

	inputAcct, err := w.manager.AddrAccount(addrmgrNs, addrs[0])
	if err != nil {
		log.Errorf("Cannot fetch account for previous output %v: %v", prevOP, err)
		return 0
	}
	return inputAcct
}

func lookupOutputChain(dbtx walletdb.ReadTx, w *Wallet, details *udb.TxDetails,
	cred udb.CreditRecord) (account uint32, internal bool, address stdaddr.Address,
	amount int64, skaAmount *big.Int, outputScript []byte) {

	addrmgrNs := dbtx.ReadBucket(waddrmgrNamespaceKey)

	output := details.MsgTx.TxOut[cred.Index]
	_, addrs := stdscript.ExtractAddrs(output.Version, output.PkScript, w.chainParams)
	if len(addrs) == 0 {
		return
	}

	ma, err := w.manager.Address(addrmgrNs, addrs[0])
	if err != nil {
		log.Errorf("Cannot fetch account for wallet output: %v", err)
		return
	}
	account = ma.Account()
	internal = ma.Internal()
	address = ma.Address()
	if output.CoinType.IsSKA() {
		if output.SKAValue != nil {
			skaAmount = new(big.Int).Set(output.SKAValue)
		}
		amount = 0
	} else {
		amount = output.Value
	}
	outputScript = output.PkScript
	return
}

func makeTxSummary(dbtx walletdb.ReadTx, w *Wallet, details *udb.TxDetails) TransactionSummary {
	serializedTx := details.SerializedTx
	if serializedTx == nil {
		var buf bytes.Buffer
		buf.Grow(details.MsgTx.SerializeSize())
		err := details.MsgTx.Serialize(&buf)
		if err != nil {
			log.Errorf("Transaction serialization: %v", err)
		}
		serializedTx = buf.Bytes()
	}

	// Identify the transaction's coin type from its outputs. Wallet-built
	// transactions are always single-coin-type (consensus rejects mixed),
	// so the first output is authoritative. Empty TxOut list defaults to VAR.
	var txCoinType cointype.CoinType
	if len(details.MsgTx.TxOut) > 0 {
		txCoinType = details.MsgTx.TxOut[0].CoinType
	}

	// Compute fee. For VAR: int64 atoms (legacy contract) — only when every
	// input is a debit (the wallet's debit record is the only source of
	// input atoms). For SKA: big.Int atoms via SKAValueIn / SKAValue —
	// SKAValueIn is bound on the wire so the value is authoritative and the
	// wallet does not need to own every input; the gate is "every input has
	// a non-nil SKAValueIn", which lets us report fees on partially-owned
	// SKA txs (e.g. coinjoin counterparty inputs).
	var fee dcrutil.Amount
	var skaFee *big.Int
	if txCoinType.IsSKA() {
		in := new(big.Int)
		haveAllInputs := true
		for _, txIn := range details.MsgTx.TxIn {
			if txIn.SKAValueIn == nil {
				haveAllInputs = false
				break
			}
			in.Add(in, txIn.SKAValueIn)
		}
		if haveAllInputs {
			out := new(big.Int)
			for _, txOut := range details.MsgTx.TxOut {
				if txOut.SKAValue != nil {
					out.Add(out, txOut.SKAValue)
				}
			}
			diff := new(big.Int).Sub(in, out)
			if diff.Sign() >= 0 {
				skaFee = diff
			}
		}
	} else if len(details.Debits) == len(details.MsgTx.TxIn) {
		for _, deb := range details.Debits {
			fee += deb.Amount
		}
		for _, txOut := range details.MsgTx.TxOut {
			fee -= dcrutil.Amount(txOut.Value)
		}
	}

	var inputs []TransactionSummaryInput
	if len(details.Debits) != 0 {
		inputs = make([]TransactionSummaryInput, len(details.Debits))
		for i, d := range details.Debits {
			var prevSKA *big.Int
			if v := details.MsgTx.TxIn[d.Index].SKAValueIn; v != nil {
				prevSKA = new(big.Int).Set(v)
			}
			inputs[i] = TransactionSummaryInput{
				Index:             d.Index,
				PreviousAccount:   lookupInputAccount(dbtx, w, details, d),
				PreviousAmount:    d.Amount,
				PreviousSKAAmount: prevSKA,
			}
		}
	}
	// Iterate Credits directly and look up the corresponding TxOut by the
	// credit's recorded Index. Walking MsgTx.TxOut in lockstep with a
	// running outputs-collected counter conflates "next credit to consider"
	// with "outputs collected so far" and silently drops wallet outputs the
	// moment credits are non-contiguous (e.g. wallet owns outputs 0 and 3
	// of a 4-output tx).
	outputs := make([]TransactionSummaryOutput, 0, len(details.Credits))
	for _, credit := range details.Credits {
		if int(credit.Index) >= len(details.MsgTx.TxOut) {
			continue
		}
		acct, internal, address, amount, skaAmount, outputScript := lookupOutputChain(dbtx, w, details, credit)
		outputs = append(outputs, TransactionSummaryOutput{
			Index:        credit.Index,
			Account:      acct,
			Internal:     internal,
			Amount:       dcrutil.Amount(amount),
			SKAAmount:    skaAmount,
			Address:      address,
			OutputScript: outputScript,
		})
	}

	var transactionType = TxTransactionType(&details.MsgTx)

	// Use earliest of receive time or block time if the transaction is mined.
	receiveTime := details.Received
	if details.Height() >= 0 && details.Block.Time.Before(receiveTime) {
		receiveTime = details.Block.Time
	}

	return TransactionSummary{
		Hash:        &details.Hash,
		Transaction: serializedTx,
		MyInputs:    inputs,
		MyOutputs:   outputs,
		Fee:         fee,
		SKAFee:      skaFee,
		Timestamp:   receiveTime.Unix(),
		Type:        transactionType,
	}
}

// totalBalances populates m with VAR-only balances per account. SKA balances
// are intentionally excluded — VAR atoms (1e8/coin, int64) and SKA atoms
// (1e18/coin, big.Int) cannot be summed into a single int64 without overflow
// or unit confusion. Use totalSKABalances for per-coin SKA totals and surface
// them via AccountBalance.SKACoinTypeBalances.
func totalBalances(dbtx walletdb.ReadTx, w *Wallet, m map[uint32]dcrutil.Amount) error {
	addrmgrNs := dbtx.ReadBucket(waddrmgrNamespaceKey)

	outputs, err := w.txStore.UnspentOutputs(dbtx, cointype.CoinTypeVAR)
	if err != nil {
		return err
	}
	for _, output := range outputs {
		_, addrs := stdscript.ExtractAddrs(scriptVersionAssumed, output.PkScript, w.chainParams)
		if len(addrs) == 0 {
			continue
		}
		outputAcct, err := w.manager.AddrAccount(addrmgrNs, addrs[0])
		if err == nil {
			if _, ok := m[outputAcct]; ok {
				m[outputAcct] += output.Amount
			}
		}
	}
	return nil
}

// totalSKABalances populates m with per-account, per-active-SKA-coin-type
// balances using big.Int amounts to avoid int64 truncation. m must be
// pre-seeded with the relevant accounts so that only requested accounts are
// updated (matching the totalBalances contract).
func totalSKABalances(dbtx walletdb.ReadTx, w *Wallet,
	m map[uint32]map[cointype.CoinType]*big.Int) error {

	if w.chainParams == nil || w.chainParams.SKACoins == nil {
		return nil
	}
	addrmgrNs := dbtx.ReadBucket(waddrmgrNamespaceKey)
	for ct, config := range w.chainParams.SKACoins {
		if config == nil || !config.Active {
			continue
		}
		outputs, err := w.txStore.UnspentOutputs(dbtx, ct)
		if err != nil {
			return err
		}
		for _, output := range outputs {
			_, addrs := stdscript.ExtractAddrs(scriptVersionAssumed, output.PkScript, w.chainParams)
			if len(addrs) == 0 {
				continue
			}
			acct, err := w.manager.AddrAccount(addrmgrNs, addrs[0])
			if err != nil {
				continue
			}
			perCoin, ok := m[acct]
			if !ok {
				continue
			}
			cur, ok := perCoin[ct]
			if !ok {
				cur = new(big.Int)
				perCoin[ct] = cur
			}
			cur.Add(cur, output.SKAAmount.BigInt())
		}
	}
	return nil
}

func flattenBalanceMap(m map[uint32]dcrutil.Amount) []AccountBalance {
	return flattenBalanceMapWithSKA(m, nil)
}

// flattenBalanceMapWithSKA produces AccountBalance entries combining VAR
// totals (varM) with optional per-account SKA-by-coin-type totals (skaM).
// Pass nil for skaM to omit SKA balances entirely.
func flattenBalanceMapWithSKA(varM map[uint32]dcrutil.Amount,
	skaM map[uint32]map[cointype.CoinType]*big.Int) []AccountBalance {

	s := make([]AccountBalance, 0, len(varM))
	for acct, vbal := range varM {
		ab := AccountBalance{
			Account:             acct,
			TotalBalance:        vbal,
			CoinTypeBalances:    map[cointype.CoinType]dcrutil.Amount{cointype.CoinTypeVAR: vbal},
			SKACoinTypeBalances: make(map[cointype.CoinType]*big.Int),
		}
		if perCoin, ok := skaM[acct]; ok {
			for ct, amt := range perCoin {
				if amt != nil {
					ab.SKACoinTypeBalances[ct] = new(big.Int).Set(amt)
				}
			}
		}
		s = append(s, ab)
	}
	return s
}

// flattenMultiCoinBalanceMap converts multi-coin balance map to AccountBalance slice.
// Note: this helper takes int64 amounts; SKA values that exceed int64 must be
// surfaced through the SKACoinTypeBalances map populated by callers that have
// the big.Int totals available.
func flattenMultiCoinBalanceMap(accountCoinBalances map[uint32]map[cointype.CoinType]dcrutil.Amount) []AccountBalance {
	s := make([]AccountBalance, 0, len(accountCoinBalances))

	for account, coinBalances := range accountCoinBalances {
		accountBalance := AccountBalance{
			Account:             account,
			CoinTypeBalances:    make(map[cointype.CoinType]dcrutil.Amount),
			SKACoinTypeBalances: make(map[cointype.CoinType]*big.Int),
		}

		// Only VAR is safe to expose via the int64 CoinTypeBalances map.
		for coinType, amount := range coinBalances {
			if coinType == cointype.CoinTypeVAR {
				accountBalance.CoinTypeBalances[coinType] = amount
				accountBalance.TotalBalance = amount
			}
		}

		s = append(s, accountBalance)
	}

	return s
}

func relevantAccounts(m map[uint32]dcrutil.Amount, txs []TransactionSummary) {
	for _, tx := range txs {
		for _, d := range tx.MyInputs {
			m[d.PreviousAccount] = 0
		}
		for _, c := range tx.MyOutputs {
			m[c.Account] = 0
		}
	}
}

func (s *NotificationServer) notifyUnminedTransaction(dbtx walletdb.ReadTx, details *udb.TxDetails) {
	defer s.mu.Unlock()
	s.mu.Lock()

	// Sanity check: should not be currently coalescing a notification for
	// mined transactions at the same time that an unmined tx is notified.
	if s.currentTxNtfn != nil {
		log.Tracef("Notifying unmined tx notification while creating notification for blocks")
	}

	clients := s.transactions
	if len(clients) == 0 {
		return
	}

	unminedTxs := []TransactionSummary{makeTxSummary(dbtx, s.wallet, details)}
	unminedHashes, err := s.wallet.txStore.UnminedTxHashes(dbtx.ReadBucket(wtxmgrNamespaceKey))
	if err != nil {
		log.Errorf("Cannot fetch unmined transaction hashes: %v", err)
		return
	}
	bals := make(map[uint32]dcrutil.Amount)
	relevantAccounts(bals, unminedTxs)
	err = totalBalances(dbtx, s.wallet, bals)
	if err != nil {
		log.Errorf("Cannot determine balances for relevant accounts: %v", err)
		return
	}
	skaBals := make(map[uint32]map[cointype.CoinType]*big.Int, len(bals))
	for acct := range bals {
		skaBals[acct] = make(map[cointype.CoinType]*big.Int)
	}
	if err := totalSKABalances(dbtx, s.wallet, skaBals); err != nil {
		log.Errorf("Cannot determine SKA balances for relevant accounts: %v", err)
		return
	}
	n := &TransactionNotifications{
		UnminedTransactions:      unminedTxs,
		UnminedTransactionHashes: unminedHashes,
		NewBalances:              flattenBalanceMapWithSKA(bals, skaBals),
	}
	for _, c := range clients {
		c <- n
	}
}

func (s *NotificationServer) notifyDetachedBlock(header *wire.BlockHeader) {
	defer s.mu.Unlock()
	s.mu.Lock()

	if s.currentTxNtfn == nil {
		s.currentTxNtfn = &TransactionNotifications{}
	}
	s.currentTxNtfn.DetachedBlocks = append(s.currentTxNtfn.DetachedBlocks, header)
}

func (s *NotificationServer) notifyMinedTransaction(dbtx walletdb.ReadTx, details *udb.TxDetails, block *udb.BlockMeta) {
	defer s.mu.Unlock()
	s.mu.Lock()

	if s.currentTxNtfn == nil {
		s.currentTxNtfn = &TransactionNotifications{}
	}
	n := len(s.currentTxNtfn.AttachedBlocks)
	if n == 0 || s.currentTxNtfn.AttachedBlocks[n-1].Header.BlockHash() != block.Hash {
		return
	}
	txs := &s.currentTxNtfn.AttachedBlocks[n-1].Transactions
	*txs = append(*txs, makeTxSummary(dbtx, s.wallet, details))
}

func (s *NotificationServer) notifyAttachedBlock(block *wire.BlockHeader, blockHash *chainhash.Hash) {
	defer s.mu.Unlock()
	s.mu.Lock()

	if s.currentTxNtfn == nil {
		s.currentTxNtfn = &TransactionNotifications{}
	}

	// Add block details if it wasn't already included for previously
	// notified mined transactions.
	n := len(s.currentTxNtfn.AttachedBlocks)
	if n == 0 || s.currentTxNtfn.AttachedBlocks[n-1].Header.BlockHash() != *blockHash {
		s.currentTxNtfn.AttachedBlocks = append(s.currentTxNtfn.AttachedBlocks, Block{
			Header: block,
		})
	}
}

func (s *NotificationServer) sendAttachedBlockNotification(ctx context.Context) {
	// Avoid work if possible
	s.mu.Lock()
	if len(s.transactions) == 0 {
		s.currentTxNtfn = nil
		s.mu.Unlock()
		return
	}
	currentTxNtfn := s.currentTxNtfn
	s.currentTxNtfn = nil
	s.mu.Unlock()

	// The UnminedTransactions field is intentionally not set.  Since the
	// hashes of all detached blocks are reported, and all transactions
	// moved from a mined block back to unconfirmed are either in the
	// UnminedTransactionHashes slice or don't exist due to conflicting with
	// a mined transaction in the new best chain, there is no possiblity of
	// a new, previously unseen transaction appearing in unconfirmed.

	var (
		w             = s.wallet
		bals          = make(map[uint32]dcrutil.Amount)
		skaBals       = make(map[uint32]map[cointype.CoinType]*big.Int)
		unminedHashes []*chainhash.Hash
	)
	err := walletdb.View(ctx, w.db, func(dbtx walletdb.ReadTx) error {
		txmgrNs := dbtx.ReadBucket(wtxmgrNamespaceKey)
		var err error
		unminedHashes, err = w.txStore.UnminedTxHashes(txmgrNs)
		if err != nil {
			return err
		}
		for _, b := range currentTxNtfn.AttachedBlocks {
			relevantAccounts(bals, b.Transactions)
		}
		if err := totalBalances(dbtx, w, bals); err != nil {
			return err
		}
		for acct := range bals {
			skaBals[acct] = make(map[cointype.CoinType]*big.Int)
		}
		return totalSKABalances(dbtx, w, skaBals)
	})
	if err != nil {
		log.Errorf("Failed to construct attached blocks notification: %v", err)
		return
	}

	currentTxNtfn.UnminedTransactionHashes = unminedHashes
	currentTxNtfn.NewBalances = flattenBalanceMapWithSKA(bals, skaBals)

	s.mu.Lock()
	for _, c := range s.transactions {
		c <- currentTxNtfn
	}
	s.mu.Unlock()
}

// TransactionNotifications is a notification of changes to the wallet's
// transaction set and the current chain tip that wallet is considered to be
// synced with.  All transactions added to the blockchain are organized by the
// block they were mined in.
//
// During a chain switch, all removed block hashes are included.  Detached
// blocks are sorted in the reverse order they were mined.  Attached blocks are
// sorted in the order mined.
//
// All newly added unmined transactions are included.  Removed unmined
// transactions are not explicitly included.  Instead, the hashes of all
// transactions still unmined are included.
//
// If any transactions were involved, each affected account's new total balance
// is included.
//
// TODO: Because this includes stuff about blocks and can be fired without any
// changes to transactions, it needs a better name.
type TransactionNotifications struct {
	AttachedBlocks           []Block
	DetachedBlocks           []*wire.BlockHeader
	UnminedTransactions      []TransactionSummary
	UnminedTransactionHashes []*chainhash.Hash
	NewBalances              []AccountBalance
}

// Block contains the properties and all relevant transactions of an attached
// block.
type Block struct {
	Header       *wire.BlockHeader // Nil if referring to mempool
	Transactions []TransactionSummary
}

// TransactionSummary contains a transaction relevant to the wallet and marks
// which inputs and outputs were relevant.
//
// Fee is the VAR transaction fee in atoms. For SKA transactions Fee is zero
// and the actual fee is in SKAFee (big.Int atoms). SKAFee is nil for VAR
// transactions and for SKA transactions where the fee could not be computed
// (e.g. the wallet does not have all input amounts cached).
type TransactionSummary struct {
	Hash        *chainhash.Hash
	Transaction []byte
	MyInputs    []TransactionSummaryInput
	MyOutputs   []TransactionSummaryOutput
	Fee         dcrutil.Amount
	SKAFee      *big.Int
	Timestamp   int64
	Type        TransactionType
}

// TransactionType describes the which type of transaction is has been observed to be.
// For instance, if it has a ticket as an input and a stake base reward as an output,
// it is known to be a vote.
type TransactionType int8

const (
	// TransactionTypeRegular transaction type for all regular transactions.
	TransactionTypeRegular TransactionType = iota

	// TransactionTypeCoinbase is the transaction type for all coinbase transactions.
	TransactionTypeCoinbase

	// TransactionTypeTicketPurchase transaction type for all transactions that
	// consume regular transactions as inputs and have commitments for future votes
	// as outputs.
	TransactionTypeTicketPurchase

	// TransactionTypeVote transaction type for all transactions that consume a ticket
	// and also offer a stake base reward output.
	TransactionTypeVote

	// TransactionTypeRevocation transaction type for all transactions that consume a
	// ticket, but offer no stake base reward.
	TransactionTypeRevocation

	// TransactionTypeSSFee transaction type for stake fee distribution transactions
	// that distribute non-VAR coin fees to voters.
	// Note: This type is mapped to REGULAR in RPC responses for backward compatibility.
	TransactionTypeSSFee
)

// TxTransactionType returns the correct TransactionType given a wire transaction
func TxTransactionType(tx *wire.MsgTx) TransactionType {
	// Check for SSFee before coinbase since both have null inputs
	if stake.IsSSFee(tx) {
		return TransactionTypeSSFee
	} else if compat.IsEitherCoinBaseTx(tx) {
		return TransactionTypeCoinbase
	} else if stake.IsSStx(tx) {
		return TransactionTypeTicketPurchase
	} else if stake.IsSSGen(tx) {
		return TransactionTypeVote
	} else if isRevocation(tx) {
		return TransactionTypeRevocation
	} else {
		return TransactionTypeRegular
	}
}

// TransactionSummaryInput describes a transaction input that is relevant to the
// wallet.  The Index field marks the transaction input index of the transaction
// (not included here).  The PreviousAccount and PreviousAmount fields describe
// how much this input debits from a wallet account.
//
// PreviousAmount is VAR atoms (zero for SKA inputs). PreviousSKAAmount is the
// big.Int SKA atoms (nil for VAR inputs).
type TransactionSummaryInput struct {
	Index             uint32
	PreviousAccount   uint32
	PreviousAmount    dcrutil.Amount
	PreviousSKAAmount *big.Int
}

// TransactionSummaryOutput describes wallet properties of a transaction output
// controlled by the wallet.  The Index field marks the transaction output index
// of the transaction (not included here).
//
// Amount is VAR atoms (zero for SKA outputs). SKAAmount is the big.Int SKA
// atoms (nil for VAR outputs).
type TransactionSummaryOutput struct {
	Index        uint32
	Account      uint32
	Internal     bool
	Amount       dcrutil.Amount
	SKAAmount    *big.Int
	Address      stdaddr.Address
	OutputScript []byte
}

// AccountBalance associates balance information with an account for notification purposes.
// This structure supports both legacy VAR-only notifications (TotalBalance field) and
// new multi-coin notifications (CoinTypeBalances map) for the dual-coin system.
//
// The structure provides zero-confirmation balance data. Balances for other minimum
// confirmation counts require more expensive logic and it is not clear which minimums
// a client is interested in, so they are not included in notifications.
//
// Fields:
//   - Account: The account number this balance notification relates to
//   - TotalBalance: Legacy VAR total balance (maintained for backward compatibility)
//   - CoinTypeBalances: Map of coin type to total balance for that coin type
//     Key 0 = VAR balance, Keys 1-255 = SKA variant balances
//
// Example notification data:
//
//	AccountBalance{
//	  Account: 0,
//	  TotalBalance: 500000000, // 5 VAR (legacy field)
//	  CoinTypeBalances: map[cointype.CoinType]dcrutil.Amount{
//	    0: 500000000,   // 5 VAR
//	    1: 1000000000,  // 10 SKA1
//	    2: 250000000,   // 2.5 SKA2
//	  }
//	}
type AccountBalance struct {
	Account      uint32
	TotalBalance dcrutil.Amount // VAR total balance (for backward compatibility)

	// Multi-coin support: VAR-only breakdown for legacy subscribers.
	// SKA balances are NOT placed here — VAR atoms (1e8/coin, int64) and SKA
	// atoms (1e18/coin, big.Int) are not safely commensurable in int64. Use
	// SKACoinTypeBalances to read SKA totals.
	CoinTypeBalances map[cointype.CoinType]dcrutil.Amount

	// SKACoinTypeBalances holds per-active-SKA-coin-type totals as big.Int
	// to avoid int64 truncation at SKA scales (1 SKA = 1e18 atoms).
	SKACoinTypeBalances map[cointype.CoinType]*big.Int
}

// TransactionNotificationsClient receives TransactionNotifications from the
// NotificationServer over the channel C.
type TransactionNotificationsClient struct {
	C      <-chan *TransactionNotifications
	server *NotificationServer
}

// TransactionNotifications returns a client for receiving
// TransactionNotifiations notifications over a channel.  The channel is
// unbuffered.
//
// When finished, the Done method should be called on the client to disassociate
// it from the server.
func (s *NotificationServer) TransactionNotifications() TransactionNotificationsClient {
	c := make(chan *TransactionNotifications)
	s.mu.Lock()
	s.transactions = append(s.transactions, c)
	s.mu.Unlock()
	return TransactionNotificationsClient{
		C:      c,
		server: s,
	}
}

// Done deregisters the client from the server and drains any remaining
// messages.  It must be called exactly once when the client is finished
// receiving notifications.
func (c *TransactionNotificationsClient) Done() {
	go func() {
		// Drain notifications until the client channel is removed from
		// the server and closed.
		for range c.C {
		}
	}()
	go func() {
		s := c.server
		s.mu.Lock()
		clients := s.transactions
		for i, ch := range clients {
			if c.C == ch {
				clients[i] = clients[len(clients)-1]
				s.transactions = clients[:len(clients)-1]
				close(ch)
				break
			}
		}
		s.mu.Unlock()
	}()
}

// RemovedTransactionNotification includes the removed transaction hash.
type RemovedTransactionNotification struct {
	TxHash chainhash.Hash
}

// RemovedTransactionNotificationsClient receives RemovedTransactionNotifications over the channel C.
type RemovedTransactionNotificationsClient struct {
	C      chan *RemovedTransactionNotification
	server *NotificationServer
}

// RemovedTransactionNotifications returns a client for receiving RemovedTransactionNotifications over
// a channel.  The channel is unbuffered.  When finished, the client's Done
// method should be called to disassociate the client from the server.
func (s *NotificationServer) RemovedTransactionNotifications() RemovedTransactionNotificationsClient {
	c := make(chan *RemovedTransactionNotification)
	s.mu.Lock()
	s.removedTransactionClients = append(s.removedTransactionClients, c)
	s.mu.Unlock()
	return RemovedTransactionNotificationsClient{
		C:      c,
		server: s,
	}
}

// Done deregisters the client from the server and drains any remaining
// messages.  It must be called exactly once when the client is finished
// receiving notifications.
func (c *RemovedTransactionNotificationsClient) Done() {
	go func() {
		for range c.C {
		}
	}()
	go func() {
		s := c.server
		s.mu.Lock()
		clients := s.removedTransactionClients
		for i, ch := range clients {
			if c.C == ch {
				clients[i] = clients[len(clients)-1]
				s.removedTransactionClients = clients[:len(clients)-1]
				close(ch)
				break
			}
		}
		s.mu.Unlock()
	}()
}

func (s *NotificationServer) notifyRemovedTransaction(hash chainhash.Hash) {
	defer s.mu.Unlock()
	s.mu.Lock()
	clients := s.removedTransactionClients
	if len(clients) == 0 {
		return
	}
	n := &RemovedTransactionNotification{
		TxHash: hash,
	}
	for _, c := range clients {
		c <- n
	}
}

// AccountNotification contains properties regarding an account, such as its
// name and the number of derived and imported keys.  When any of these
// properties change, the notification is fired.
type AccountNotification struct {
	AccountNumber    uint32
	AccountName      string
	ExternalKeyCount uint32
	InternalKeyCount uint32
	ImportedKeyCount uint32
}

func (s *NotificationServer) notifyAccountProperties(props *udb.AccountProperties) {
	defer s.mu.Unlock()
	s.mu.Lock()
	clients := s.accountClients
	if len(clients) == 0 {
		return
	}
	n := &AccountNotification{
		AccountNumber:    props.AccountNumber,
		AccountName:      props.AccountName,
		ExternalKeyCount: 0,
		InternalKeyCount: 0,
		ImportedKeyCount: props.ImportedKeyCount,
	}
	// Key counts have to be fudged for BIP0044 accounts a little bit because
	// only the last used child index is saved.  Add the gap limit since these
	// addresses have also been generated and are being watched for transaction
	// activity.
	if props.AccountNumber <= udb.MaxAccountNum {
		n.ExternalKeyCount = min(hdkeychain.HardenedKeyStart,
			props.LastUsedExternalIndex+s.wallet.gapLimit)
		n.InternalKeyCount = min(hdkeychain.HardenedKeyStart,
			props.LastUsedInternalIndex+s.wallet.gapLimit)
	}
	for _, c := range clients {
		c <- n
	}
}

// AccountNotificationsClient receives AccountNotifications over the channel C.
type AccountNotificationsClient struct {
	C      chan *AccountNotification
	server *NotificationServer
}

// AccountNotifications returns a client for receiving AccountNotifications over
// a channel.  The channel is unbuffered.  When finished, the client's Done
// method should be called to disassociate the client from the server.
func (s *NotificationServer) AccountNotifications() AccountNotificationsClient {
	c := make(chan *AccountNotification)
	s.mu.Lock()
	s.accountClients = append(s.accountClients, c)
	s.mu.Unlock()
	return AccountNotificationsClient{
		C:      c,
		server: s,
	}
}

// Done deregisters the client from the server and drains any remaining
// messages.  It must be called exactly once when the client is finished
// receiving notifications.
func (c *AccountNotificationsClient) Done() {
	go func() {
		for range c.C {
		}
	}()
	go func() {
		s := c.server
		s.mu.Lock()
		clients := s.accountClients
		for i, ch := range clients {
			if c.C == ch {
				clients[i] = clients[len(clients)-1]
				s.accountClients = clients[:len(clients)-1]
				close(ch)
				break
			}
		}
		s.mu.Unlock()
	}()
}

// MainTipChangedNotification describes processed changes to the main chain tip
// block.  Attached and detached blocks are sorted by increasing heights.
//
// This is intended to be a lightweight alternative to TransactionNotifications
// when only info regarding the main chain tip block changing is needed.
type MainTipChangedNotification struct {
	AttachedBlocks []*chainhash.Hash
	DetachedBlocks []*chainhash.Hash
	NewHeight      int32
}

// MainTipChangedNotificationsClient receives MainTipChangedNotifications over
// the channel C.
type MainTipChangedNotificationsClient struct {
	C      chan *MainTipChangedNotification
	server *NotificationServer
}

// MainTipChangedNotifications returns a client for receiving
// MainTipChangedNotification over a channel.  The channel is unbuffered.  When
// finished, the client's Done method should be called to disassociate the
// client from the server.
func (s *NotificationServer) MainTipChangedNotifications() MainTipChangedNotificationsClient {
	c := make(chan *MainTipChangedNotification)
	s.mu.Lock()
	s.tipChangedClients = append(s.tipChangedClients, c)
	s.mu.Unlock()
	return MainTipChangedNotificationsClient{
		C:      c,
		server: s,
	}
}

// Done deregisters the client from the server and drains any remaining
// messages.  It must be called exactly once when the client is finished
// receiving notifications.
func (c *MainTipChangedNotificationsClient) Done() {
	go func() {
		for range c.C {
		}
	}()
	go func() {
		s := c.server
		s.mu.Lock()
		clients := s.tipChangedClients
		for i, ch := range clients {
			if c.C == ch {
				clients[i] = clients[len(clients)-1]
				s.tipChangedClients = clients[:len(clients)-1]
				close(ch)
				break
			}
		}
		s.mu.Unlock()
	}()
}

func (s *NotificationServer) notifyMainChainTipChanged(n *MainTipChangedNotification) {
	s.mu.Lock()

	for _, c := range s.tipChangedClients {
		c <- n
	}

	if len(s.confClients) > 0 {
		var wg sync.WaitGroup
		wg.Add(len(s.confClients))
		for _, c := range s.confClients {
			c := c
			go func() {
				c.process(n.NewHeight)
				wg.Done()
			}()
		}
		wg.Wait()
	}

	s.mu.Unlock()
}

// ConfirmationNotifications registers a client for confirmation notifications
// from the notification server.
func (s *NotificationServer) ConfirmationNotifications(ctx context.Context) *ConfirmationNotificationsClient {
	c := &ConfirmationNotificationsClient{
		watched: make(map[chainhash.Hash]int32),
		r:       make(chan *confNtfnResult),
		ctx:     ctx,
		s:       s,
	}

	// Register with the server
	s.mu.Lock()
	s.confClients = append(s.confClients, c)
	s.mu.Unlock()

	// Cleanup when caller signals done.
	go func() {
		<-ctx.Done()

		// Remove item from notification server's slice
		s.mu.Lock()
		slice := &s.confClients
		for i, sc := range *slice {
			if c == sc {
				(*slice)[i] = (*slice)[len(*slice)-1]
				*slice = (*slice)[:len(*slice)-1]
				break
			}
		}
		s.mu.Unlock()
	}()

	return c
}

// ConfirmationNotificationsClient provides confirmation notifications of watched
// transactions until the caller's context signals done.  Callers register for
// notifications using Watch and receive notifications by calling Recv.
type ConfirmationNotificationsClient struct {
	watched map[chainhash.Hash]int32
	mu      sync.Mutex

	r   chan *confNtfnResult
	ctx context.Context
	s   *NotificationServer
}

type confNtfnResult struct {
	result []ConfirmationNotification
	err    error
}

// ConfirmationNotification describes the number of confirmations of a single
// transaction, or -1 if the transaction is unknown or removed from the wallet.
// If the transaction is mined (Confirmations >= 1), the block hash and height
// is included.  Otherwise the block hash is nil and the block height is set to
// -1.
type ConfirmationNotification struct {
	TxHash        *chainhash.Hash
	Confirmations int32
	BlockHash     *chainhash.Hash // nil when unmined
	BlockHeight   int32           // -1 when unmined
}

// Watch adds additional transactions to watch and create confirmation results
// for.  Results are immediately created with the current number of
// confirmations and are watched until stopAfter confirmations is met or the
// transaction is unknown or removed from the wallet.
func (c *ConfirmationNotificationsClient) Watch(txHashes []*chainhash.Hash, stopAfter int32) {
	if len(txHashes) == 0 {
		return
	}
	w := c.s.wallet
	r := make([]ConfirmationNotification, 0, len(c.watched))
	err := walletdb.View(c.ctx, w.db, func(dbtx walletdb.ReadTx) error {
		txmgrNs := dbtx.ReadBucket(wtxmgrNamespaceKey)
		_, tipHeight := w.txStore.MainChainTip(dbtx)
		// cannot range here, txHashes may be modified
		for i := 0; i < len(txHashes); {
			h := txHashes[i]
			height, err := w.txStore.TxBlockHeight(dbtx, h)
			var confs int32
			switch {
			case errors.Is(err, errors.NotExist):
				confs = -1
			case err != nil:
				return err
			default:
				// Remove tx hash from watching list if tx block has been mined
				// and then invalidated by next block
				if tipHeight > height && height > 0 {
					txDetails, err := w.txStore.TxDetails(txmgrNs, h)
					if err != nil {
						return err
					}
					_, invalidated := w.txStore.BlockInMainChain(dbtx, &txDetails.Block.Hash)
					if invalidated {
						confs = -1
						break
					}
				}
				confs = confirms(height, tipHeight)
			}
			r = append(r, ConfirmationNotification{
				TxHash:        h,
				Confirmations: confs,
				BlockHeight:   -1,
			})
			if confs > 0 {
				result := &r[len(r)-1]
				height, err := w.txStore.TxBlockHeight(dbtx, result.TxHash)
				if err != nil {
					return err
				}
				blockHash, err := w.txStore.GetMainChainBlockHashForHeight(txmgrNs, height)
				if err != nil {
					return err
				}
				result.BlockHash = &blockHash
				result.BlockHeight = height
			}
			if confs >= stopAfter || confs == -1 {
				// Remove this hash from the slice so it is not added to the
				// watch map.  Do not increment i so this same index is used
				// next iteration with the new hash.
				s := &txHashes
				(*s)[i] = (*s)[len(*s)-1]
				*s = (*s)[:len(*s)-1]
			} else {
				i++
			}
		}
		return nil
	})
	if err != nil {
		r = nil
	}
	select {
	case c.r <- &confNtfnResult{r, err}:
	case <-c.ctx.Done():
	}

	c.mu.Lock()
	for _, h := range txHashes {
		c.watched[*h] = stopAfter
	}
	c.mu.Unlock()
}

// Recv waits for the next notification.  Returns context.Canceled when the
// context is canceled.
func (c *ConfirmationNotificationsClient) Recv() ([]ConfirmationNotification, error) {
	select {
	case <-c.ctx.Done():
		return nil, context.Canceled
	case r := <-c.r:
		return r.result, r.err
	}
}

func (c *ConfirmationNotificationsClient) process(tipHeight int32) {
	select {
	case <-c.ctx.Done():
		return
	default:
	}

	c.mu.Lock()
	w := c.s.wallet
	r := &confNtfnResult{
		result: make([]ConfirmationNotification, 0, len(c.watched)),
	}
	var unwatch []*chainhash.Hash
	err := walletdb.View(c.ctx, w.db, func(dbtx walletdb.ReadTx) error {
		txmgrNs := dbtx.ReadBucket(wtxmgrNamespaceKey)
		for txHash, stopAfter := range c.watched {
			txHash := txHash // copy
			height, err := w.txStore.TxBlockHeight(dbtx, &txHash)
			var confs int32
			switch {
			case errors.Is(err, errors.NotExist):
				confs = -1
			case err != nil:
				return err
			default:
				// Remove tx hash from watching list if tx block has been mined
				// and then invalidated by next block
				if tipHeight > height && height > 0 {
					txDetails, err := w.txStore.TxDetails(txmgrNs, &txHash)
					if err != nil {
						return err
					}
					_, invalidated := w.txStore.BlockInMainChain(dbtx, &txDetails.Block.Hash)
					if invalidated {
						confs = -1
						break
					}
				}
				confs = confirms(height, tipHeight)
			}
			r.result = append(r.result, ConfirmationNotification{
				TxHash:        &txHash,
				Confirmations: confs,
				BlockHeight:   -1,
			})
			if confs > 0 {
				result := &r.result[len(r.result)-1]
				height, err := w.txStore.TxBlockHeight(dbtx, result.TxHash)
				if err != nil {
					return err
				}
				blockHash, err := w.txStore.GetMainChainBlockHashForHeight(txmgrNs, height)
				if err != nil {
					return err
				}
				result.BlockHash = &blockHash
				result.BlockHeight = height
			}
			if confs >= stopAfter || confs == -1 {
				unwatch = append(unwatch, &txHash)
			}
		}
		return nil
	})
	if err != nil {
		r.result = nil
		r.err = err
	}
	for _, h := range unwatch {
		delete(c.watched, *h)
	}
	c.mu.Unlock()

	select {
	case c.r <- r:
	case <-c.ctx.Done():
	}
}
