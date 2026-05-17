// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// NOTE: This file is intended to house the RPC commands that are supported by
// a wallet server.

package types

import (
	"github.com/monetarium/monetarium-node/dcrjson"
	mondtypes "github.com/monetarium/monetarium-node/rpc/jsonrpc/types"
)

// Method describes the exact type used when registering methods with dcrjson.
type Method string

// AbandonTransactionCmd describes the command and parameters for performing the
// abandontransaction method.
type AbandonTransactionCmd struct {
	Hash string `json:"hash"`
}

// AccountAddressIndexCmd is a type handling custom marshaling and
// unmarshaling of accountaddressindex JSON wallet extension
// commands.
type AccountAddressIndexCmd struct {
	Account string `json:"account"`
	Branch  int    `json:"branch"`
}

// NewAccountAddressIndexCmd creates a new AccountAddressIndexCmd.
func NewAccountAddressIndexCmd(acct string, branch int) *AccountAddressIndexCmd {
	return &AccountAddressIndexCmd{
		Account: acct,
		Branch:  branch,
	}
}

// AccountSyncAddressIndexCmd is a type handling custom marshaling and
// unmarshaling of accountsyncaddressindex JSON wallet extension
// commands.
type AccountSyncAddressIndexCmd struct {
	Account string `json:"account"`
	Branch  int    `json:"branch"`
	Index   int    `json:"index"`
}

// NewAccountSyncAddressIndexCmd creates a new AccountSyncAddressIndexCmd.
func NewAccountSyncAddressIndexCmd(acct string, branch int,
	idx int) *AccountSyncAddressIndexCmd {
	return &AccountSyncAddressIndexCmd{
		Account: acct,
		Branch:  branch,
		Index:   idx,
	}
}

// AddMultisigAddressCmd defines the addmutisigaddress JSON-RPC command.
type AddMultisigAddressCmd struct {
	NRequired int
	Keys      []string
	Account   *string
}

// NewAddMultisigAddressCmd returns a new instance which can be used to issue a
// addmultisigaddress JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewAddMultisigAddressCmd(nRequired int, keys []string, account *string) *AddMultisigAddressCmd {
	return &AddMultisigAddressCmd{
		NRequired: nRequired,
		Keys:      keys,
		Account:   account,
	}
}

// AddTransactionCmd manually adds a single mined transaction to the wallet,
// which may be useful to add a transaction which was mined before a private
// key was imported.
// There is currently no validation that the transaction is indeed mined in
// this block.
type AddTransactionCmd struct {
	BlockHash   string `json:"blockhash"`
	Transaction string `json:"transaction"`
}

// AuditReuseCmd defines the auditreuse JSON-RPC command.
//
// This method returns an object keying reused addresses to two or more outputs
// referencing them, optionally filtering results of address reusage since a
// particular block height.
type AuditReuseCmd struct {
	Since *int32 `json:"since"`
}

// ConsolidateCmd is a type handling custom marshaling and
// unmarshaling of consolidate JSON wallet extension
// commands.
type ConsolidateCmd struct {
	Inputs   int `json:"inputs"`
	Account  *string
	Address  *string
	CoinType *uint8 `json:"cointype,omitempty"` // Optional: specify coin type (0=VAR, 1-255=SKA)
}

// NewConsolidateCmd creates a new ConsolidateCmd.
func NewConsolidateCmd(inputs int, acct *string, addr *string) *ConsolidateCmd {
	return &ConsolidateCmd{Inputs: inputs, Account: acct, Address: addr}
}

// NewConsolidateCmdWithCoinType creates a new ConsolidateCmd with coin type specified.
func NewConsolidateCmdWithCoinType(inputs int, acct *string, addr *string, coinType *uint8) *ConsolidateCmd {
	return &ConsolidateCmd{Inputs: inputs, Account: acct, Address: addr, CoinType: coinType}
}

// CreateMultisigCmd defines the createmultisig JSON-RPC command.
type CreateMultisigCmd struct {
	NRequired int
	Keys      []string
}

// NewCreateMultisigCmd returns a new instance which can be used to issue a
// createmultisig JSON-RPC command.
func NewCreateMultisigCmd(nRequired int, keys []string) *CreateMultisigCmd {
	return &CreateMultisigCmd{
		NRequired: nRequired,
		Keys:      keys,
	}
}

// CreateSignatureCmd defines the createsignature JSON-RPC command.
type CreateSignatureCmd struct {
	Address               string
	InputIndex            int
	HashType              int
	PreviousPkScript      string
	SerializedTransaction string
}

// CreateNewAccountCmd defines the createnewaccount JSON-RPC command.
type CreateNewAccountCmd struct {
	Account string
}

// NewCreateNewAccountCmd returns a new instance which can be used to issue a
// createnewaccount JSON-RPC command.
func NewCreateNewAccountCmd(account string) *CreateNewAccountCmd {
	return &CreateNewAccountCmd{
		Account: account,
	}
}

// CreateAuthorizedEmissionCmd describes the command and parameters for creating
// a cryptographically authorized SKA emission transaction with governance-defined parameters.
//
// Height and Nonce are optional operator overrides. By default, Height is the
// wallet's local synced tip — which may be stale or inconsistent with the
// node's canonical tip during a reorg; operators can pass an explicit Height
// to sign at a specific point in the emission window. Nonce defaults to 1
// (first emission) and should only be overridden when re-authorizing a coin
// type that has already been emitted once.
//
// ForceWindow and ForceNonce are independent override flags that bypass the
// corresponding safety guard. ForceWindow allows signing at a height outside
// the configured emission window; ForceNonce allows signing with a nonce
// other than 1. Each flag must be set explicitly: a single global "force"
// affordance is intentionally absent so that an operator who wants to bypass
// only one guard cannot accidentally bypass both.
type CreateAuthorizedEmissionCmd struct {
	CoinType        uint8   `json:"cointype"`            // SKA coin type (1-255)
	EmissionKeyName string  `json:"emissionkeyname"`     // Name of imported emission private key
	Passphrase      string  `json:"passphrase"`          // Wallet passphrase for key access
	Height          *int64  `json:"height,omitempty"`    // Optional: explicit block height to sign (defaults to wallet tip)
	Nonce           *uint64 `json:"nonce,omitempty"`     // Optional: emission nonce (defaults to 1)
	ForceWindow     *bool   `json:"forcewindow,omitempty" jsonrpcdefault:"false"` // Optional: bypass the out-of-window-height safety error (default false)
	ForceNonce      *bool   `json:"forcenonce,omitempty" jsonrpcdefault:"false"`  // Optional: bypass the non-default-nonce safety error (default false)
	// NOTE: Emission addresses, amounts, and windows are defined by governance
	// and retrieved from chain parameters - users cannot specify arbitrary values
}

// NewCreateAuthorizedEmissionCmd returns a new instance which can be used to issue a
// createauthorizedemission JSON-RPC command with governance-defined parameters.
func NewCreateAuthorizedEmissionCmd(coinType uint8, emissionKeyName, passphrase string) *CreateAuthorizedEmissionCmd {
	return &CreateAuthorizedEmissionCmd{
		CoinType:        coinType,
		EmissionKeyName: emissionKeyName,
		Passphrase:      passphrase,
	}
}

// GenerateEmissionKeyCmd defines the generateemissionkey JSON-RPC command for
// generating new private keys for SKA emission authorization (primary flow).
//
// WalletPassphrase is the wallet's master passphrase. It is used to unlock the
// wallet for the duration of this single call so the emission key can be stored
// in the wallet DB; the wallet is re-locked on return if it was locked
// beforehand. The ambient walletpassphrase unlock window is NOT used.
//
// Passphrase is used to encrypt the returned backup blob (only when
// ReturnEncryptedBackup=true) and is unrelated to WalletPassphrase.
type GenerateEmissionKeyCmd struct {
	KeyName               string `json:"keyname"`                         // Unique identifier for this emission key
	WalletPassphrase      string `json:"walletpassphrase"`                // Wallet master passphrase; required (per-call unlock)
	Passphrase            string `json:"passphrase"`                      // Passphrase used to encrypt the returned backup blob (if requested)
	CoinType              *uint8 `json:"cointype,omitempty"`              // Optional SKA coin type (1-255) - for user organization only
	ReturnEncryptedBackup *bool  `json:"returnencryptedbackup,omitempty"` // If true, include the encrypted private-key backup in the response; default false (canonical backup is the wallet DB)
}

