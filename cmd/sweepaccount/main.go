// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"

	"github.com/monetarium/monetarium-wallet/rpc/jsonrpc/types"
	"github.com/monetarium/monetarium-wallet/wallet/txauthor"
	"github.com/monetarium/monetarium-wallet/wallet/txrules"
	"github.com/monetarium/monetarium-wallet/wallet/txsizes"
	"github.com/monetarium/monetarium-node/chaincfg/chainhash"
	"github.com/monetarium/monetarium-node/chaincfg"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/dcrutil"
	"github.com/monetarium/monetarium-node/txscript/stdaddr"
	"github.com/monetarium/monetarium-node/wire"
	"github.com/jessevdk/go-flags"
	"github.com/jrick/wsrpc/v2"
	"golang.org/x/term"
)

var (
	activeNet           = chaincfg.MainNetParams()
	walletDataDirectory = dcrutil.AppDataDir("monetarium-wallet", false)
	newlineBytes        = []byte{'\n'}
	// feeRateAtomsParsed is the cached result of decoding opts.FeeRate via
	// the active coin type's scale. Populated once in parseFlags so sweep()
	// does not re-parse and risk drifting from the validated value.
	feeRateAtomsParsed *big.Int
)

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Stderr.Write(newlineBytes)
	os.Exit(1)
}

func errContext(err error, context string) error {
	return fmt.Errorf("%s: %v", context, err)
}

// Flags.
var opts = struct {
	TestNet               bool    `long:"testnet" description:"Use the test monetarium network"`
	SimNet                bool    `long:"simnet" description:"Use the simulation monetarium network"`
	RPCConnect            string  `short:"c" long:"connect" description:"Hostname[:port] of wallet RPC server"`
	RPCUsername           string  `short:"u" long:"rpcuser" description:"Wallet RPC username"`
	RPCPassword           string  `short:"P" long:"rpcpass" description:"Wallet RPC password"`
	RPCCertificateFile    string  `long:"cafile" description:"Wallet RPC TLS certificate"`
	FeeRate               string  `long:"feerate" description:"Transaction fee per kilobyte as a decimal coin amount of the selected coin type (e.g. '0.0001' for VAR)"`
	CoinType              uint8   `long:"cointype" description:"Coin type to sweep (0 = VAR, 1-255 = SKA). Outputs of other coin types are skipped with a warning."`
	SourceAccount         string  `long:"sourceacct" description:"Account to sweep outputs from"`
	SourceAddress         string  `long:"sourceaddr" description:"Address to sweep outputs from"`
	DestinationAccount    string  `long:"destacct" description:"Account to send sweeped outputs to"`
	DestinationAddress    string  `long:"destaddr" description:"Address to send sweeped outputs to"`
	RequiredConfirmations int64   `long:"minconf" description:"Required confirmations to include an output"`
	DryRun                bool    `long:"dryrun" description:"Do not actually send any transactions but output what would have happened"`
}{
	TestNet:               false,
	SimNet:                false,
	RPCConnect:            "localhost",
	RPCUsername:           "",
	RPCPassword:           "",
	RPCCertificateFile:    filepath.Join(walletDataDirectory, "rpc.cert"),
	FeeRate:               "0.0001",
	CoinType:              0,
	SourceAccount:         "",
	SourceAddress:         "",
	DestinationAccount:    "",
	DestinationAddress:    "",
	RequiredConfirmations: 2,
	DryRun:                false,
}

// normalizeAddress returns the normalized form of the address, adding a default
// port if necessary.  An error is returned if the address, even without a port,
// is not valid.
func normalizeAddress(addr string, defaultPort string) (hostport string, err error) {
	// If the first SplitHostPort errors because of a missing port and not
	// for an invalid host, add the port.  If the second SplitHostPort
	// fails, then a port is not missing and the original error should be
	// returned.
	host, port, origErr := net.SplitHostPort(addr)
	if origErr == nil {
		return net.JoinHostPort(host, port), nil
	}
	addr = net.JoinHostPort(addr, defaultPort)
	_, _, err = net.SplitHostPort(addr)
	if err != nil {
		return "", origErr
	}
	return addr, nil
}

func walletPort(net *chaincfg.Params) string {
	switch net.Net {
	case wire.MainNet:
		return "9510"
	case wire.TestNet3:
		return "19510"
	case wire.SimNet:
		return "19957"
	default:
		return ""
	}
}

