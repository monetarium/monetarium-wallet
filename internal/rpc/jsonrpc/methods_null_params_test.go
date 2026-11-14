// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package jsonrpc

import (
	"encoding/json"
	"testing"

	"github.com/monetarium/monetarium-node/dcrjson"
	"github.com/monetarium/monetarium-wallet/rpc/jsonrpc/types"
)

// TestExplicitNullLeavesOptionalParamsNil pins the parsing rule the handler
// nil-guards rely on: dcrjson fills in defaults for omitted trailing
// parameters only. A parameter the client passes explicitly as JSON null is
// unmarshaled into the command's optional pointer field as nil and never
// defaulted, so a handler that dereferences the field without a nil check
// panics — and, before the websocket handler learned to recover, took the
// whole wallet down with it.
func TestExplicitNullLeavesOptionalParamsNil(t *testing.T) {
	tests := []struct {
		method      string
		params      []json.RawMessage
		checkNil    func(cmd any) bool
		description string
	}{
		{
			"getbalance", []json.RawMessage{json.RawMessage(`"*"`), json.RawMessage(`null`)},
			func(cmd any) bool { return cmd.(*types.GetBalanceCmd).MinConf == nil },
			"getbalance minconf",
		},
		{
			"listtransactions", []json.RawMessage{json.RawMessage(`"*"`), json.RawMessage(`null`), json.RawMessage(`null`)},
			func(cmd any) bool {
				c := cmd.(*types.ListTransactionsCmd)
				return c.Count == nil && c.From == nil
			},
			"listtransactions count and from",
		},
		{
			"sendrawtransaction", []json.RawMessage{json.RawMessage(`"0000"`), json.RawMessage(`null`)},
			func(cmd any) bool { return cmd.(*types.SendRawTransactionCmd).AllowHighFees == nil },
			"sendrawtransaction allowhighfees",
		},
		{
			"signrawtransaction", []json.RawMessage{json.RawMessage(`"0000"`), json.RawMessage(`null`), json.RawMessage(`null`), json.RawMessage(`null`)},
			func(cmd any) bool { return cmd.(*types.SignRawTransactionCmd).Flags == nil },
			"signrawtransaction flags",
		},
	}
	for _, test := range tests {
		cmd, err := dcrjson.ParseParams(types.Method(test.method), test.params)
		if err != nil {
			t.Errorf("%s: ParseParams(%s): %v", test.description, test.params, err)
			continue
		}
		if !test.checkNil(cmd) {
			t.Errorf("%s: explicit null was not kept as a nil optional field; "+
				"a handler dereferencing it without a nil check panics",
				test.description)
		}
	}
}
