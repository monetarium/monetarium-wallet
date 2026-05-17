// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

// FundRawTransactionResult models the data from the fundrawtransaction command.
//
// Fee is a decimal coin string for both VAR and SKA, formatted against the
// transaction's coin type's atomsPerCoin. The string form preserves SKA
// big.Int precision and unifies the wire shape across coin types.
type FundRawTransactionResult struct {
	Hex string `json:"hex"`
	Fee string `json:"fee"`
}

// GetAccountBalanceResult models the account data from the getbalance command.
//
// Wire contract:
//   - All amount fields are decimal coin strings for both VAR and SKA, formatted
//     against the coin type's atomsPerCoin. Examples: "1.23456789" for VAR,
//     "899999999999999.000000000400001" for SKA. The string form preserves full
//     SKA big.Int precision (atomsPerCoin is typically 1e18 for SKA).
//
// This is a breaking change from the prior interface{} (float64-or-string)
// shape: VAR clients that parsed amounts as JSON numbers must now parse them as
// JSON strings.
type GetAccountBalanceResult struct {
	AccountName             string `json:"accountname"`
	ImmatureCoinbaseRewards string `json:"immaturecoinbaserewards"`
	ImmatureStakeGeneration string `json:"immaturestakegeneration"`
	LockedByTickets         string `json:"lockedbytickets"`
	Spendable               string `json:"spendable"`
	Total                   string `json:"total"`
	Unconfirmed             string `json:"unconfirmed"`
	VotingAuthority         string `json:"votingauthority"`
}

// GetBalanceResult models the data from the getbalance command.
//
// Wire contract: total fields are decimal coin strings (see GetAccountBalanceResult
// for the full shape). Breaking change from the prior interface{} typing.
type GetBalanceResult struct {
	Balances                     []GetAccountBalanceResult `json:"balances"`
	BlockHash                    string                    `json:"blockhash"`
	TotalImmatureCoinbaseRewards string                    `json:"totalimmaturecoinbaserewards,omitempty"`
	TotalImmatureStakeGeneration string                    `json:"totalimmaturestakegeneration,omitempty"`
	TotalLockedByTickets         string                    `json:"totallockedbytickets,omitempty"`
	TotalSpendable               string                    `json:"totalspendable,omitempty"`
	CumulativeTotal              string                    `json:"cumulativetotal,omitempty"`
	TotalUnconfirmed             string                    `json:"totalunconfirmed,omitempty"`
	TotalVotingAuthority         string                    `json:"totalvotingauthority,omitempty"`
}

// GetMultisigOutInfoResult models the data returned from the getmultisigoutinfo
// command.
type GetMultisigOutInfoResult struct {
	Address      string   `json:"address"`
	RedeemScript string   `json:"redeemscript"`
	M            uint8    `json:"m"`
	N            uint8    `json:"n"`
	Pubkeys      []string `json:"pubkeys"`
	TxHash       string   `json:"txhash"`
	BlockHeight  uint32   `json:"blockheight"`
	BlockHash    string   `json:"blockhash"`
	Spent        bool     `json:"spent"`
	SpentBy      string   `json:"spentby"`
	SpentByIndex uint32   `json:"spentbyindex"`
	// Amount is the output value as a decimal string (VAR or SKA, see CoinType).
	Amount   string `json:"amount"`
	CoinType uint8  `json:"cointype"`
}

// CreateMultiSigResult models the data returned from the createmultisig
// command.
type CreateMultiSigResult struct {
	Address      string `json:"address"`
	RedeemScript string `json:"redeemScript"`
}

// CreateSignatureResult models the data returned from the createsignature
// command.
type CreateSignatureResult struct {
	Signature string `json:"signature"`
	PublicKey string `json:"publickey"`
}

// CreateAuthorizedEmissionResult models the data returned from the createauthorizedemission
// command.
//
// Warning is populated only when the local one-shot guard could not be
// persisted to the wallet DB after a successful sign on the forceNonce path
// (where a prior record exists). Operators MUST treat its presence as a
// signal not to retry the call without forcenonce=true. When the local
// guard fails on a fresh (coin, nonce) the call is refused outright rather
// than returning a signed tx with a warning — see the function comment in
// internal/rpc/jsonrpc/methods.go for the rationale.
type CreateAuthorizedEmissionResult struct {
	Transaction     string `json:"transaction"`       // Hex-encoded signed transaction
	TransactionHash string `json:"transactionhash"`   // Transaction hash
	Nonce           uint64 `json:"nonce"`             // Nonce used in this emission
	TotalAmount     string `json:"totalamount"`       // Total amount being emitted (string for big.Int precision)
	CoinType        uint8  `json:"cointype"`          // Coin type being emitted
	Warning         string `json:"warning,omitempty"` // Operator-visible warning (e.g. local one-shot guard persistence failed)
}