// parseFlags reads the global opts struct from the process command line and
// validates / normalises every field. Called from main(), NOT from init(),
// so go test binaries don't trigger CLI flag parsing on -test.* flags. The
// previous design lived in init() and required an isTestBinary() heuristic
// to opt out under tests; calling this from main() removes that heuristic.
func parseFlags() {
	// Unset localhost defaults if certificate file can not be found.
	_, err := os.Stat(opts.RPCCertificateFile)
	if err != nil {
		opts.RPCConnect = ""
		opts.RPCCertificateFile = ""
	}

	_, err = flags.Parse(&opts)
	if err != nil {
		os.Exit(1)
	}

	if opts.TestNet && opts.SimNet {
		fatalf("Multiple monetarium networks may not be used simultaneously")
	}
	if opts.TestNet {
		activeNet = chaincfg.TestNet3Params()
	} else if opts.SimNet {
		activeNet = chaincfg.SimNetParams()
	}

	if opts.RPCConnect == "" {
		fatalf("RPC hostname[:port] is required")
	}
	rpcConnect, err := normalizeAddress(opts.RPCConnect, walletPort(activeNet))
	if err != nil {
		fatalf("Invalid RPC network address `%v`: %v", opts.RPCConnect, err)
	}
	opts.RPCConnect = rpcConnect

	if opts.RPCUsername == "" {
		fatalf("RPC username is required")
	}

	_, err = os.Stat(opts.RPCCertificateFile)
	if err != nil {
		fatalf("RPC certificate file `%s` not found", opts.RPCCertificateFile)
	}

	// Bound the fee rate against the active coin type's atoms-per-coin scale,
	// so the same "exceptionally low/high" semantics (1e-6 .. 1 coin/kB) apply
	// uniformly to VAR and SKA without losing precision.
	atomsPerCoin := atomsPerCoinFor(activeNet, cointype.CoinType(opts.CoinType))
	if atomsPerCoin == nil {
		fatalf("No SKA config for cointype %d on the active network", opts.CoinType)
	}
	feeAtoms, err := cointype.DecimalStringToAtoms(opts.FeeRate, atomsPerCoin)
	if err != nil {
		fatalf("Invalid --feerate `%s`: %v", opts.FeeRate, err)
	}
	// Upper bound on the fee rate. For VAR keep the historical 1-coin/kB cap.
	// For SKA, the network's mandatory min relay fee can exceed 1 SKA/kB
	// (currently 4 SKA/kB), so the legacy bound is unusable; use
	// MaxFeeMultiplier × MinRelayTxFee from chain params instead.
	upperBound := new(big.Int).Set(atomsPerCoin)
	if cointype.CoinType(opts.CoinType).IsSKA() {
		if cfg := activeNet.GetSKACoinConfig(cointype.CoinType(opts.CoinType)); cfg != nil && cfg.MinRelayTxFee != nil && cfg.MinRelayTxFee.Sign() > 0 {
			mult := big.NewInt(cfg.EffectiveMaxFeeMultiplier())
			upperBound = new(big.Int).Mul(cfg.MinRelayTxFee, mult)
		}
	}
	if feeAtoms.Cmp(upperBound) > 0 {
		fatalf("Fee rate `%s/kB` is exceptionally high (max %s atoms/kB)", opts.FeeRate, upperBound)
	}
	// Lower bound on the fee rate. The historical floor is atomsPerCoin/1e6
	// (1e-6 coin/kB), but the network's mandatory MinRelayTxFee can be much
	// higher — for SKA1 mainnet it is 4e18 atoms/kB, well above the
	// historical floor — and authoring below it produces a tx the node will
	// reject. Take the max of the two so the CLI rejects unrelayable rates
	// before the wallet authors them.
	lowBound := new(big.Int).Quo(atomsPerCoin, big.NewInt(1_000_000))
	ct := cointype.CoinType(opts.CoinType)
	if ct.IsSKA() {
		if cfg := activeNet.GetSKACoinConfig(ct); cfg != nil && cfg.MinRelayTxFee != nil && cfg.MinRelayTxFee.Sign() > 0 {
			if cfg.MinRelayTxFee.Cmp(lowBound) > 0 {
				lowBound = new(big.Int).Set(cfg.MinRelayTxFee)
			}
		}
	} else {
		netMin := big.NewInt(int64(txrules.DefaultRelayFeePerKb))
		if netMin.Cmp(lowBound) > 0 {
			lowBound = netMin
		}
	}
	if feeAtoms.Cmp(lowBound) < 0 {
		fatalf("Fee rate `%s/kB` is exceptionally low (network minimum is %s atoms/kB)", opts.FeeRate, lowBound.String())
	}
	feeRateAtomsParsed = feeAtoms
	if opts.SourceAccount == "" && opts.SourceAddress == "" {
		fatalf("A source is required")
	}
	if opts.SourceAccount != "" && opts.SourceAccount == opts.DestinationAccount {
		fatalf("Source and destination accounts should not be equal")
	}
	if opts.DestinationAccount == "" && opts.DestinationAddress == "" {
		fatalf("A destination is required")
	}
	if opts.DestinationAccount != "" && opts.DestinationAddress != "" {
		fatalf("Destination must be either an account or an address")
	}
	if opts.RequiredConfirmations < 0 {
		fatalf("Required confirmations must be non-negative")
	}
}