// NewGenerateEmissionKeyCmd returns a new instance which can be used to issue a
// generateemissionkey JSON-RPC command.
func NewGenerateEmissionKeyCmd(keyName, walletPassphrase, passphrase string) *GenerateEmissionKeyCmd {
	return &GenerateEmissionKeyCmd{
		KeyName:          keyName,
		WalletPassphrase: walletPassphrase,
		Passphrase:       passphrase,
		CoinType:         nil,
	}
}

// NewGenerateEmissionKeyCmdWithCoinType returns a new instance with cointype parameter.
func NewGenerateEmissionKeyCmdWithCoinType(coinType uint8, keyName, walletPassphrase, passphrase string) *GenerateEmissionKeyCmd {
	return &GenerateEmissionKeyCmd{
		KeyName:          keyName,
		WalletPassphrase: walletPassphrase,
		Passphrase:       passphrase,
		CoinType:         &coinType,
	}
}

// ImportEmissionKeyCmd defines the importemissionkey JSON-RPC command for
// importing private keys used for SKA emission authorization (emergency/recovery only).
//
// WalletPassphrase is the wallet's master passphrase. It is used to unlock the
// wallet for the duration of this single call so the imported key can be stored
// in the wallet DB; the wallet is re-locked on return if it was locked
// beforehand. The ambient walletpassphrase unlock window is NOT used.
//
// Passphrase is the backup-blob passphrase used when the key was exported via
// generateemissionkey (only consumed when PrivateKey is an encrypted blob); it
// is unrelated to WalletPassphrase. Only v3 ("aes256gcm:v3:...") blobs are
// accepted; legacy v1 (no KDF) and v2 (no AAD on KDF params) blobs are
// cryptographically weak and rejected.
type ImportEmissionKeyCmd struct {
	KeyName          string `json:"keyname"`            // Unique identifier for this key
	PrivateKey       string `json:"privatekey"`         // Hex-encoded secp256k1 private key or v3 encrypted backup blob
	WalletPassphrase string `json:"walletpassphrase"`   // Wallet master passphrase; required (per-call unlock)
	Passphrase       string `json:"passphrase"`         // Backup-blob passphrase (only used when PrivateKey is an encrypted blob)
	CoinType         *uint8 `json:"cointype,omitempty"` // Optional SKA coin type (1-255) - for user organization only
}

// NewImportEmissionKeyCmd returns a new instance which can be used to issue an
// importemissionkey JSON-RPC command.
func NewImportEmissionKeyCmd(coinType uint8, keyName, privateKey, walletPassphrase, passphrase string) *ImportEmissionKeyCmd {
	return &ImportEmissionKeyCmd{
		KeyName:          keyName,
		PrivateKey:       privateKey,
		WalletPassphrase: walletPassphrase,
		Passphrase:       passphrase,
		CoinType:         &coinType,
	}
}

// NewImportEmissionKeyCmdNoCoinType returns a new instance without cointype parameter.
func NewImportEmissionKeyCmdNoCoinType(keyName, privateKey, walletPassphrase, passphrase string) *ImportEmissionKeyCmd {
	return &ImportEmissionKeyCmd{
		KeyName:          keyName,
		PrivateKey:       privateKey,
		WalletPassphrase: walletPassphrase,
		Passphrase:       passphrase,
		CoinType:         nil,
	}
}

// CreateVotingAccountCmd is a type for handling custom marshaling and
// unmarshalling of createvotingaccount JSON-RPC command.
type CreateVotingAccountCmd struct {
	Name       string
	PubKey     string
	ChildIndex *uint32 `jsonrpcdefault:"0"`
}

// NewCreateVotingAccountCmd creates a new CreateVotingAccountCmd.
func NewCreateVotingAccountCmd(name, pubKey string, childIndex *uint32) *CreateVotingAccountCmd {
	return &CreateVotingAccountCmd{name, pubKey, childIndex}
}

// DumpPrivKeyCmd defines the dumpprivkey JSON-RPC command.
type DumpPrivKeyCmd struct {
	Address string
}

// NewDumpPrivKeyCmd returns a new instance which can be used to issue a
// dumpprivkey JSON-RPC command.
func NewDumpPrivKeyCmd(address string) *DumpPrivKeyCmd {
	return &DumpPrivKeyCmd{
		Address: address,
	}
}

// FundRawTransactionOptions represents the optional inputs to fund
// a raw transaction.
type FundRawTransactionOptions struct {
	ChangeAddress *string  `json:"changeaddress"`
	FeeRate       *float64 `json:"feerate"`
	ConfTarget    *int32   `json:"conf_target"`
}

// FundRawTransactionCmd is a type handling custom marshaling and
// unmarshaling of fundrawtransaction JSON wallet extension commands.
type FundRawTransactionCmd struct {
	HexString   string
	FundAccount string
	Options     *FundRawTransactionOptions
}

// NewFundRawTransactionCmd returns a new instance which can be used to issue a
// fundrawtransaction JSON-RPC command.
func NewFundRawTransactionCmd(hexString string, fundAccount string, options *FundRawTransactionOptions) *FundRawTransactionCmd {
	return &FundRawTransactionCmd{
		HexString:   hexString,
		FundAccount: fundAccount,
		Options:     options,
	}
}

// GetAccountCmd defines the getaccount JSON-RPC command.
type GetAccountCmd struct {
	Address string
}

// NewGetAccountCmd returns a new instance which can be used to issue a
// getaccount JSON-RPC command.
func NewGetAccountCmd(address string) *GetAccountCmd {
	return &GetAccountCmd{
		Address: address,
	}
}

// GetAccountAddressCmd defines the getaccountaddress JSON-RPC command.
type GetAccountAddressCmd struct {
	Account string
}

// NewGetAccountAddressCmd returns a new instance which can be used to issue a
// getaccountaddress JSON-RPC command.
func NewGetAccountAddressCmd(account string) *GetAccountAddressCmd {
	return &GetAccountAddressCmd{
		Account: account,
	}
}

// GetAddressesByAccountCmd defines the getaddressesbyaccount JSON-RPC command.
type GetAddressesByAccountCmd struct {
	Account string
}

// NewGetAddressesByAccountCmd returns a new instance which can be used to issue
// a getaddressesbyaccount JSON-RPC command.
func NewGetAddressesByAccountCmd(account string) *GetAddressesByAccountCmd {
	return &GetAddressesByAccountCmd{
		Account: account,
	}
}

// GetBalanceCmd defines the getbalance JSON-RPC command.
type GetBalanceCmd struct {
	Account  *string `json:"account"`
	MinConf  *int    `json:"minconf" jsonrpcdefault:"1"`
	CoinType *uint8  `json:"cointype,omitempty"` // Optional: specify coin type (0=VAR, 1-255=SKA)
}

// NewGetBalanceCmd returns a new instance which can be used to issue a
// getbalance JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetBalanceCmd(account *string, minConf *int) *GetBalanceCmd {
	return &GetBalanceCmd{
		Account: account,
		MinConf: minConf,
	}
}

// NewGetBalanceCmdWithCoinType returns a new GetBalanceCmd with coin type specified.
func NewGetBalanceCmdWithCoinType(account *string, minConf *int, coinType *uint8) *GetBalanceCmd {
	return &GetBalanceCmd{
		Account:  account,
		MinConf:  minConf,
		CoinType: coinType,
	}
}

// GetMasterPubkeyCmd is a type handling custom marshaling and unmarshaling of
// getmasterpubkey JSON wallet extension commands.
type GetMasterPubkeyCmd struct {
	Account *string
}

// NewGetMasterPubkeyCmd creates a new GetMasterPubkeyCmd.
func NewGetMasterPubkeyCmd(acct *string) *GetMasterPubkeyCmd {
	return &GetMasterPubkeyCmd{Account: acct}
}

// GetMultisigOutInfoCmd is a type handling custom marshaling and
// unmarshaling of getmultisigoutinfo JSON websocket extension
// commands.
type GetMultisigOutInfoCmd struct {
	Hash  string
	Index uint32
}

// NewGetMultisigOutInfoCmd creates a new GetMultisigOutInfoCmd.
func NewGetMultisigOutInfoCmd(hash string, index uint32) *GetMultisigOutInfoCmd {
	return &GetMultisigOutInfoCmd{hash, index}
}

// GetNewAddressCmd defines the getnewaddress JSON-RPC command.
type GetNewAddressCmd struct {
	Account   *string
	GapPolicy *string
}

// NewGetNewAddressCmd returns a new instance which can be used to issue a
// getnewaddress JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetNewAddressCmd(account *string, gapPolicy *string) *GetNewAddressCmd {
	return &GetNewAddressCmd{
		Account:   account,
		GapPolicy: gapPolicy,
	}
}

// GetRawChangeAddressCmd defines the getrawchangeaddress JSON-RPC command.
type GetRawChangeAddressCmd struct {
	Account *string
}