// GenerateEmissionKeyResult models the data returned from the generateemissionkey command.
//
// EncryptedPrivateKey is only populated when the request sets
// ReturnEncryptedBackup=true. The canonical backup is the wallet DB itself
// (stored under CKTEmission, scrypt-protected by the wallet's master passphrase).
type GenerateEmissionKeyResult struct {
	Success             bool   `json:"success"`                       // Whether the key was successfully generated
	CoinType            uint8  `json:"cointype,omitempty"`            // Optional coin type for user reference
	KeyName             string `json:"keyname"`                       // Name identifier for the generated key
	PublicKey           string `json:"publickey"`                     // Hex-encoded public key for governance proposals
	EncryptedPrivateKey string `json:"encryptedprivatekey,omitempty"` // Encrypted private-key backup (scrypt+AES-GCM); only present when ReturnEncryptedBackup=true
}

// ImportEmissionKeyResult models the data returned from the importemissionkey command.
type ImportEmissionKeyResult struct {
	Success   bool   `json:"success"`   // Whether the key was successfully imported
	CoinType  uint8  `json:"cointype"`  // Coin type the key was imported for
	KeyName   string `json:"keyname"`   // Name identifier for the imported key
	PublicKey string `json:"publickey"` // Hex-encoded public key for verification
}

// GetWalletFeeResult models the data returned from the getwalletfee command.
type GetWalletFeeResult struct {
	Fee    string `json:"fee"`    // Fee amount in coins per KB (string for big.Int precision)
	Source string `json:"source"` // Source of the fee: "manual", "rpc", or "static"
}

// GetPeerInfoResult models the data returned from the getpeerinfo command.
type GetPeerInfoResult struct {
	ID             int32  `json:"id"`
	Addr           string `json:"addr"`
	AddrLocal      string `json:"addrlocal"`
	Services       string `json:"services"`
	Version        uint32 `json:"version"`
	SubVer         string `json:"subver"`
	StartingHeight int64  `json:"startingheight"`
	BanScore       int32  `json:"banscore"`
}

// GetStakeInfoResult models the data returned from the getstakeinfo
// command.
type GetStakeInfoResult struct {
	BlockHeight  int64   `json:"blockheight"`
	Difficulty   float64 `json:"difficulty"`
	TotalSubsidy float64 `json:"totalsubsidy"`

	OwnMempoolTix  uint32 `json:"ownmempooltix"`
	Immature       uint32 `json:"immature"`
	Unspent        uint32 `json:"unspent"`
	Voted          uint32 `json:"voted"`
	Revoked        uint32 `json:"revoked"`
	UnspentExpired uint32 `json:"unspentexpired"`

	// Not available to SPV wallets
	PoolSize         uint32  `json:"poolsize,omitempty"`
	AllMempoolTix    uint32  `json:"allmempooltix,omitempty"`
	Live             uint32  `json:"live,omitempty"`
	ProportionLive   float64 `json:"proportionlive,omitempty"`
	Missed           uint32  `json:"missed,omitempty"`
	ProportionMissed float64 `json:"proportionmissed,omitempty"`
	Expired          uint32  `json:"expired,omitempty"`
}

// GetTicketsResult models the data returned from the gettickets
// command.
type GetTicketsResult struct {
	Hashes []string `json:"hashes"`
}

// GetTransactionDetailsResult models the details data from the gettransaction command.
//
// This models the "short" version of the ListTransactionsResult type, which
// excludes fields common to the transaction.  These common fields are instead
// part of the GetTransactionResult.
//
// Amount and Fee are decimal coin strings for both VAR and SKA. Breaking change
// from the prior interface{} typing — see GetAccountBalanceResult.
type GetTransactionDetailsResult struct {
	Account           string `json:"account"`
	Address           string `json:"address,omitempty"`
	Amount            string `json:"amount"`
	Category          string `json:"category"`
	InvolvesWatchOnly bool   `json:"involveswatchonly,omitempty"`
	Fee               string `json:"fee,omitempty"`
	Vout              uint32 `json:"vout"`
}