// noInputValue describes an error returned by the input source when no inputs
// were selected because each previous output value was zero.  Callers of
// txauthor.NewUnsignedTransaction need not report these errors to the user.
type noInputValue struct {
}

func (noInputValue) Error() string { return "no input value" }

// makeInputSource creates an InputSource that creates inputs for every unspent
// output with non-zero output values matching the requested coinType. UTXOs
// of other coin types are skipped with a warning. For SKA the inputs carry
// big.Int totals via InputDetail.SKAAmount; for VAR the existing int64 path
// is preserved.
func makeInputSource(outputs []types.ListUnspentResult, coinType cointype.CoinType, atomsPerCoin *big.Int) (txauthor.InputSource, *big.Int) {
	isSKA := coinType.IsSKA()
	var (
		totalInputValue   dcrutil.Amount     // VAR
		totalInputSKA     = cointype.Zero()  // SKA
		inputs            = make([]*wire.TxIn, 0, len(outputs))
		redeemScriptSizes = make([]int, 0, len(outputs))
		sourceErr         error
		mismatchedSkipped int
	)
	for _, output := range outputs {
		if output.CoinType != uint8(coinType) {
			mismatchedSkipped++
			continue
		}

		previousOutPoint, err := parseOutPoint(&output)
		if err != nil {
			sourceErr = fmt.Errorf(
				"invalid data in listunspent result: %v", err)
			break
		}

		atoms, err := cointype.DecimalStringToAtoms(output.Amount, atomsPerCoin)
		if err != nil {
			sourceErr = fmt.Errorf(
				"invalid amount `%v` in listunspent result: %v",
				output.Amount, err)
			break
		}
		if atoms.Sign() == 0 {
			continue
		}
		if isSKA {
			totalInputSKA = totalInputSKA.Add(cointype.NewSKAAmount(atoms))
			txIn := wire.NewTxIn(&previousOutPoint, 0, nil)
			txIn.SKAValueIn = new(big.Int).Set(atoms)
			inputs = append(inputs, txIn)
			redeemScriptSizes = append(redeemScriptSizes, txsizes.RedeemP2PKHSigScriptSize)
		} else {
			if !atoms.IsInt64() {
				sourceErr = fmt.Errorf(
					"impossible VAR amount `%v` in listunspent result (overflows int64)",
					output.Amount)
				break
			}
			outputAmount := dcrutil.Amount(atoms.Int64())
			if !saneOutputValue(outputAmount) {
				sourceErr = fmt.Errorf(
					"impossible output amount `%v` in listunspent result",
					outputAmount)
				break
			}
			totalInputValue += outputAmount
			txIn := wire.NewTxIn(&previousOutPoint, int64(outputAmount), nil)
			inputs = append(inputs, txIn)
			redeemScriptSizes = append(redeemScriptSizes, txsizes.RedeemP2PKHSigScriptSize)
		}
	}

	if sourceErr == nil {
		if isSKA {
			if totalInputSKA.IsZero() {
				sourceErr = noInputValue{}
			}
		} else if totalInputValue == 0 {
			sourceErr = noInputValue{}
		}
	}

	if mismatchedSkipped > 0 {
		fmt.Fprintf(os.Stderr,
			"sweepaccount: warning: skipped %d UTXO(s) not matching --cointype %d.\n",
			mismatchedSkipped, coinType)
		// All UTXOs in the input set were filtered out — the operator
		// almost certainly typed the wrong --cointype. Emit a tagged
		// WARN that scripts can grep for; the downstream "no input
		// value" surfaces as a generic error otherwise.
		if len(inputs) == 0 {
			fmt.Fprintf(os.Stderr,
				"sweepaccount: WARN: --cointype %d filter excluded all %d UTXO(s) "+
					"in this account; nothing to sweep.\n",
				coinType, mismatchedSkipped)
		}
	}

	totalAtomsForReport := new(big.Int)
	if isSKA {
		totalAtomsForReport = totalInputSKA.BigInt()
	} else {
		totalAtomsForReport.SetInt64(int64(totalInputValue))
	}

	src := func(dcrutil.Amount, cointype.SKAAmount) (*txauthor.InputDetail, error) {
		inputDetail := txauthor.InputDetail{
			Amount:            totalInputValue,
			SKAAmount:         totalInputSKA,
			Inputs:            inputs,
			Scripts:           nil,
			RedeemScriptSizes: redeemScriptSizes,
		}
		return &inputDetail, sourceErr
	}
	return src, totalAtomsForReport
}