// NewGetRawChangeAddressCmd returns a new instance which can be used to issue a
// getrawchangeaddress JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetRawChangeAddressCmd(account *string) *GetRawChangeAddressCmd {
	return &GetRawChangeAddressCmd{
		Account: account,
	}
}

// GetReceivedByAccountCmd defines the getreceivedbyaccount JSON-RPC command.
type GetReceivedByAccountCmd struct {
	Account string
	MinConf *int `jsonrpcdefault:"1"`
}

// NewGetReceivedByAccountCmd returns a new instance which can be used to issue
// a getreceivedbyaccount JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetReceivedByAccountCmd(account string, minConf *int) *GetReceivedByAccountCmd {
	return &GetReceivedByAccountCmd{
		Account: account,
		MinConf: minConf,
	}
}

// GetReceivedByAddressCmd defines the getreceivedbyaddress JSON-RPC command.
type GetReceivedByAddressCmd struct {
	Address  string
	MinConf  *int   `jsonrpcdefault:"1"`
	CoinType *uint8 `jsonrpcdefault:"0"`
}

// NewGetReceivedByAddressCmd returns a new instance which can be used to issue
// a getreceivedbyaddress JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetReceivedByAddressCmd(address string, minConf *int) *GetReceivedByAddressCmd {
	return &GetReceivedByAddressCmd{
		Address:  address,
		MinConf:  minConf,
		CoinType: nil,
	}
}

// NewGetReceivedByAddressCmdWithCoinType returns a new instance with coin type specified.
func NewGetReceivedByAddressCmdWithCoinType(address string, minConf *int, coinType *uint8) *GetReceivedByAddressCmd {
	return &GetReceivedByAddressCmd{
		Address:  address,
		MinConf:  minConf,
		CoinType: coinType,
	}
}

// GetStakeInfoCmd is a type handling custom marshaling and
// unmarshaling of getstakeinfo JSON wallet extension commands.
type GetStakeInfoCmd struct {
}

// NewGetStakeInfoCmd creates a new GetStakeInfoCmd.
func NewGetStakeInfoCmd() *GetStakeInfoCmd {
	return &GetStakeInfoCmd{}
}

// GetTicketsCmd is a type handling custom marshaling and
// unmarshaling of gettickets JSON wallet extension
// commands.
type GetTicketsCmd struct {
	IncludeImmature bool
}

// NewGetTicketsCmd returns a new instance which can be used to issue a
// gettickets JSON-RPC command.
func NewGetTicketsCmd(includeImmature bool) *GetTicketsCmd {
	return &GetTicketsCmd{includeImmature}
}

// GetTransactionCmd defines the gettransaction JSON-RPC command.
type GetTransactionCmd struct {
	Txid             string
	IncludeWatchOnly *bool `jsonrpcdefault:"false"`
}

// NewGetTransactionCmd returns a new instance which can be used to issue a
// gettransaction JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetTransactionCmd(txHash string, includeWatchOnly *bool) *GetTransactionCmd {
	return &GetTransactionCmd{
		Txid:             txHash,
		IncludeWatchOnly: includeWatchOnly,
	}
}

// GetUnconfirmedBalanceCmd defines the getunconfirmedbalance JSON-RPC command.
type GetUnconfirmedBalanceCmd struct {
	Account *string
}

// NewGetUnconfirmedBalanceCmd returns a new instance which can be used to issue
// a getunconfirmedbalance JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetUnconfirmedBalanceCmd(account *string) *GetUnconfirmedBalanceCmd {
	return &GetUnconfirmedBalanceCmd{
		Account: account,
	}
}

// GetCoinBalanceCmd defines the getcoinbalance JSON-RPC command for querying
// balance of a specific coin type (VAR or SKA).
type GetCoinBalanceCmd struct {
	CoinType uint8   `json:"cointype"`                             // Required: coin type (0=VAR, 1-255=SKA)
	Account  *string `json:"account,omitempty"`                    // Optional: account name ("*" for all accounts)
	MinConf  *int    `json:"minconf,omitempty" jsonrpcdefault:"1"` // Optional: minimum confirmations
}

// NewGetCoinBalanceCmd returns a new instance which can be used to issue a
// getcoinbalance JSON-RPC command.
func NewGetCoinBalanceCmd(coinType uint8, account *string, minConf *int) *GetCoinBalanceCmd {
	return &GetCoinBalanceCmd{
		CoinType: coinType,
		Account:  account,
		MinConf:  minConf,
	}
}

// ListCoinTypesCmd defines the listcointypes JSON-RPC command for discovering
// all coin types with non-zero balances in the wallet.
type ListCoinTypesCmd struct {
	MinConf *int `json:"minconf,omitempty" jsonrpcdefault:"1"` // Optional: minimum confirmations
}

// NewListCoinTypesCmd returns a new instance which can be used to issue a
// listcointypes JSON-RPC command.
func NewListCoinTypesCmd(minConf *int) *ListCoinTypesCmd {
	return &ListCoinTypesCmd{
		MinConf: minConf,
	}
}

// GetVoteChoicesCmd returns a new instance which can be used to issue a
// getvotechoices JSON-RPC command.
type GetVoteChoicesCmd struct {
	TicketHash *string
}

// NewGetVoteChoicesCmd returns a new instance which can be used to
// issue a JSON-RPC getvotechoices command.
func NewGetVoteChoicesCmd(tickethash *string) *GetVoteChoicesCmd {
	return &GetVoteChoicesCmd{
		TicketHash: tickethash,
	}
}

// GetWalletFeeCmd defines the getwalletfee JSON-RPC command.
type GetWalletFeeCmd struct {
	CoinType *uint8 `jsonrpcdefault:"0"`
}

// NewGetWalletFeeCmd returns a new instance which can be used to issue a
// getwalletfee JSON-RPC command.
func NewGetWalletFeeCmd() *GetWalletFeeCmd {
	return &GetWalletFeeCmd{}
}

// NewGetWalletFeeCmdWithCoinType returns a new instance which can be used to issue a
// getwalletfee JSON-RPC command with a specific coin type.
func NewGetWalletFeeCmdWithCoinType(coinType uint8) *GetWalletFeeCmd {
	return &GetWalletFeeCmd{
		CoinType: &coinType,
	}
}

// GetVoteFeeConsolidationAddressCmd defines the getvotefeeconsolidationaddress JSON-RPC command.
type GetVoteFeeConsolidationAddressCmd struct {
	Account string
}

// NewGetVoteFeeConsolidationAddressCmd returns a new instance which can be used to issue a
// getvotefeeconsolidationaddress JSON-RPC command.
func NewGetVoteFeeConsolidationAddressCmd(account string) *GetVoteFeeConsolidationAddressCmd {
	return &GetVoteFeeConsolidationAddressCmd{
		Account: account,
	}
}

// SetVoteFeeConsolidationAddressCmd defines the setvotefeeconsolidationaddress JSON-RPC command.
type SetVoteFeeConsolidationAddressCmd struct {
	Account string
	Address string
}

// NewSetVoteFeeConsolidationAddressCmd returns a new instance which can be used to issue a
// setvotefeeconsolidationaddress JSON-RPC command.
func NewSetVoteFeeConsolidationAddressCmd(account string, address string) *SetVoteFeeConsolidationAddressCmd {
	return &SetVoteFeeConsolidationAddressCmd{
		Account: account,
		Address: address,
	}
}

// ClearVoteFeeConsolidationAddressCmd defines the clearvotefeeconsolidationaddress JSON-RPC command.
type ClearVoteFeeConsolidationAddressCmd struct {
	Account string
}

// NewClearVoteFeeConsolidationAddressCmd returns a new instance which can be used to issue a
// clearvotefeeconsolidationaddress JSON-RPC command.
func NewClearVoteFeeConsolidationAddressCmd(account string) *ClearVoteFeeConsolidationAddressCmd {
	return &ClearVoteFeeConsolidationAddressCmd{
		Account: account,
	}
}

// ImportPrivKeyCmd defines the importprivkey JSON-RPC command.
type ImportPrivKeyCmd struct {
	PrivKey  string
	Label    *string
	Rescan   *bool `jsonrpcdefault:"true"`
	ScanFrom *int
}

// NewImportPrivKeyCmd returns a new instance which can be used to issue a
// importprivkey JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewImportPrivKeyCmd(privKey string, label *string, rescan *bool, scanFrom *int) *ImportPrivKeyCmd {
	return &ImportPrivKeyCmd{
		PrivKey:  privKey,
		Label:    label,
		Rescan:   rescan,
		ScanFrom: scanFrom,
	}
}