// GetTransactionResult models the data from the gettransaction command.
//
// Amount and Fee are decimal coin strings for both VAR and SKA. Breaking change
// from the prior interface{} typing — see GetAccountBalanceResult.
type GetTransactionResult struct {
	Amount          string                        `json:"amount"`
	Fee             string                        `json:"fee,omitempty"`
	Confirmations   int64                         `json:"confirmations"`
	BlockHash       string                        `json:"blockhash"`
	BlockIndex      int64                         `json:"blockindex"`
	BlockTime       int64                         `json:"blocktime"`
	TxID            string                        `json:"txid"`
	WalletConflicts []string                      `json:"walletconflicts"`
	Time            int64                         `json:"time"`
	TimeReceived    int64                         `json:"timereceived"`
	Details         []GetTransactionDetailsResult `json:"details"`
	Hex             string                        `json:"hex"`
	Type            string                        `json:"type"`
	TicketStatus    string                        `json:"ticketstatus,omitempty"`
}

// GetCFilterV2Result models the data returned from the getcfilterv2 command.
type GetCFilterV2Result struct {
	BlockHash string `json:"blockhash"`
	Filter    string `json:"filter"`
	Key       string `json:"key"`
}

// VoteChoice models the data for a vote choice in the getvotechoices result.
type VoteChoice struct {
	AgendaID          string `json:"agendaid"`
	AgendaDescription string `json:"agendadescription,omitempty"`
	ChoiceID          string `json:"choiceid"`
	ChoiceDescription string `json:"choicedescription,omitempty"`
}

// GetVoteChoicesResult models the data returned by the getvotechoices command.
type GetVoteChoicesResult struct {
	Version uint32       `json:"version"`
	Choices []VoteChoice `json:"choices"`
}

// GetVoteFeeConsolidationAddressResult models the data returned from the
// getvotefeeconsolidationaddress command.
type GetVoteFeeConsolidationAddressResult struct {
	Account   string `json:"account"`
	Address   string `json:"address"`
	IsDefault bool   `json:"isdefault"` // True if using auto-default (first external address)
}

// SyncStatusResult models the data returned by the syncstatus command.
type SyncStatusResult struct {
	Synced               bool    `json:"synced"`
	InitialBlockDownload bool    `json:"initialblockdownload"`
	HeadersFetchProgress float32 `json:"headersfetchprogress"`
}

// InfoResult models the data returned by the wallet server getinfo
// command.
type InfoResult struct {
	Version         int32   `json:"version"`
	ProtocolVersion int32   `json:"protocolversion"`
	WalletVersion   int32   `json:"walletversion"`
	Balance         float64 `json:"balance"`
	Blocks          int32   `json:"blocks"`
	TimeOffset      int64   `json:"timeoffset"`
	Connections     int32   `json:"connections"`
	Proxy           string  `json:"proxy"`
	Difficulty      float64 `json:"difficulty"`
	TestNet         bool    `json:"testnet"`
	KeypoolOldest   int64   `json:"keypoololdest"`
	KeypoolSize     int32   `json:"keypoolsize"`
	UnlockedUntil   int64   `json:"unlocked_until"`
	PaytxFee        string  `json:"paytxfee"`
	RelayFee        string  `json:"relayfee"`
	CoinType        uint32  `json:"cointype,omitempty"`
	Errors          string  `json:"errors"`
}

// InfoWalletResult aliases InfoResult.
type InfoWalletResult = InfoResult

// ListTransactionsTxType defines the type used in the listtransactions JSON-RPC
// result for the TxType command field.
type ListTransactionsTxType string

const (
	// LTTTRegular indicates a regular transaction.
	LTTTRegular ListTransactionsTxType = "regular"

	// LTTTTicket indicates a ticket.
	LTTTTicket ListTransactionsTxType = "ticket"

	// LTTTVote indicates a vote.
	LTTTVote ListTransactionsTxType = "vote"

	// LTTTRevocation indicates a revocation.
	LTTTRevocation ListTransactionsTxType = "revocation"
)