// destinationScriptSourceToAccount is a ChangeSource which is used to receive
// all correlated previous input value.
type destinationScriptSourceToAccount struct {
	accountName string
	rpcClient   *wsrpc.Client
}

// Source creates a non-change address.
func (src *destinationScriptSourceToAccount) Script() ([]byte, uint16, error) {
	var destinationAddressStr string
	err := src.rpcClient.Call(context.Background(), "getnewaddress", &destinationAddressStr,
		src.accountName)
	if err != nil {
		return nil, 0, err
	}

	destinationAddress, err := stdaddr.DecodeAddress(destinationAddressStr, activeNet)
	if err != nil {
		return nil, 0, err
	}

	scriptVer, script := destinationAddress.PaymentScript()

	return script, scriptVer, nil
}

func (src *destinationScriptSourceToAccount) ScriptSize() int {
	return 25 // P2PKHPkScriptSize
}

// destinationScriptSourceToAddress s a ChangeSource which is used to
// receive all correlated previous input value.
type destinationScriptSourceToAddress struct {
	address string
}

// Source creates a non-change address.
func (src *destinationScriptSourceToAddress) Script() ([]byte, uint16, error) {
	destinationAddress, err := stdaddr.DecodeAddress(src.address, activeNet)
	if err != nil {
		return nil, 0, err
	}
	scriptVer, script := destinationAddress.PaymentScript()
	return script, scriptVer, err
}

func (src *destinationScriptSourceToAddress) ScriptSize() int {
	return 25 // P2PKHPkScriptSize
}

func main() {
	parseFlags()
	ctx := context.Background()
	err := sweep(ctx)
	if err != nil {
		fatalf("%v", err)
	}
}