// ImportPrivKeyCmd defines the importprivkey JSON-RPC command.
type ImportPubKeyCmd struct {
	PubKey   string
	Label    *string
	Rescan   *bool `jsonrpcdefault:"true"`
	ScanFrom *int
}

// NewImportPubKeyCmd returns a new instance which can be used to issue a
// importpubkey JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewImportPubKeyCmd(pubKey string, label *string, rescan *bool, scanFrom *int) *ImportPubKeyCmd {
	return &ImportPubKeyCmd{
		PubKey:   pubKey,
		Label:    label,
		Rescan:   rescan,
		ScanFrom: scanFrom,
	}
}

// ImportScriptCmd is a type for handling custom marshaling and
// unmarshaling of importscript JSON wallet extension commands.
type ImportScriptCmd struct {
	Hex      string
	Rescan   *bool `jsonrpcdefault:"true"`
	ScanFrom *int
}

// NewImportScriptCmd creates a new GetImportScriptCmd.
func NewImportScriptCmd(hex string, rescan *bool, scanFrom *int) *ImportScriptCmd {
	return &ImportScriptCmd{hex, rescan, scanFrom}
}

// ImportXpubCmd is a type for handling custom marshaling and unmarshaling of
// importxpub JSON-RPC commands.
type ImportXpubCmd struct {
	Name string `json:"name"`
	Xpub string `json:"xpub"`
}

// ListAccountsCmd defines the listaccounts JSON-RPC command.
type ListAccountsCmd struct {
	MinConf *int `jsonrpcdefault:"1"`
}

// NewListAccountsCmd returns a new instance which can be used to issue a
// listaccounts JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewListAccountsCmd(minConf *int) *ListAccountsCmd {
	return &ListAccountsCmd{
		MinConf: minConf,
	}
}

// ListLockUnspentCmd defines the listlockunspent JSON-RPC command.
type ListLockUnspentCmd struct {
	Account *string
}

// NewListLockUnspentCmd returns a new instance which can be used to issue a
// listlockunspent JSON-RPC command.
func NewListLockUnspentCmd() *ListLockUnspentCmd {
	return &ListLockUnspentCmd{}
}

// ListReceivedByAccountCmd defines the listreceivedbyaccount JSON-RPC command.
type ListReceivedByAccountCmd struct {
	MinConf          *int  `jsonrpcdefault:"1"`
	IncludeEmpty     *bool `jsonrpcdefault:"false"`
	IncludeWatchOnly *bool `jsonrpcdefault:"false"`
}

// NewListReceivedByAccountCmd returns a new instance which can be used to issue
// a listreceivedbyaccount JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewListReceivedByAccountCmd(minConf *int, includeEmpty, includeWatchOnly *bool) *ListReceivedByAccountCmd {
	return &ListReceivedByAccountCmd{
		MinConf:          minConf,
		IncludeEmpty:     includeEmpty,
		IncludeWatchOnly: includeWatchOnly,
	}
}

// ListReceivedByAddressCmd defines the listreceivedbyaddress JSON-RPC command.
type ListReceivedByAddressCmd struct {
	MinConf          *int  `jsonrpcdefault:"1"`
	IncludeEmpty     *bool `jsonrpcdefault:"false"`
	IncludeWatchOnly *bool `jsonrpcdefault:"false"`
}

// NewListReceivedByAddressCmd returns a new instance which can be used to issue
// a listreceivedbyaddress JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewListReceivedByAddressCmd(minConf *int, includeEmpty, includeWatchOnly *bool) *ListReceivedByAddressCmd {
	return &ListReceivedByAddressCmd{
		MinConf:          minConf,
		IncludeEmpty:     includeEmpty,
		IncludeWatchOnly: includeWatchOnly,
	}
}

// ListAddressTransactionsCmd defines the listaddresstransactions JSON-RPC
// command.
type ListAddressTransactionsCmd struct {
	Addresses []string
	Account   *string
}

// NewListAddressTransactionsCmd returns a new instance which can be used to
// issue a listaddresstransactions JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewListAddressTransactionsCmd(addresses []string, account *string) *ListAddressTransactionsCmd {
	return &ListAddressTransactionsCmd{
		Addresses: addresses,
		Account:   account,
	}
}

// ListAllTransactionsCmd defines the listalltransactions JSON-RPC command.
type ListAllTransactionsCmd struct {
	Account *string
}

// NewListAllTransactionsCmd returns a new instance which can be used to issue a
// listalltransactions JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewListAllTransactionsCmd(account *string) *ListAllTransactionsCmd {
	return &ListAllTransactionsCmd{
		Account: account,
	}
}

// ListSinceBlockCmd defines the listsinceblock JSON-RPC command.
type ListSinceBlockCmd struct {
	BlockHash           *string
	TargetConfirmations *int  `jsonrpcdefault:"1"`
	IncludeWatchOnly    *bool `jsonrpcdefault:"false"`
}

// NewListSinceBlockCmd returns a new instance which can be used to issue a
// listsinceblock JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewListSinceBlockCmd(blockHash *string, targetConfirms *int, includeWatchOnly *bool) *ListSinceBlockCmd {
	return &ListSinceBlockCmd{
		BlockHash:           blockHash,
		TargetConfirmations: targetConfirms,
		IncludeWatchOnly:    includeWatchOnly,
	}
}

// ListTransactionsCmd defines the listtransactions JSON-RPC command.
type ListTransactionsCmd struct {
	Account          *string
	Count            *int  `jsonrpcdefault:"10"`
	From             *int  `jsonrpcdefault:"0"`
	IncludeWatchOnly *bool `jsonrpcdefault:"false"`
}

// NewListTransactionsCmd returns a new instance which can be used to issue a
// listtransactions JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewListTransactionsCmd(account *string, count, from *int, includeWatchOnly *bool) *ListTransactionsCmd {
	return &ListTransactionsCmd{
		Account:          account,
		Count:            count,
		From:             from,
		IncludeWatchOnly: includeWatchOnly,
	}
}

// ListUnspentCmd defines the listunspent JSON-RPC command.
type ListUnspentCmd struct {
	MinConf   *int      `json:"minconf" jsonrpcdefault:"1"`
	MaxConf   *int      `json:"maxconf" jsonrpcdefault:"9999999"`
	Addresses *[]string `json:"addresses,omitempty"`
	Account   *string   `json:"account,omitempty"`
	CoinType  *uint8    `json:"cointype,omitempty"` // Optional: filter by coin type (0=VAR, 1-255=SKA)
}

// NewListUnspentCmd returns a new instance which can be used to issue a
// listunspent JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewListUnspentCmd(minConf, maxConf *int, addresses *[]string) *ListUnspentCmd {
	return &ListUnspentCmd{
		MinConf:   minConf,
		MaxConf:   maxConf,
		Addresses: addresses,
	}
}

// NewListUnspentCmdWithCoinType returns a new ListUnspentCmd with coin type specified.
func NewListUnspentCmdWithCoinType(minConf, maxConf *int, addresses *[]string, account *string, coinType *uint8) *ListUnspentCmd {
	return &ListUnspentCmd{
		MinConf:   minConf,
		MaxConf:   maxConf,
		Addresses: addresses,
		Account:   account,
		CoinType:  coinType,
	}
}

// LockUnspentCmd defines the lockunspent JSON-RPC command.
type LockUnspentCmd struct {
	Unlock       bool
	Transactions []mondtypes.TransactionInput
}

// NewLockUnspentCmd returns a new instance which can be used to issue a
// lockunspent JSON-RPC command.
func NewLockUnspentCmd(unlock bool, transactions []mondtypes.TransactionInput) *LockUnspentCmd {
	return &LockUnspentCmd{
		Unlock:       unlock,
		Transactions: transactions,
	}
}

// PurchaseTicketCmd is a type handling custom marshaling and
// unmarshaling of purchaseticket JSON RPC commands.
type PurchaseTicketCmd struct {
	FromAccount string
	SpendLimit  float64 // In Coins
	MinConf     *int    `jsonrpcdefault:"1"`
	NumTickets  *int    `jsonrpcdefault:"1"`
	Expiry      *int
	Comment     *string
	DontSignTx  *bool
}

// NewPurchaseTicketCmd creates a new PurchaseTicketCmd.
func NewPurchaseTicketCmd(fromAccount string, spendLimit float64, minConf *int,
	numTickets *int, expiry *int, comment *string) *PurchaseTicketCmd {
	return &PurchaseTicketCmd{
		FromAccount: fromAccount,
		SpendLimit:  spendLimit,
		MinConf:     minConf,
		NumTickets:  numTickets,
		Expiry:      expiry,
		Comment:     comment,
	}
}