// ListTransactionsResult models the data from the listtransactions command.
//
// Amount and Fee are decimal coin strings for both VAR and SKA. Breaking change
// from the prior interface{} typing — see GetAccountBalanceResult.
type ListTransactionsResult struct {
	Account           string                  `json:"account"`
	Address           string                  `json:"address,omitempty"`
	Amount            string                  `json:"amount"`
	BlockHash         string                  `json:"blockhash,omitempty"`
	BlockIndex        *int64                  `json:"blockindex,omitempty"`
	BlockTime         int64                   `json:"blocktime,omitempty"`
	Category          string                  `json:"category"`
	Confirmations     int64                   `json:"confirmations"`
	Fee               string                  `json:"fee,omitempty"`
	Generated         bool                    `json:"generated,omitempty"`
	InvolvesWatchOnly bool                    `json:"involveswatchonly,omitempty"`
	Time              int64                   `json:"time"`
	TimeReceived      int64                   `json:"timereceived"`
	TxID              string                  `json:"txid"`
	TxType            *ListTransactionsTxType `json:"txtype,omitempty"`
	Vout              uint32                  `json:"vout"`
	WalletConflicts   []string                `json:"walletconflicts"`
	Comment           string                  `json:"comment,omitempty"`
	OtherAccount      string                  `json:"otheraccount,omitempty"`
}

// ListReceivedByAccountResult models the data from the listreceivedbyaccount
// command.
type ListReceivedByAccountResult struct {
	Account string `json:"account"`
	// Amount is the total received as a decimal string (VAR or SKA atoms,
	// per the configured network).
	Amount        string `json:"amount"`
	Confirmations uint64 `json:"confirmations"`
}

// ListReceivedByAddressResult models the data from the listreceivedbyaddress
// command.
type ListReceivedByAddressResult struct {
	Account string `json:"account"`
	Address string `json:"address"`
	// Amount is the total received as a decimal string (VAR or SKA atoms,
	// per the configured network).
	Amount            string   `json:"amount"`
	Confirmations     uint64   `json:"confirmations"`
	TxIDs             []string `json:"txids,omitempty"`
	InvolvesWatchonly bool     `json:"involvesWatchonly,omitempty"`
}

// ListSinceBlockResult models the data from the listsinceblock command.
type ListSinceBlockResult struct {
	Transactions []ListTransactionsResult `json:"transactions"`
	LastBlock    string                   `json:"lastblock"`
}

// ListUnspentResult models a successful response from the listunspent request.
//
// Wire contract:
//   - CoinType is 0 for VAR, 1-255 for SKA. Always present.
//   - Amount is a decimal coin string for both VAR and SKA, formatted against
//     the coin type's atomsPerCoin. Examples: "0.001" for VAR, "1.5" for SKA.
//     The string form preserves full SKA big.Int precision (atomsPerCoin is
//     typically 1e18 for SKA).
//
// This is a breaking change from the prior "Amount float64 + SkaAmount string"
// shape: VAR clients that parsed Amount as a JSON number must now parse it as
// a JSON string.
type ListUnspentResult struct {
	TxID          string `json:"txid"`
	Vout          uint32 `json:"vout"`
	Tree          int8   `json:"tree"`
	TxType        int    `json:"txtype"`
	Address       string `json:"address"`
	Account       string `json:"account"`
	ScriptPubKey  string `json:"scriptPubKey"`
	RedeemScript  string `json:"redeemScript,omitempty"`
	Amount        string `json:"amount"` // Decimal coins as string (VAR or SKA, see CoinType)
	Confirmations int64  `json:"confirmations"`
	Spendable     bool   `json:"spendable"`
	CoinType      uint8  `json:"cointype"` // Dual-coin support: coin type (0=VAR, 1-255=SKA)
}

// RedeemMultiSigOutResult models the data returned from the redeemmultisigout
// command.
type RedeemMultiSigOutResult struct {
	Hex      string                    `json:"hex"`
	Complete bool                      `json:"complete"`
	Errors   []SignRawTransactionError `json:"errors,omitempty"`
}

// RedeemMultiSigOutsResult models the data returned from the redeemmultisigouts
// command.
//
// Truncated is set to true when the wallet capped the number of multisig
// outputs processed in a single call (server-side cap, currently 256). Callers
// that see Truncated=true should spend the returned redemption transactions
// and re-call to drain the remaining outputs.
//
// Skipped lists multisig credits the wallet considered unspent locally but the
// node reports as missing/spent — typically orphans of a failed-publish multisig
// tx that the wallet authored but the network never accepted. Surfaced rather
// than silently dropped so operators can correlate stale wallet state with
// on-chain truth.
type RedeemMultiSigOutsResult struct {
	Results   []RedeemMultiSigOutResult `json:"results"`
	Truncated bool                      `json:"truncated"`
	Skipped   []SkippedMultisigOutpoint `json:"skipped,omitempty"`
}