func sweep(ctx context.Context) error {
	rpcPassword := opts.RPCPassword

	if rpcPassword == "" {
		secret, err := promptSecret("Wallet RPC password")
		if err != nil {
			return errContext(err, "failed to read RPC password")
		}

		rpcPassword = secret
	}

	// Open RPC client.
	rpcCertificate, err := os.ReadFile(opts.RPCCertificateFile)
	if err != nil {
		return errContext(err, "failed to read RPC certificate")
	}
	caPool := x509.NewCertPool()
	if ok := caPool.AppendCertsFromPEM(rpcCertificate); !ok {
		err := errors.New("unparsable certificate authority")
		return errContext(err, err.Error())
	}
	tc := &tls.Config{RootCAs: caPool, MinVersion: tls.VersionTLS12}
	tlsOpt := wsrpc.WithTLSConfig(tc)

	authOpt := wsrpc.WithBasicAuth(opts.RPCUsername, rpcPassword)

	rpcClient, err := wsrpc.Dial(ctx, opts.RPCConnect, tlsOpt, authOpt)
	if err != nil {
		return errContext(err, "failed to create RPC client")
	}
	defer rpcClient.Close()

	// Fetch all unspent outputs, ignore those not from the source
	// account, and group by their destination address.  Each grouping of
	// outputs will be used as inputs for a single transaction sending to a
	// new destination account address.
	var unspentOutputs []types.ListUnspentResult
	err = rpcClient.Call(ctx, "listunspent", &unspentOutputs)
	if err != nil {
		return errContext(err, "failed to fetch unspent outputs")
	}
	sourceOutputs := make(map[string][]types.ListUnspentResult)
	for _, unspentOutput := range unspentOutputs {
		if !unspentOutput.Spendable {
			continue
		}
		if unspentOutput.Confirmations < opts.RequiredConfirmations {
			continue
		}
		if opts.SourceAccount != "" && opts.SourceAccount != unspentOutput.Account {
			continue
		}
		if opts.SourceAddress != "" && opts.SourceAddress != unspentOutput.Address {
			continue
		}
		sourceAddressOutputs := sourceOutputs[unspentOutput.Address]
		sourceOutputs[unspentOutput.Address] = append(sourceAddressOutputs, unspentOutput)
	}

	for address, outputs := range sourceOutputs {
		outputNoun := pickNoun(len(outputs), "output", "outputs")
		fmt.Printf("Found %d matching unspent %s for address %s\n",
			len(outputs), outputNoun, address)
	}

	var privatePassphrase string
	if len(sourceOutputs) != 0 {
		privatePassphrase, err = promptSecret("Wallet private passphrase")
		if err != nil {
			return errContext(err, "failed to read private passphrase")
		}
	}

	totalSwept := new(big.Int)
	var numErrors int
	// Prefix every operator-visible error with "sweepaccount: " so logs
	// from the broader wallet pipeline can be attributed at a glance —
	// matches the op-prefix convention used by errors.E elsewhere.
	const errOp = "sweepaccount: "
	var reportError = func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, errOp+format, args...)
		os.Stderr.Write(newlineBytes)
		numErrors++
	}

	coinType := cointype.CoinType(opts.CoinType)
	atomsPerCoin := atomsPerCoinFor(activeNet, coinType)
	if atomsPerCoin == nil {
		// parseFlags rejects coin types without a config at startup, so
		// reaching here means parseFlags was bypassed (e.g. a future
		// non-CLI entry point). Panic to signal the contract violation
		// unambiguously rather than rely on a "internal:" string the
		// caller might silently swallow.
		panic(fmt.Sprintf(
			"contract violation: atomsPerCoinFor returned nil for cointype %d after parseFlags",
			opts.CoinType))
	}
	// Use the fee rate already parsed and validated in parseFlags. Avoiding
	// a second parse here removes the risk of drifting from the validated
	// value and the contract-violation error path that came with it.
	if feeRateAtomsParsed == nil {
		return fmt.Errorf("internal: fee rate not parsed at startup")
	}
	feeRate := cointype.NewSKAAmount(new(big.Int).Set(feeRateAtomsParsed))
	for _, previousOutputs := range sourceOutputs {
		inputSource, _ := makeInputSource(previousOutputs, coinType, atomsPerCoin)

		var destinationSourceToAccount *destinationScriptSourceToAccount
		var destinationSourceToAddress *destinationScriptSourceToAddress
		var atx *txauthor.AuthoredTx
		var err error

		if opts.DestinationAccount != "" {
			destinationSourceToAccount = &destinationScriptSourceToAccount{
				accountName: opts.DestinationAccount,
				rpcClient:   rpcClient,
			}
			atx, err = txauthor.NewUnsignedSweepTransaction(coinType, feeRate,
				inputSource, destinationSourceToAccount, activeNet.MaxTxSize)
		}

		if opts.DestinationAddress != "" {
			destinationSourceToAddress = &destinationScriptSourceToAddress{
				address: opts.DestinationAddress,
			}
			atx, err = txauthor.NewUnsignedSweepTransaction(coinType, feeRate,
				inputSource, destinationSourceToAddress, activeNet.MaxTxSize)
		}

		if err != nil {
			if !errors.Is(err, (noInputValue{})) {
				reportError("Failed to create unsigned transaction: %v", err)
			}
			continue
		}

		// Unlock the wallet, sign the transaction, and immediately lock.
		// The walletlock call is deferred so a panic (or any non-local
		// exit) between unlock and lock cannot leave the wallet unlocked
		// for the rest of the 60s passphrase timeout.
		err = rpcClient.Call(ctx, "walletpassphrase", nil, privatePassphrase, 60)
		if err != nil {
			reportError("Failed to unlock wallet: %v", err)
			continue
		}

		var srtResult types.SignRawTransactionResult
		signAndLock := func() error {
			defer func() {
				if lockErr := rpcClient.Call(ctx, "walletlock", nil); lockErr != nil {
					// Bump numErrors so a treasury operator scripting against
					// the binary detects the failure via exit code; a silent
					// stderr line would let an unlocked-wallet window persist
					// undetected for up to 60s.
					reportError("walletlock failed: %v (wallet may stay unlocked for up to 60s)", lockErr)
				}
			}()
			return rpcClient.Call(ctx, "signrawtransaction", &srtResult, atx.Tx)
		}
		err = signAndLock()
		if err != nil {
			reportError("Failed to sign transaction: %v", err)
			continue
		}
		if !srtResult.Complete {
			reportError("Failed to sign every input")
			continue
		}

		// Publish the signed sweep transaction.
		txHash := "DRYRUN"
		if opts.DryRun {
			fmt.Printf("DRY RUN: not actually sending transaction\n")
		} else {
			var hash string
			err := rpcClient.Call(ctx, "sendrawtransaction", &hash, srtResult.Hex, false)
			if err != nil {
				reportError("Failed to publish transaction: %v", err)
				continue
			}

			txHash = hash
		}

		// Report the swept amount as decimal coins (precision-preserving for SKA).
		// Use the post-fee on-chain output value, not the gross input total —
		// a treasury operator scripting against this output otherwise sees a
		// reconciliation gap equal to the relay fee on the destination side.
		swept := sweptOutputValue(atx, coinType)
		fmt.Printf("Swept %s coins (cointype %d) to destination with transaction %v\n",
			cointype.AtomsToDecimalString(swept, atomsPerCoin), opts.CoinType, txHash)
		totalSwept.Add(totalSwept, swept)
	}

	numPublished := len(sourceOutputs) - numErrors
	transactionNoun := pickNoun(numErrors, "transaction", "transactions")
	if numPublished != 0 {
		fmt.Printf("Swept %s coins (cointype %d) to destination across %d %s\n",
			cointype.AtomsToDecimalString(totalSwept, atomsPerCoin), opts.CoinType, numPublished, transactionNoun)
	}
	if numErrors > 0 {
		return fmt.Errorf("failed to publish %d %s", numErrors, transactionNoun)
	}

	return nil
}