// CreateUnsignedTicketResult is returned from PurchaseTicketCmd
// when dontSignTx is true.
type CreateUnsignedTicketResult struct {
	UnsignedTickets []string `json:"unsignedtickets"`
	SplitTx         string   `json:"splittx"`
}

// RedeemMultiSigOutCmd is a type handling custom marshaling and
// unmarshaling of redeemmultisigout JSON RPC commands.
type RedeemMultiSigOutCmd struct {
	Hash     string
	Index    uint32
	Tree     int8
	Address  *string
	CoinType *uint8 `json:"cointype,omitempty"`
}

// NewRedeemMultiSigOutCmd creates a new RedeemMultiSigOutCmd.
func NewRedeemMultiSigOutCmd(hash string, index uint32, tree int8,
	address *string) *RedeemMultiSigOutCmd {
	return &RedeemMultiSigOutCmd{
		Hash:    hash,
		Index:   index,
		Tree:    tree,
		Address: address,
	}
}

// RedeemMultiSigOutsCmd is a type handling custom marshaling and
// unmarshaling of redeemmultisigout JSON RPC commands.
type RedeemMultiSigOutsCmd struct {
	FromScrAddress string
	ToAddress      *string
	Number         *int
	CoinType       *uint8 `json:"cointype,omitempty"`
}

// NewRedeemMultiSigOutsCmd creates a new RedeemMultiSigOutsCmd.
func NewRedeemMultiSigOutsCmd(from string, to *string,
	number *int) *RedeemMultiSigOutsCmd {
	return &RedeemMultiSigOutsCmd{
		FromScrAddress: from,
		ToAddress:      to,
		Number:         number,
	}
}

// RenameAccountCmd defines the renameaccount JSON-RPC command.
type RenameAccountCmd struct {
	OldAccount string
	NewAccount string
}

// NewRenameAccountCmd returns a new instance which can be used to issue a
// renameaccount JSON-RPC command.
func NewRenameAccountCmd(oldAccount, newAccount string) *RenameAccountCmd {
	return &RenameAccountCmd{
		OldAccount: oldAccount,
		NewAccount: newAccount,
	}
}

// RescanWalletCmd describes the rescanwallet JSON-RPC request and parameters.
type RescanWalletCmd struct {
	BeginHeight *int `jsonrpcdefault:"0"`
}

// SendFromCmd defines the sendfrom JSON-RPC command.
type SendFromCmd struct {
	FromAccount           string
	ToAddress             string
	Amount                string // Coin amount as string (preserves precision for SKA)
	MinConf               *int   `jsonrpcdefault:"1"`
	Comment               *string
	CommentTo             *string
	CoinType              *uint8 `json:"cointype,omitempty"`              // Optional: specify coin type (0=VAR, 1-255=SKA)
	SubtractFeeFromAmount *bool  `json:"subtractfeefromamount,omitempty"` // Optional: when true, fee is taken from the recipient amount instead of change (Bitcoin Core parity)
}

// NewSendFromCmd returns a new instance which can be used to issue a sendfrom
// JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewSendFromCmd(fromAccount, toAddress, amount string, minConf *int, comment, commentTo *string) *SendFromCmd {
	return &SendFromCmd{
		FromAccount: fromAccount,
		ToAddress:   toAddress,
		Amount:      amount,
		MinConf:     minConf,
		Comment:     comment,
		CommentTo:   commentTo,
	}
}

// NewSendFromCmdWithCoinType returns a new SendFromCmd with coin type specified.
func NewSendFromCmdWithCoinType(fromAccount, toAddress, amount string, minConf *int, comment, commentTo *string, coinType *uint8) *SendFromCmd {
	return &SendFromCmd{
		FromAccount: fromAccount,
		ToAddress:   toAddress,
		Amount:      amount,
		MinConf:     minConf,
		Comment:     comment,
		CommentTo:   commentTo,
		CoinType:    coinType,
	}
}

// NewSendFromCmdWithSubtractFee returns a new SendFromCmd with both coin type
// and the subtractfeefromamount flag specified. Either pointer may be nil to
// fall back to the default (VAR / false).
func NewSendFromCmdWithSubtractFee(fromAccount, toAddress, amount string, minConf *int, comment, commentTo *string, coinType *uint8, subtractFeeFromAmount *bool) *SendFromCmd {
	return &SendFromCmd{
		FromAccount:           fromAccount,
		ToAddress:             toAddress,
		Amount:                amount,
		MinConf:               minConf,
		Comment:               comment,
		CommentTo:             commentTo,
		CoinType:              coinType,
		SubtractFeeFromAmount: subtractFeeFromAmount,
	}
}

// SendManyCmd defines the sendmany JSON-RPC command.
type SendManyCmd struct {
	FromAccount string            `json:"fromaccount"`
	Amounts     map[string]string `json:"amounts" jsonrpcusage:"{\"address\":\"amount\",...}"` // Amounts as strings (preserves precision for SKA)
	MinConf     *int              `json:"minconf" jsonrpcdefault:"1"`
	Comment     *string           `json:"comment,omitempty"`
	CoinType    *uint8            `json:"cointype,omitempty"` // Optional: specify coin type (0=VAR, 1-255=SKA)
}

// NewSendManyCmd returns a new instance which can be used to issue a sendmany
// JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewSendManyCmd(fromAccount string, amounts map[string]string, minConf *int, comment *string) *SendManyCmd {
	return &SendManyCmd{
		FromAccount: fromAccount,
		Amounts:     amounts,
		MinConf:     minConf,
		Comment:     comment,
	}
}

// NewSendManyCmdWithCoinType returns a new SendManyCmd with coin type specified.
func NewSendManyCmdWithCoinType(fromAccount string, amounts map[string]string, minConf *int, comment *string, coinType *uint8) *SendManyCmd {
	return &SendManyCmd{
		FromAccount: fromAccount,
		Amounts:     amounts,
		MinConf:     minConf,
		Comment:     comment,
		CoinType:    coinType,
	}
}

// SendToAddressCmd defines the sendtoaddress JSON-RPC command.
type SendToAddressCmd struct {
	Address               string  `json:"address"`
	Amount                string  `json:"amount"` // Coin amount as string (preserves precision for SKA)
	Comment               *string `json:"comment,omitempty"`
	CommentTo             *string `json:"commentto,omitempty"`
	CoinType              *uint8  `json:"cointype,omitempty"`              // Optional: specify coin type (0=VAR, 1-255=SKA)
	SubtractFeeFromAmount *bool   `json:"subtractfeefromamount,omitempty"` // Optional: when true, fee is taken from the recipient amount instead of change (Bitcoin Core parity)
}

// NewSendToAddressCmd returns a new instance which can be used to issue a
// sendtoaddress JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewSendToAddressCmd(address, amount string, comment, commentTo *string) *SendToAddressCmd {
	return &SendToAddressCmd{
		Address:   address,
		Amount:    amount,
		Comment:   comment,
		CommentTo: commentTo,
	}
}

// NewSendToAddressCmdWithCoinType returns a new SendToAddressCmd with coin type specified.
func NewSendToAddressCmdWithCoinType(address, amount string, comment, commentTo *string, coinType *uint8) *SendToAddressCmd {
	return &SendToAddressCmd{
		Address:   address,
		Amount:    amount,
		Comment:   comment,
		CommentTo: commentTo,
		CoinType:  coinType,
	}
}

// NewSendToAddressCmdWithSubtractFee returns a new SendToAddressCmd with both
// coin type and the subtractfeefromamount flag specified. Either pointer may
// be nil to fall back to the default (VAR / false).
func NewSendToAddressCmdWithSubtractFee(address, amount string, comment, commentTo *string, coinType *uint8, subtractFeeFromAmount *bool) *SendToAddressCmd {
	return &SendToAddressCmd{
		Address:               address,
		Amount:                amount,
		Comment:               comment,
		CommentTo:             commentTo,
		CoinType:              coinType,
		SubtractFeeFromAmount: subtractFeeFromAmount,
	}
}

// SendToMultiSigCmd is a type handling custom marshaling and
// unmarshaling of sendtomultisig JSON RPC commands. The amount is carried as a
// string so SKA values (up to AtomsPerCoin = 1e18 atoms, far beyond float64's
// 15–17 significant decimal digits) are transported losslessly.
type SendToMultiSigCmd struct {
	FromAccount string
	Amount      string
	Pubkeys     []string
	NRequired   *int   `jsonrpcdefault:"1"`
	MinConf     *int   `jsonrpcdefault:"1"`
	Comment     *string
	CoinType    *uint8 `json:"cointype,omitempty"`
}

