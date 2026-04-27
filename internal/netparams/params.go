// Copyright (c) 2013-2015 The btcsuite developers
// Copyright (c) 2016-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package netparams

import "github.com/monetarium/monetarium-node/chaincfg"

// Params is used to group parameters for various networks such as the main
// network and test networks.
type Params struct {
	*chaincfg.Params
	JSONRPCClientPort string
	JSONRPCServerPort string
	GRPCServerPort    string
}

// MainNetParams contains parameters specific to running monetarium-wallet and
// monetarium-node on the main network (wire.MainNet).
var MainNetParams = Params{
	Params:            chaincfg.MainNetParams(),
	JSONRPCClientPort: "9509",
	JSONRPCServerPort: "9510",
	GRPCServerPort:    "9511",
}

// TestNet3Params contains parameters specific to running monetarium-wallet and
// monetarium-node on the test network (version 3) (wire.TestNet3).
var TestNet3Params = Params{
	Params:            chaincfg.TestNet3Params(),
	JSONRPCClientPort: "19509",
	JSONRPCServerPort: "19510",
	GRPCServerPort:    "19511",
}

// SimNetParams contains parameters specific to the simulation test network
// (wire.SimNet).
var SimNetParams = Params{
	Params:            chaincfg.SimNetParams(),
	JSONRPCClientPort: "19956",
	JSONRPCServerPort: "19957",
	GRPCServerPort:    "19958",
}