// atomsPerCoinFor returns the per-coin atom scale for a given coin type on
// the active network. Returns nil when the requested SKA coin type has no
// configured SKACoinConfig.
func atomsPerCoinFor(params *chaincfg.Params, ct cointype.CoinType) *big.Int {
	if !ct.IsSKA() {
		return big.NewInt(cointype.AtomsPerVAR)
	}
	cfg := params.GetSKACoinConfig(ct)
	if cfg == nil {
		return nil
	}
	return cfg.GetAtomsPerCoin()
}

func promptSecret(what string) (string, error) {
	fmt.Printf("%s: ", what)
	fd := int(os.Stdin.Fd())
	input, err := term.ReadPassword(fd)
	fmt.Println()
	if err != nil {
		return "", err
	}
	return string(input), nil
}

func saneOutputValue(amount dcrutil.Amount) bool {
	return amount >= 0 && amount <= dcrutil.Amount(cointype.MaxVARAmount)
}

// sweptOutputValue returns the on-chain swept amount as atoms by summing the
// transaction's output values for the requested coin type. Sweep txs produced
// by NewUnsignedSweepTransaction have a single change-style output; if a
// future change adds non-change outputs the sum still represents what reached
// the destination. The returned value is post-fee.
func sweptOutputValue(atx *txauthor.AuthoredTx, ct cointype.CoinType) *big.Int {
	swept := new(big.Int)
	if atx == nil || atx.Tx == nil {
		return swept
	}
	for _, out := range atx.Tx.TxOut {
		if ct.IsSKA() {
			if out.SKAValue != nil {
				swept.Add(swept, out.SKAValue)
			}
		} else {
			swept.Add(swept, big.NewInt(out.Value))
		}
	}
	return swept
}

func parseOutPoint(input *types.ListUnspentResult) (wire.OutPoint, error) {
	txHash, err := chainhash.NewHashFromStr(input.TxID)
	if err != nil {
		return wire.OutPoint{}, err
	}
	return wire.OutPoint{Hash: *txHash, Index: input.Vout, Tree: input.Tree}, nil
}

func pickNoun(n int, singularForm, pluralForm string) string {
	if n == 1 {
		return singularForm
	}
	return pluralForm
}