// NewSendToMultiSigCmd creates a new SendToMultiSigCmd.
func NewSendToMultiSigCmd(fromaccount string, amount string, pubkeys []string,
	nrequired *int, minConf *int, comment *string) *SendToMultiSigCmd {
	return &SendToMultiSigCmd{
		FromAccount: fromaccount,
		Amount:      amount,
		Pubkeys:     pubkeys,
		NRequired:   nrequired,
		MinConf:     minConf,
		Comment:     comment,
	}
}

// SendToTreasuryCmd defines the sendtotreasury JSON-RPC command.
type SendToTreasuryCmd struct {
	Amount float64
}

// NewSendToTreasuryCmd returns a new instance which can be used to issue a
// sendtotreasury JSON-RPC command.
func NewSendToTreasuryCmd(amount float64, comment, commentTo *string) *SendToTreasuryCmd {
	return &SendToTreasuryCmd{
		Amount: amount,
	}
}

// SendFromTreasuryCmd defines the sendfromtreasury JSON-RPC command.
type SendFromTreasuryCmd struct {
	Key     string
	Amounts map[string]float64
}

// NewSendFromTreasuryCmd returns a new instance which can be used to issue a
// sendfromtreasury JSON-RPC command.
func NewSendFromTreasuryCmd(pubkey string, amounts map[string]float64) *SendFromTreasuryCmd {
	return &SendFromTreasuryCmd{
		Key:     pubkey,
		Amounts: amounts,
	}
}

// SendToBurnCmd defines the sendtoburn JSON-RPC command for permanently
// burning SKA coins.
type SendToBurnCmd struct {
	Amount     string  `json:"amount"`            // Amount of SKA coins to burn (string for precision)
	CoinType   uint8   `json:"cointype"`          // SKA coin type (1-255)
	Passphrase string  `json:"passphrase"`        // Wallet passphrase for authorization
	Comment    *string `json:"comment,omitempty"` // Optional comment for user records
}

// NewSendToBurnCmd returns a new instance which can be used to issue a
// sendtoburn JSON-RPC command.
//
// WARNING: This operation is IRREVERSIBLE. Burned coins are permanently destroyed.
func NewSendToBurnCmd(amount string, coinType uint8, passphrase string, comment *string) *SendToBurnCmd {
	return &SendToBurnCmd{
		Amount:     amount,
		CoinType:   coinType,
		Passphrase: passphrase,
		Comment:    comment,
	}
}

// DisapprovePercentCmd defines the parameters for the disapprovepercent
// JSON-RPC command.
type DisapprovePercentCmd struct{}

// SetDisapprovePercentCmd defines the parameters for the setdisapprovepercent
// JSON-RPC command.
type SetDisapprovePercentCmd struct {
	Percent uint32
}

// TreasuryPolicyCmd defines the parameters for the treasurypolicy JSON-RPC
// command.
type TreasuryPolicyCmd struct {
	Key    *string
	Ticket *string
}

// SetTreasuryPolicyCmd defines the parameters for the settreasurypolicy
// JSON-RPC command.
type SetTreasuryPolicyCmd struct {
	Key    string
	Policy string
	Ticket *string
}

// NewSetTreasuryPolicyCmd returns a new instance which can be used to issue a settreasurypolicy
// JSON-RPC command.
func NewSetTreasuryPolicyCmd(key string, policy string, ticket *string) *SetTreasuryPolicyCmd {
	return &SetTreasuryPolicyCmd{
		Key:    key,
		Policy: policy,
		Ticket: ticket,
	}
}

// TSpendPolicyCmd defines the parameters for the tspendpolicy JSON-RPC
// command.
type TSpendPolicyCmd struct {
	Hash   *string
	Ticket *string
}

// SetTSpendPolicyCmd defines the parameters for the settspendpolicy
// JSON-RPC command.
type SetTSpendPolicyCmd struct {
	Hash   string
	Policy string
	Ticket *string
}

// NewSetTSpendPolicyCmd returns a new instance which can be used to issue a settspendpolicy
// JSON-RPC command.
func NewSetTSpendPolicyCmd(hash string, policy string, ticket *string) *SetTSpendPolicyCmd {
	return &SetTSpendPolicyCmd{
		Hash:   hash,
		Policy: policy,
		Ticket: ticket,
	}
}

// SetTxFeeCmd defines the settxfee JSON-RPC command.
type SetTxFeeCmd struct {
	// Amount in coins as string (supports big.Int precision for SKA).
	Amount   string
	CoinType *uint8 `jsonrpcdefault:"0"`
}

// NewSetTxFeeCmd returns a new instance which can be used to issue a settxfee
// JSON-RPC command.
func NewSetTxFeeCmd(amount string) *SetTxFeeCmd {
	return &SetTxFeeCmd{
		Amount: amount,
	}
}

// NewSetTxFeeCmdWithCoinType returns a new instance which can be used to issue a
// settxfee JSON-RPC command with a specific coin type.
func NewSetTxFeeCmdWithCoinType(amount string, coinType uint8) *SetTxFeeCmd {
	return &SetTxFeeCmd{
		Amount:   amount,
		CoinType: &coinType,
	}
}

// SetVoteChoiceCmd defines the parameters to the setvotechoice method.
type SetVoteChoiceCmd struct {
	AgendaID   string
	ChoiceID   string
	TicketHash *string
}

// NewSetVoteChoiceCmd returns a new instance which can be used to issue a
// setvotechoice JSON-RPC command.
func NewSetVoteChoiceCmd(agendaID, choiceID string, tickethash *string) *SetVoteChoiceCmd {
	return &SetVoteChoiceCmd{AgendaID: agendaID, ChoiceID: choiceID, TicketHash: tickethash}
}

// SignMessageCmd defines the signmessage JSON-RPC command.
type SignMessageCmd struct {
	Address string
	Message string
}

// NewSignMessageCmd returns a new instance which can be used to issue a
// signmessage JSON-RPC command.
func NewSignMessageCmd(address, message string) *SignMessageCmd {
	return &SignMessageCmd{
		Address: address,
		Message: message,
	}
}

// RawTxInput models the data needed for raw transaction input that is used in
// the SignRawTransactionCmd struct.  Contains Decred additions.
//
// SKAValueIn is a decimal-coin string (e.g. "1.234567890123456789") asserting
// the SKA atom value of the prevout being spent. It is required for SKA inputs
// when the wallet does not own the prevout — without it the wire-format tx
// either carries SKAValueIn through deserialize/sign/serialize (the wallet's
// own unsigned-tx hex case), or arrives with SKAValueIn=nil from a third-party
// tool, in which case the wallet must populate it from a wallet UTXO lookup or
// this caller-supplied value before signing. VAR inputs ignore this field.
type RawTxInput struct {
	Txid         string  `json:"txid"`
	Vout         uint32  `json:"vout"`
	Tree         int8    `json:"tree"`
	ScriptPubKey string  `json:"scriptPubKey"`
	RedeemScript string  `json:"redeemScript"`
	SKAValueIn   *string `json:"skaValueIn,omitempty"`
}

// SignRawTransactionCmd defines the signrawtransaction JSON-RPC command.
type SignRawTransactionCmd struct {
	RawTx    string
	Inputs   *[]RawTxInput
	PrivKeys *[]string
	Flags    *string `jsonrpcdefault:"\"ALL\""`
}

// NewSignRawTransactionCmd returns a new instance which can be used to issue a
// signrawtransaction JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewSignRawTransactionCmd(hexEncodedTx string, inputs *[]RawTxInput, privKeys *[]string, flags *string) *SignRawTransactionCmd {
	return &SignRawTransactionCmd{
		RawTx:    hexEncodedTx,
		Inputs:   inputs,
		PrivKeys: privKeys,
		Flags:    flags,
	}
}

// SignRawTransactionsCmd defines the signrawtransactions JSON-RPC command.
type SignRawTransactionsCmd struct {
	RawTxs []string
	Send   *bool `jsonrpcdefault:"true"`
}

// NewSignRawTransactionsCmd returns a new instance which can be used to issue a
// signrawtransactions JSON-RPC command.
func NewSignRawTransactionsCmd(hexEncodedTxs []string,
	send *bool) *SignRawTransactionsCmd {
	return &SignRawTransactionsCmd{
		RawTxs: hexEncodedTxs,
		Send:   send,
	}
}

// SweepAccountCmd defines the sweep account JSON-RPC command.
//
// FeePerKb is a decimal coin string (e.g. "0.0001" for VAR, "8" for SKA) so it
// can carry SKA's 1e18-atom precision; float64 cannot, and earlier numeric
// schemas were rejected by the wallet's precision guard for SKA coin types.
type SweepAccountCmd struct {
	SourceAccount         string
	DestinationAddress    string
	RequiredConfirmations *uint32
	FeePerKb              *string
	CoinType              *uint8 `json:"cointype,omitempty"` // Optional: 0=VAR (default), 1-255=SKA
}