// SkippedMultisigOutpoint describes a multisig credit that the
// redeemmultisigouts handler refused to author a redemption for because the
// node's UTXO set disagrees with the wallet's bucketMultisigUsp record.
type SkippedMultisigOutpoint struct {
	Hash     string `json:"hash"`
	Vout     uint32 `json:"vout"`
	Tree     int8   `json:"tree"`
	CoinType uint8  `json:"cointype"`
	Reason   string `json:"reason"`
}

// SendToMultiSigResult models the data returned from the sendtomultisig
// command.
type SendToMultiSigResult struct {
	TxHash       string `json:"txhash"`
	Address      string `json:"address"`
	RedeemScript string `json:"redeemscript"`
}

// SignRawTransactionError models the data that contains script verification
// errors from the signrawtransaction request.
type SignRawTransactionError struct {
	TxID      string `json:"txid"`
	Vout      uint32 `json:"vout"`
	ScriptSig string `json:"scriptSig"`
	Sequence  uint32 `json:"sequence"`
	Error     string `json:"error"`
}

// SignRawTransactionResult models the data from the signrawtransaction
// command.
type SignRawTransactionResult struct {
	Hex      string                    `json:"hex"`
	Complete bool                      `json:"complete"`
	Errors   []SignRawTransactionError `json:"errors,omitempty"`
}

// SignedTransaction is a signed transaction resulting from a signrawtransactions
// command.
type SignedTransaction struct {
	SigningResult SignRawTransactionResult `json:"signingresult"`
	Sent          bool                     `json:"sent"`
	TxHash        *string                  `json:"txhash,omitempty"`
}

// SignRawTransactionsResult models the data returned from the signrawtransactions
// command.
type SignRawTransactionsResult struct {
	Results []SignedTransaction `json:"results"`
}

// SweepAccountResult models the data returned from the sweepaccount
// command.
//
// Amount fields are full-precision base-10 decimal coin strings computed
// directly from *big.Int atoms.  This avoids the float64 round-trip
// rounding that SKA amounts (1e18 atoms/coin) cannot survive and that
// drifts large VAR balances past ~9e7 VAR.
type SweepAccountResult struct {
	UnsignedTransaction       string `json:"unsignedtransaction"`
	TotalPreviousOutputAmount string `json:"totalpreviousoutputamount"`
	TotalOutputAmount         string `json:"totaloutputamount"`
	EstimatedSignedSize       uint32 `json:"estimatedsignedsize"`
}

// TicketInfoResult models the data returned from the ticketinfo command.
type TicketInfoResult struct {
	Hash string `json:"hash"`
	// Cost is the ticket purchase price as a decimal string (VAR atoms).
	Cost          string       `json:"cost"`
	VotingAddress string       `json:"votingaddress"`
	Status        string       `json:"status"`
	BlockHash     string       `json:"blockhash,omitempty"`
	BlockHeight   int32        `json:"blockheight"`
	Vote          string       `json:"vote,omitempty"`
	Revocation    string       `json:"revocation,omitempty"`
	Choices       []VoteChoice `json:"choices,omitempty"`
	VSPHost       string       `json:"vsphost,omitempty"`
}

// TreasuryPolicyResult models objects returned by the treasurypolicy command.
type TreasuryPolicyResult struct {
	Key    string `json:"key"`
	Policy string `json:"policy"`
	Ticket string `json:"ticket,omitempty"`
}

// TSpendPolicyResult models objects returned by the tspendpolicy command.
type TSpendPolicyResult struct {
	Hash   string `json:"hash"`
	Policy string `json:"policy"`
	Ticket string `json:"ticket,omitempty"`
}

// ValidateAddressResult models the data returned by the wallet server
// validateaddress command.
type ValidateAddressResult struct {
	IsValid      bool     `json:"isvalid"`
	Address      string   `json:"address,omitempty"`
	IsMine       bool     `json:"ismine,omitempty"`
	IsWatchOnly  bool     `json:"iswatchonly,omitempty"`
	IsScript     bool     `json:"isscript,omitempty"`
	PubKeyAddr   string   `json:"pubkeyaddr,omitempty"`
	PubKey       string   `json:"pubkey,omitempty"`
	IsCompressed bool     `json:"iscompressed,omitempty"`
	Account      string   `json:"account,omitempty"`
	Addresses    []string `json:"addresses,omitempty"`
	Hex          string   `json:"hex,omitempty"`
	Script       string   `json:"script,omitempty"`
	SigsRequired int32    `json:"sigsrequired,omitempty"`
	AccountN     *uint32  `json:"accountn,omitempty"`
	Branch       *uint32  `json:"branch,omitempty"`
	Index        *uint32  `json:"index,omitempty"`
}

