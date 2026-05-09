// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/monetarium/monetarium-node/chaincfg"
	"github.com/monetarium/monetarium-node/dcrec"
	"github.com/monetarium/monetarium-node/dcrec/secp256k1"
	"github.com/monetarium/monetarium-node/dcrutil"
)

func generateKeys(params *chaincfg.Params, outputPath string, unsafePrint bool) error {
	key, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		return err
	}

	keyBytes := key.Serialize()
	wif, err := dcrutil.NewWIF(keyBytes, params.PrivateKeyID,
		dcrec.STSchnorrSecp256k1)
	if err != nil {
		return err
	}

	pubKey := fmt.Sprintf("%x", key.PubKey().SerializeCompressed())

	if outputPath != "" {
		f, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer f.Close()
		if _, err := fmt.Fprintf(f, "Private key: %x\nPublic  key: %s\nWIF        : %s\n",
			keyBytes, pubKey, wif); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		fmt.Fprintf(os.Stderr,
			"Private key + WIF written to %s (mode 0600).\n"+
				"Public key: %s\n"+
				"WARNING: keep this file offline; delete it after importing.\n",
			outputPath, pubKey)
		return nil
	}

	if !unsafePrint {
		return fmt.Errorf(
			"refusing to print private key to stdout; pass --output <file> " +
				"to write a 0600 keyfile, or --unsafe-print to override " +
				"(scrollback / multiplexer logging exposes the key)")
	}

	fmt.Fprintln(os.Stderr,
		"WARNING: printing private key to stdout. Clear terminal scrollback after use.")
	fmt.Printf("Private key: %x\n", keyBytes)
	fmt.Printf("Public  key: %s\n", pubKey)
	fmt.Printf("WIF        : %s\n", wif)

	return nil
}

func main() {
	mainnet := flag.Bool("mainnet", false, "use mainnet parameters")
	simnet := flag.Bool("simnet", false, "use simnet parameters")
	regnet := flag.Bool("regnet", false, "use regnet parameters")
	testnet := flag.Bool("testnet", false, "use testnet parameters")
	output := flag.String("output", "",
		"write the generated key material to this file (mode 0600); "+
			"file must not already exist")
	unsafePrint := flag.Bool("unsafe-print", false,
		"print the private key to stdout (exposes the key to terminal "+
			"scrollback / multiplexer logging); prefer --output")
	flag.Parse()

	var net *chaincfg.Params
	flags := 0
	if *mainnet {
		flags++
		net = chaincfg.MainNetParams()
	}
	if *testnet {
		flags++
		net = chaincfg.TestNet3Params()
	}
	if *simnet {
		flags++
		net = chaincfg.SimNetParams()
	}
	if *regnet {
		flags++
		net = chaincfg.RegNetParams()
	}
	if flags != 1 {
		fmt.Println("One and only one network flag must be selected")
		flag.Usage()
		os.Exit(1)
	}

	if err := generateKeys(net, *output, *unsafePrint); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