// NewSweepAccountCmd returns a new instance which can be used to issue a JSON-RPC SweepAccountCmd command.
func NewSweepAccountCmd(sourceAccount string, destinationAddress string, requiredConfs *uint32, feePerKb *string) *SweepAccountCmd {
	return &SweepAccountCmd{
		SourceAccount:         sourceAccount,
		DestinationAddress:    destinationAddress,
		RequiredConfirmations: requiredConfs,
		FeePerKb:              feePerKb,
	}
}

// SyncStatusCmd defines the syncstatus JSON-RPC command.
type SyncStatusCmd struct{}

// WalletInfoCmd defines the walletinfo JSON-RPC command.
type WalletInfoCmd struct {
}

// NewWalletInfoCmd returns a new instance which can be used to issue a
// walletinfo JSON-RPC command.
func NewWalletInfoCmd() *WalletInfoCmd {
	return &WalletInfoCmd{}
}

// WalletIsLockedCmd defines the walletislocked JSON-RPC command.
type WalletIsLockedCmd struct{}

// NewWalletIsLockedCmd returns a new instance which can be used to issue a
// walletislocked JSON-RPC command.
func NewWalletIsLockedCmd() *WalletIsLockedCmd {
	return &WalletIsLockedCmd{}
}

// WalletLockCmd defines the walletlock JSON-RPC command.
type WalletLockCmd struct{}

// NewWalletLockCmd returns a new instance which can be used to issue a
// walletlock JSON-RPC command.
func NewWalletLockCmd() *WalletLockCmd {
	return &WalletLockCmd{}
}

// WalletPassphraseCmd defines the walletpassphrase JSON-RPC command.
type WalletPassphraseCmd struct {
	Passphrase string
	Timeout    int64
}

// NewWalletPassphraseCmd returns a new instance which can be used to issue a
// walletpassphrase JSON-RPC command.
func NewWalletPassphraseCmd(passphrase string, timeout int64) *WalletPassphraseCmd {
	return &WalletPassphraseCmd{
		Passphrase: passphrase,
		Timeout:    timeout,
	}
}

// WalletPassphraseChangeCmd defines the walletpassphrase JSON-RPC command.
type WalletPassphraseChangeCmd struct {
	OldPassphrase string
	NewPassphrase string
}

// NewWalletPassphraseChangeCmd returns a new instance which can be used to
// issue a walletpassphrasechange JSON-RPC command.
func NewWalletPassphraseChangeCmd(oldPassphrase, newPassphrase string) *WalletPassphraseChangeCmd {
	return &WalletPassphraseChangeCmd{
		OldPassphrase: oldPassphrase,
		NewPassphrase: newPassphrase,
	}
}

// MixAccountCmd defines the mixaccount JSON-RPC command.
type MixAccountCmd struct{}

// MixOutputCmd defines the mixoutput JSON-RPC command.
type MixOutputCmd struct {
	Outpoint string `json:"outpoint"`
}

// DiscoverUsageCmd defines the discoverusage JSON-RPC command.
type DiscoverUsageCmd struct {
	StartBlock       *string `json:"startblock"`
	DiscoverAccounts *bool   `json:"discoveraccounts"`
	GapLimit         *uint32 `json:"gaplimit"`
}

// ImportCfiltersV2Cmd defines the importcfiltersv2 JSON-RPC command.
type ImportCFiltersV2Cmd struct {
	StartHeight int32    `json:"startheight"`
	Filters     []string `json:"filters"`
}

// TicketInfoCmd defines the ticketinfo JSON-RPC command.
type TicketInfoCmd struct {
	StartHeight *int32 `json:"startheight" jsonrpcdefault:"0"`
}

// WalletPubPassphraseChangeCmd defines the walletpubpassphrasechange JSON-RPC command.
type WalletPubPassphraseChangeCmd struct {
	OldPassphrase string
	NewPassphrase string
}

// SetAccountPassphraseCmd defines the setaccountpassphrase JSON-RPC command
// arguments.
type SetAccountPassphraseCmd struct {
	Account    string
	Passphrase string
}

// UnlockAccountCmd defines the unlockaccount JSON-RPC command arguments.
type UnlockAccountCmd struct {
	Account    string
	Passphrase string
}

// LockAccountCmd defines the lockaccount JSON-RPC command arguments.
type LockAccountCmd struct {
	Account string
}

// AccountUnlockedCmd defines the accountunlocked JSON-RPC command arguments.
type AccountUnlockedCmd struct {
	Account string
}

// ProcessUnmanagedTicket defines the processunmanagedticket JSON-RPC command arguments.
type ProcessUnmanagedTicketCmd struct {
	TicketHash string
}

// GetCoinjoinsByAcctCmd defines the getcoinjoinsbyaccount JSON-RPC command arguments.
type GetCoinjoinsByAcctCmd struct{}

// SpendOutputsCmd defines the spendoutputs JSON-RPC command arguments.
type SpendOutputsCmd struct {
	Account           string
	PreviousOutpoints []string
	Outputs           []AddressAmountPair
	CoinType          *uint8 `json:"cointype,omitempty"` // Optional: specify coin type (0=VAR, 1-255=SKA)
}

// AddressAmountPair represents a JSON object defining an address and an
// amount. Amount is a decimal coin string for both VAR and SKA — preserving
// SKA big.Int precision and avoiding float64 round-trip loss for VAR.
type AddressAmountPair struct {
	Address string `json:"address"`
	Amount  string `json:"amount"`
}