// ValidateAddressWalletResult aliases ValidateAddressResult.
type ValidateAddressWalletResult = ValidateAddressResult

// WalletInfoResult models the data returned from the walletinfo command.
type WalletInfoResult struct {
	DaemonConnected  bool   `json:"daemonconnected"`
	SPV              bool   `json:"spv"`
	Unlocked         bool   `json:"unlocked"`
	CoinType         uint32 `json:"cointype,omitempty"`
	TxFee            string `json:"txfee"`
	VoteBits         uint16 `json:"votebits"`
	VoteBitsExtended string `json:"votebitsextended"`
	VoteVersion      uint32 `json:"voteversion"`
	Voting           bool   `json:"voting"`
	VSP              string `json:"vsp"`
	ManualTickets    bool   `json:"manualtickets"`
	BirthHash        string `json:"birthhash"`
	BirthHeight      uint32 `json:"birthheight"`
}

// AccountUnlockedResult models the data returned by the accountunlocked
// command. When Encrypted is false, Unlocked should be nil.
type AccountUnlockedResult struct {
	Encrypted bool  `json:"encrypted"`
	Unlocked  *bool `json:"unlocked,omitempty"`
}

// GetCoinBalanceResult models the data returned from the getcoinbalance command.
// This provides detailed balance information for a specific coin type.
//
// All amount fields are decimal coin strings for both VAR and SKA. Breaking
// change from the prior interface{} typing — see GetAccountBalanceResult.
type GetCoinBalanceResult struct {
	CoinType                     uint8                         `json:"cointype"`                     // The coin type (0=VAR, 1-255=SKA)
	BlockHash                    string                        `json:"blockhash"`                    // Current block hash
	TotalImmatureCoinbaseRewards string                        `json:"totalimmaturecoinbaserewards"` // Total immature coinbase rewards
	TotalImmatureStakeGeneration string                        `json:"totalimmaturestakegeneration"` // Total immature stake generation
	TotalLockedByTickets         string                        `json:"totallockedbytickets"`         // Total locked by tickets
	TotalSpendable               string                        `json:"totalspendable"`               // Total spendable balance
	TotalUnconfirmed             string                        `json:"totalunconfirmed"`             // Total unconfirmed balance
	TotalVotingAuthority         string                        `json:"totalvotingauthority"`         // Total voting authority
	CumulativeTotal              string                        `json:"cumulativetotal"`              // Cumulative total balance
	Balances                     []GetCoinAccountBalanceResult `json:"balances"`                     // Per-account breakdown
}

// GetCoinAccountBalanceResult models per-account balance data within GetCoinBalanceResult.
//
// All amount fields are decimal coin strings for both VAR and SKA. Breaking
// change from the prior interface{} typing — see GetAccountBalanceResult.
type GetCoinAccountBalanceResult struct {
	AccountName             string `json:"accountname"`             // Account name
	CoinType                uint8  `json:"cointype"`                // The coin type (0=VAR, 1-255=SKA)
	ImmatureCoinbaseRewards string `json:"immaturecoinbaserewards"` // Immature coinbase rewards
	ImmatureStakeGeneration string `json:"immaturestakegeneration"` // Immature stake generation
	LockedByTickets         string `json:"lockedbytickets"`         // Locked by tickets
	Spendable               string `json:"spendable"`               // Spendable balance
	Total                   string `json:"total"`                   // Total balance
	Unconfirmed             string `json:"unconfirmed"`             // Unconfirmed balance
	VotingAuthority         string `json:"votingauthority"`         // Voting authority
}

// ListCoinTypesResult models the data returned from the listcointypes command.
// This lists all coin types that have non-zero balances in the wallet.
type ListCoinTypesResult struct {
	CoinTypes []CoinTypeInfo `json:"cointypes"` // List of active coin types
}

// CoinTypeInfo provides information about a specific coin type.
//
// Balance is a decimal coin string for both VAR and SKA. Breaking change from
// the prior interface{} typing — see GetAccountBalanceResult.
type CoinTypeInfo struct {
	CoinType uint8  `json:"cointype"` // The coin type number (0=VAR, 1-255=SKA)
	Name     string `json:"name"`     // Human-readable name (e.g., "VAR", "SKA1", "SKA2")
	Balance  string `json:"balance"`  // Total spendable balance for this coin type
}
