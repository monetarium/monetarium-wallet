/*
 * Copyright (c) 2016-2020 The Decred developers
 *
 * Permission to use, copy, modify, and distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"github.com/monetarium/monetarium-wallet/rpc/jsonrpc/types"
	"github.com/monetarium/monetarium-wallet/wallet/txauthor"
	"github.com/monetarium/monetarium-wallet/wallet/txsizes"
	"github.com/monetarium/monetarium-node/chaincfg/chainhash"
	"github.com/monetarium/monetarium-node/chaincfg"
	"github.com/monetarium/monetarium-node/cointype"
	"github.com/monetarium/monetarium-node/dcrutil"
	"github.com/monetarium/monetarium-node/txscript/stdaddr"
	"github.com/monetarium/monetarium-node/wire"
)

// shellQuote wraps s as a single-quoted POSIX shell token. Internal single
// quotes are encoded as '\'' (close, escaped quote, reopen).
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// formatSKAValueIn renders a TxIn's SKAValueIn for the helper-script JSON as
// a decimal coin string — the format the wallet-side populateSKAValueIn
// expects. Returns "" for VAR inputs or when SKAValueIn is unset, so the
// signInput JSON's omitempty drops the field cleanly.
func formatSKAValueIn(in *wire.TxIn, isSKA bool, atomsPerCoin *big.Int) string {
	if !isSKA || in.SKAValueIn == nil {
		return ""
	}
	return cointype.AtomsToDecimalString(in.SKAValueIn, atomsPerCoin)
}

// shellQuoteFields splits whitespace-separated tokens and quotes each
// individually so the result remains multiple shell arguments while being
// safe against metacharacter injection from untrusted config.
func shellQuoteFields(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	quoted := make([]string, len(fields))
	for i, f := range fields {
		quoted[i] = shellQuote(f)
	}
	return strings.Join(quoted, " ")
}

// stripRPCPassFields removes any --rpcpass occurrences from the
// whitespace-separated args list. Both --rpcpass=VALUE and the two-token
// "--rpcpass VALUE" form are recognised. Returns the remaining tokens and a
// boolean indicating whether at least one --rpcpass token was stripped, so the
// generated sign.sh can substitute "$MONCTL_RPCPASS" instead of persisting the
// password to disk.
func stripRPCPassFields(s string) ([]string, bool) {
	fields := strings.Fields(s)
	out := make([]string, 0, len(fields))
	stripped := false
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if strings.HasPrefix(f, "--rpcpass=") {
			stripped = true
			continue
		}
		if f == "--rpcpass" {
			stripped = true
			if i+1 < len(fields) {
				i++ // skip the value too
			}
			continue
		}
		out = append(out, f)
	}
	return out, stripped
}

// params is the global representing the chain parameters. It is assigned
// in main.
var params *chaincfg.Params

// configJSON is a configuration file used for transaction generation.
type configJSON struct {
	TxFee         string `json:"txfee"` // decimal coin amount per kB in the selected coin type
	SendToAddress string `json:"sendtoaddress"`
	Network       string `json:"network"`
	MonctlArgs    string `json:"monctlargs"`
	CoinType      uint8  `json:"cointype"` // 0=VAR (default), 1-255=SKA. Outputs of other coin types are skipped.
}

func saneOutputValue(amount dcrutil.Amount) bool {
	return amount >= 0 && amount <= dcrutil.Amount(cointype.MaxVARAmount)
}

func parseOutPoint(input *types.ListUnspentResult) (wire.OutPoint, error) {
	txHash, err := chainhash.NewHashFromStr(input.TxID)
	if err != nil {
		return wire.OutPoint{}, err
	}
	return wire.OutPoint{Hash: *txHash, Index: input.Vout, Tree: input.Tree}, nil
}

// noInputValue describes an error returned by the input source when no inputs
// were selected because each previous output value was zero.  Callers of
// txauthor.NewUnsignedTransaction need not report these errors to the user.
type noInputValue struct {
}

func (noInputValue) Error() string { return "no input value" }

// signPrevout carries the previous-output scripts that the signrawtransaction
// helper needs to reproduce when the operator runs the generated sign.sh
// script against an air-gapped wallet that does not have its own view of the
// chain. Both fields are hex-encoded.
type signPrevout struct {
	PkScript     string
	RedeemScript string
}

// destChangeSource is a txauthor.ChangeSource that returns a fixed
// destination pkScript. movefunds passes this to
// txauthor.NewUnsignedSweepTransaction so that the entire input value
// (less the per-kB-rate fee txauthor computes from the actual tx size)
// lands at cfg.SendToAddress as the sole output.
type destChangeSource struct {
	script  []byte
	version uint16
}

func (d *destChangeSource) Script() ([]byte, uint16, error) { return d.script, d.version, nil }
func (d *destChangeSource) ScriptSize() int                 { return len(d.script) }

// makeInputSource creates an InputSource that creates inputs for every unspent
// output with non-zero output values matching coinType. UTXOs of other coin
// types are skipped with a warning. Returns the total input atoms (big.Int)
// for both VAR and SKA — the caller knows which scale to apply — alongside a
// per-outpoint prevScripts map carrying the pkScript and redeemScript for
// each consumed UTXO so the helper-script generator can emit canonical
// signrawtransaction inputs JSON.
func makeInputSource(outputs []types.ListUnspentResult, coinType cointype.CoinType, atomsPerCoin *big.Int) (*big.Int, txauthor.InputSource, map[wire.OutPoint]signPrevout) {
	isSKA := coinType.IsSKA()
	var (
		totalInputValue   dcrutil.Amount    // VAR
		totalInputSKA     = cointype.Zero() // SKA
		inputs            = make([]*wire.TxIn, 0, len(outputs))
		redeemScriptSizes = make([]int, 0, len(outputs))
		prevScripts       = make(map[wire.OutPoint]signPrevout, len(outputs))
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
		prevScripts[previousOutPoint] = signPrevout{
			PkScript:     output.ScriptPubKey,
			RedeemScript: output.RedeemScript,
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
			"movefunds: warning: skipped %d UTXO(s) not matching cointype %d.\n",
			mismatchedSkipped, coinType)
		// All UTXOs were filtered out — most likely a misconfigured
		// cointype in config.json. Emit a tagged WARN so the operator
		// gets actionable feedback rather than a generic "no input
		// value" failure later in the flow.
		if len(inputs) == 0 {
			fmt.Fprintf(os.Stderr,
				"movefunds: WARN: cointype %d filter excluded all %d UTXO(s) "+
					"in unspent.json; nothing to send.\n",
				coinType, mismatchedSkipped)
		}
	}

	totalAtoms := new(big.Int)
	if isSKA {
		totalAtoms = totalInputSKA.BigInt()
	} else {
		totalAtoms.SetInt64(int64(totalInputValue))
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
	return totalAtoms, src, prevScripts
}

func main() {
	// Create an inputSource from the loaded utxos.
	unspentFile, err := os.Open("unspent.json")
	if err != nil {
		fmt.Printf("error opening unspent file unspent.json: %v", err)
		return
	}
	defer unspentFile.Close()

	var utxos []types.ListUnspentResult

	jsonParser := json.NewDecoder(unspentFile)
	if err = jsonParser.Decode(&utxos); err != nil {
		fmt.Printf("error parsing unspent file: %v", err)
		return
	}

	// Load and parse the movefunds config.
	configFile, err := os.Open("config.json")
	if err != nil {
		fmt.Printf("error opening config file config.json: %v", err)
		return
	}
	defer configFile.Close()

	cfg := new(configJSON)

	jsonParser = json.NewDecoder(configFile)
	if err = jsonParser.Decode(cfg); err != nil {
		fmt.Printf("error parsing config file: %v", err)
		return
	}

	switch cfg.Network {
	case "testnet":
		params = chaincfg.TestNet3Params()
	case "mainnet":
		params = chaincfg.MainNetParams()
	case "simnet":
		params = chaincfg.SimNetParams()
	default:
		fmt.Printf("unknown network specified: %s", cfg.Network)
		return
	}

	coinType := cointype.CoinType(cfg.CoinType)
	atomsPerCoin := atomsPerCoinFor(params, coinType)
	if atomsPerCoin == nil {
		fmt.Printf("no SKA config for cointype %d on %s", cfg.CoinType, cfg.Network)
		return
	}

	_, inputSource, prevScripts := makeInputSource(utxos, coinType, atomsPerCoin)

	addr, err := stdaddr.DecodeAddress(cfg.SendToAddress, params)
	if err != nil {
		fmt.Printf("failed to parse address %s: %v", cfg.SendToAddress, err)
		return
	}

	// Resolve fee atoms from the decimal coin string against the active coin
	// type's atoms-per-coin scale. The string form preserves SKA's 1e18-scale
	// precision that float64 cannot.
	feeAtoms, err := cointype.DecimalStringToAtoms(cfg.TxFee, atomsPerCoin)
	if err != nil {
		fmt.Printf("invalid txfee `%s`: %v", cfg.TxFee, err)
		return
	}
	feeSKA := cointype.NewSKAAmount(feeAtoms)

	// movefunds drains every configured UTXO of the selected coin type to
	// cfg.SendToAddress. cfg.TxFee is the per-kB relay-fee rate (as
	// documented in config.json), not an absolute fee. txauthor computes
	// the on-tx fee from the actual serialised size and emits a single
	// "change" output to cfg.SendToAddress for totalInput - size×rate. The
	// destination thus shows up in the AuthoredTx as the change output
	// (txauthor's terminology) — on chain it's just a plain TxOut to the
	// operator's destination address.
	pkScriptVer, pkScript := addr.PaymentScript()
	changeSrc := &destChangeSource{script: pkScript, version: pkScriptVer}

	atx, err := txauthor.NewUnsignedSweepTransaction(coinType, feeSKA,
		inputSource, changeSrc, params.MaxTxSize)
	if err != nil {
		fmt.Printf("failed to create unsigned transaction: %s", err)
		return
	}

	if atx.Tx.SerializeSize() > params.MaxTxSize {
		fmt.Printf("tx too big: got %v, max %v", atx.Tx.SerializeSize(),
			params.MaxTxSize)
		return
	}

	// Generate the signrawtransaction command.
	txB, err := atx.Tx.Bytes()
	if err != nil {
		fmt.Println("Failed to serialize tx: ", err.Error())
		return
	}

	// Build the signrawtransaction inputs JSON in-process so it is well-
	// formed regardless of script-hash content, then shell-quote both the
	// monetarium-ctl args (split per-token) and the JSON before writing the
	// helper script. This keeps the previous air-gapped workflow (operator
	// runs sign.sh later when the wallet is unlocked) while preventing
	// command injection if cfg.MonctlArgs contains shell metacharacters.
	//
	// Field names match the canonical signrawtransaction RawTxInput shape
	// (scriptPubKey/redeemScript) so the helper works against an air-gapped
	// wallet that requires the prevout scripts to be supplied. Values come
	// from the prevScripts map populated by makeInputSource — i.e. the actual
	// previous-output scripts from listunspent, not the empty SignatureScript
	// of the unsigned tx.
	// SKAValueIn carries the SKA value as a decimal coin string (e.g.
	// "1.234567890123456789") per the project convention of collapsing
	// VAR-or-SKA numeric fields to one decimal string. Populated only for SKA
	// inputs; emitted via omitempty so VAR flows are unchanged. SigHashAll
	// does not bind SKAValueIn (see SECURITY.md "Offline / cold SKA
	// signing"), so an offline signer MUST verify this value against the
	// chain's prevout SKAValue before signing.
	type signInput struct {
		Txid         string `json:"txid"`
		Vout         uint32 `json:"vout"`
		Tree         int8   `json:"tree"`
		ScriptPubKey string `json:"scriptPubKey"`
		RedeemScript string `json:"redeemScript"`
		SKAValueIn   string `json:"skaValueIn,omitempty"`
	}
	inputs := make([]signInput, len(atx.Tx.TxIn))
	for i, in := range atx.Tx.TxIn {
		ps := prevScripts[in.PreviousOutPoint]
		inputs[i] = signInput{
			Txid:         in.PreviousOutPoint.Hash.String(),
			Vout:         in.PreviousOutPoint.Index,
			Tree:         in.PreviousOutPoint.Tree,
			ScriptPubKey: ps.PkScript,
			RedeemScript: ps.RedeemScript,
			SKAValueIn:   formatSKAValueIn(in, coinType.IsSKA(), atomsPerCoin),
		}
	}
	inputsJSON, err := json.Marshal(inputs)
	if err != nil {
		fmt.Println("Failed to marshal sign inputs: ", err.Error())
		return
	}

	// Strip --rpcpass from the persisted command line and read it from
	// $MONCTL_RPCPASS at runtime instead. sign.sh persists across operator
	// runs (mode 0700 doesn't help against backups, ps snooping, or
	// accidental commits to a repo), and the password is the only token
	// in MonctlArgs that's actually a secret.
	monctlFields, hadRPCPass := stripRPCPassFields(cfg.MonctlArgs)
	monctlQuoted := make([]string, len(monctlFields))
	for i, f := range monctlFields {
		monctlQuoted[i] = shellQuote(f)
	}

	var buf bytes.Buffer
	buf.WriteString("#!/bin/sh\n")
	buf.WriteString("set -e\n")
	if hadRPCPass {
		buf.WriteString(": ${MONCTL_RPCPASS:?MONCTL_RPCPASS must be set; export it before running $0}\n")
	}
	buf.WriteString("monetarium-ctl")
	if len(monctlQuoted) > 0 {
		buf.WriteString(" ")
		buf.WriteString(strings.Join(monctlQuoted, " "))
	}
	if hadRPCPass {
		buf.WriteString(` --rpcpass="$MONCTL_RPCPASS"`)
	}
	buf.WriteString(" signrawtransaction ")
	buf.WriteString(shellQuote(hex.EncodeToString(txB)))
	buf.WriteString(" ")
	buf.WriteString(shellQuote(string(inputsJSON)))
	buf.WriteString(" | jq -r .hex\n")

	// Write sign.sh into a freshly-created 0700 tempdir rather than CWD.
	// The signing script contains the unsigned tx and inputs JSON (addresses,
	// amounts, account topology); writing it to CWD on a multi-tenant host
	// could expose those to other users via a world-readable working
	// directory. MkdirTemp creates the dir with mode 0700 by default; the
	// explicit Chmod is belt-and-suspenders against future Go-toolchain
	// changes to that default.
	dir, err := os.MkdirTemp("", "monetarium-movefunds-")
	if err != nil {
		fmt.Println("Failed to create temp dir for signing script: ", err.Error())
		return
	}
	if err := os.Chmod(dir, 0700); err != nil {
		fmt.Println("Failed to chmod temp dir: ", err.Error())
		return
	}
	scriptPath := filepath.Join(dir, "sign.sh")
	if err := os.WriteFile(scriptPath, buf.Bytes(), 0700); err != nil {
		fmt.Println("Failed to write signing script: ", err.Error())
		return
	}

	fmt.Printf("Successfully wrote signing script to %s\n", scriptPath)
	if hadRPCPass {
		fmt.Printf("Run with: MONCTL_RPCPASS='<your-rpc-password>' %s\n", scriptPath)
	}
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