func init() {
	type registeredMethod struct {
		method string
		cmd    any
	}

	// Wallet-specific methods
	register := []registeredMethod{
		{"abandontransaction", (*AbandonTransactionCmd)(nil)},
		{"accountaddressindex", (*AccountAddressIndexCmd)(nil)},
		{"accountsyncaddressindex", (*AccountSyncAddressIndexCmd)(nil)},
		{"accountunlocked", (*AccountUnlockedCmd)(nil)},
		{"addmultisigaddress", (*AddMultisigAddressCmd)(nil)},
		{"addtransaction", (*AddTransactionCmd)(nil)},
		{"auditreuse", (*AuditReuseCmd)(nil)},
		{"consolidate", (*ConsolidateCmd)(nil)},
		{"createmultisig", (*CreateMultisigCmd)(nil)},
		{"createnewaccount", (*CreateNewAccountCmd)(nil)},
		{"createauthorizedemission", (*CreateAuthorizedEmissionCmd)(nil)},
		{"createsignature", (*CreateSignatureCmd)(nil)},
		{"generateemissionkey", (*GenerateEmissionKeyCmd)(nil)},
		{"importemissionkey", (*ImportEmissionKeyCmd)(nil)},
		{"createvotingaccount", (*CreateVotingAccountCmd)(nil)},
		{"disapprovepercent", (*DisapprovePercentCmd)(nil)},
		{"discoverusage", (*DiscoverUsageCmd)(nil)},
		{"dumpprivkey", (*DumpPrivKeyCmd)(nil)},
		{"fundrawtransaction", (*FundRawTransactionCmd)(nil)},
		{"getaccount", (*GetAccountCmd)(nil)},
		{"getaccountaddress", (*GetAccountAddressCmd)(nil)},
		{"getaddressesbyaccount", (*GetAddressesByAccountCmd)(nil)},
		{"getbalance", (*GetBalanceCmd)(nil)},
		{"getcoinbalance", (*GetCoinBalanceCmd)(nil)},
		{"getcoinjoinsbyacct", (*GetCoinjoinsByAcctCmd)(nil)},
		{"getmasterpubkey", (*GetMasterPubkeyCmd)(nil)},
		{"getmultisigoutinfo", (*GetMultisigOutInfoCmd)(nil)},
		{"getnewaddress", (*GetNewAddressCmd)(nil)},
		{"getrawchangeaddress", (*GetRawChangeAddressCmd)(nil)},
		{"getreceivedbyaccount", (*GetReceivedByAccountCmd)(nil)},
		{"getreceivedbyaddress", (*GetReceivedByAddressCmd)(nil)},
		{"getstakeinfo", (*GetStakeInfoCmd)(nil)},
		{"gettickets", (*GetTicketsCmd)(nil)},
		{"gettransaction", (*GetTransactionCmd)(nil)},
		{"getunconfirmedbalance", (*GetUnconfirmedBalanceCmd)(nil)},
		{"getvotechoices", (*GetVoteChoicesCmd)(nil)},
		{"getvotefeeconsolidationaddress", (*GetVoteFeeConsolidationAddressCmd)(nil)},
		{"getwalletfee", (*GetWalletFeeCmd)(nil)},
		{"clearvotefeeconsolidationaddress", (*ClearVoteFeeConsolidationAddressCmd)(nil)},
		{"importcfiltersv2", (*ImportCFiltersV2Cmd)(nil)},
		{"importprivkey", (*ImportPrivKeyCmd)(nil)},
		{"importpubkey", (*ImportPubKeyCmd)(nil)},
		{"importscript", (*ImportScriptCmd)(nil)},
		{"importxpub", (*ImportXpubCmd)(nil)},
		{"listaccounts", (*ListAccountsCmd)(nil)},
		{"listaddresstransactions", (*ListAddressTransactionsCmd)(nil)},
		{"listcointypes", (*ListCoinTypesCmd)(nil)},
		{"listalltransactions", (*ListAllTransactionsCmd)(nil)},
		{"listlockunspent", (*ListLockUnspentCmd)(nil)},
		{"listreceivedbyaccount", (*ListReceivedByAccountCmd)(nil)},
		{"listreceivedbyaddress", (*ListReceivedByAddressCmd)(nil)},
		{"listsinceblock", (*ListSinceBlockCmd)(nil)},
		{"listtransactions", (*ListTransactionsCmd)(nil)},
		{"listunspent", (*ListUnspentCmd)(nil)},
		{"lockaccount", (*LockAccountCmd)(nil)},
		{"lockunspent", (*LockUnspentCmd)(nil)},
		{"mixaccount", (*MixAccountCmd)(nil)},
		{"mixoutput", (*MixOutputCmd)(nil)},
		{"purchaseticket", (*PurchaseTicketCmd)(nil)},
		{"processunmanagedticket", (*ProcessUnmanagedTicketCmd)(nil)},
		{"redeemmultisigout", (*RedeemMultiSigOutCmd)(nil)},
		{"redeemmultisigouts", (*RedeemMultiSigOutsCmd)(nil)},
		{"renameaccount", (*RenameAccountCmd)(nil)},
		{"rescanwallet", (*RescanWalletCmd)(nil)},
		{"sendfrom", (*SendFromCmd)(nil)},
		{"sendfromtreasury", (*SendFromTreasuryCmd)(nil)},
		{"sendmany", (*SendManyCmd)(nil)},
		{"sendtoaddress", (*SendToAddressCmd)(nil)},
		{"sendtomultisig", (*SendToMultiSigCmd)(nil)},
		{"sendtotreasury", (*SendToTreasuryCmd)(nil)},
		{"sendtoburn", (*SendToBurnCmd)(nil)},
		{"setaccountpassphrase", (*SetAccountPassphraseCmd)(nil)},
		{"setdisapprovepercent", (*SetDisapprovePercentCmd)(nil)},
		{"settreasurypolicy", (*SetTreasuryPolicyCmd)(nil)},
		{"settspendpolicy", (*SetTSpendPolicyCmd)(nil)},
		{"settxfee", (*SetTxFeeCmd)(nil)},
		{"setvotechoice", (*SetVoteChoiceCmd)(nil)},
		{"setvotefeeconsolidationaddress", (*SetVoteFeeConsolidationAddressCmd)(nil)},
		{"signmessage", (*SignMessageCmd)(nil)},
		{"signrawtransaction", (*SignRawTransactionCmd)(nil)},
		{"signrawtransactions", (*SignRawTransactionsCmd)(nil)},
		{"spendoutputs", (*SpendOutputsCmd)(nil)},
		{"sweepaccount", (*SweepAccountCmd)(nil)},
		{"syncstatus", (*SyncStatusCmd)(nil)},
		{"ticketinfo", (*TicketInfoCmd)(nil)},
		{"treasurypolicy", (*TreasuryPolicyCmd)(nil)},
		{"tspendpolicy", (*TSpendPolicyCmd)(nil)},
		{"unlockaccount", (*UnlockAccountCmd)(nil)},
		{"walletinfo", (*WalletInfoCmd)(nil)},
		{"walletislocked", (*WalletIsLockedCmd)(nil)},
		{"walletlock", (*WalletLockCmd)(nil)},
		{"walletpassphrase", (*WalletPassphraseCmd)(nil)},
		{"walletpassphrasechange", (*WalletPassphraseChangeCmd)(nil)},
		{"walletpubpassphrasechange", (*WalletPubPassphraseChangeCmd)(nil)},
	}
	for i := range register {
		dcrjson.MustRegister(Method(register[i].method), register[i].cmd, 0)
	}

	// mond methods also implemented by monwallet
	register = []registeredMethod{
		{"createrawtransaction", (*CreateRawTransactionCmd)(nil)},
		{"debuglevel", (*DebugLevelCmd)(nil)},
		{"getbestblock", (*GetBestBlockCmd)(nil)},
		{"getbestblockhash", (*GetBestBlockHashCmd)(nil)},
		{"getblockcount", (*GetBlockCountCmd)(nil)},
		{"getblockhash", (*GetBlockHashCmd)(nil)},
		{"getblockheader", (*GetBlockHeaderCmd)(nil)},
		{"getblock", (*GetBlockCmd)(nil)},
		{"getcfilterv2", (*GetCFilterV2Cmd)(nil)},
		{"getcurrentnet", (*GetCurrentNetCmd)(nil)},
		{"getinfo", (*GetInfoCmd)(nil)},
		{"getpeerinfo", (*GetPeerInfoCmd)(nil)},
		{"gettxout", (*GetTxOutCmd)(nil)},
		{"help", (*HelpCmd)(nil)},
		{"sendrawtransaction", (*SendRawTransactionCmd)(nil)},
		{"validateaddress", (*ValidateAddressCmd)(nil)},
		{"verifymessage", (*VerifyMessageCmd)(nil)},
		{"version", (*VersionCmd)(nil)},
	}
	for i := range register {
		dcrjson.MustRegister(Method(register[i].method), register[i].cmd, 0)
	}

	// Websocket-specific methods implemented by monwallet
	register = []registeredMethod{
		{"authenticate", (*AuthenticateCmd)(nil)},
	}
	for i := range register {
		dcrjson.MustRegister(Method(register[i].method), register[i].cmd,
			dcrjson.UFWebsocketOnly)
	}
}

// CreateRawTransactionCmd extends the mond createrawtransaction command with
// an optional CoinType. Amounts is a single map[string]string for both VAR
// and SKA — decimal coin strings preserve SKA big.Int precision and avoid
// float64 round-trip loss for VAR amounts above ~9e7 VAR.
//
// InputAmounts is an optional parallel slice of decimal-coin strings indexed
// by input position; when an entry is non-empty the parser uses it instead
// of the float64 Amount field on TransactionInput, preserving full precision.
// An entry of "" falls back to the float64 Amount for backwards compatibility.
type CreateRawTransactionCmd struct {
	Inputs       []mondtypes.TransactionInput
	Amounts      map[string]string `jsonrpcusage:"{\"address\":\"amount\",...}"` // Decimal coin strings (VAR or SKA per CoinType)
	LockTime     *int64
	Expiry       *int64
	CoinType     *uint8    `json:"cointype,omitempty"`
	InputAmounts *[]string `json:"inputamounts,omitempty"` // Optional decimal-coin override per input (index-aligned with Inputs)
}

// newtype definitions of mond commands we implement.
type (
	AuthenticateCmd mondtypes.AuthenticateCmd
	DebugLevelCmd   mondtypes.DebugLevelCmd
	GetBestBlockCmd         mondtypes.GetBestBlockCmd
	GetBestBlockHashCmd     mondtypes.GetBestBlockHashCmd
	GetBlockCountCmd        mondtypes.GetBlockCountCmd
	GetBlockHashCmd         mondtypes.GetBlockHashCmd
	GetBlockHeaderCmd       mondtypes.GetBlockHeaderCmd
	GetBlockCmd             mondtypes.GetBlockCmd
	GetCFilterV2Cmd         mondtypes.GetCFilterV2Cmd
	GetCurrentNetCmd        mondtypes.GetCurrentNetCmd
	GetInfoCmd              mondtypes.GetInfoCmd
	GetPeerInfoCmd          mondtypes.GetPeerInfoCmd
	GetTxOutCmd             mondtypes.GetTxOutCmd
	HelpCmd                 mondtypes.HelpCmd
	SendRawTransactionCmd   mondtypes.SendRawTransactionCmd
	ValidateAddressCmd      mondtypes.ValidateAddressCmd
	VerifyMessageCmd        mondtypes.VerifyMessageCmd
	VersionCmd              mondtypes.VersionCmd
)
