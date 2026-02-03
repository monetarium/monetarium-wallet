// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"context"

	"github.com/monetarium/monetarium-node/chaincfg/chainhash"
	"github.com/monetarium/monetarium-node/dcrutil"
	"github.com/monetarium/monetarium-node/mixing"
	"github.com/monetarium/monetarium-node/txscript/stdaddr"
	"github.com/monetarium/monetarium-node/wire"
)

// mockNetwork implements all methods of NetworkBackend, returning zero values
// without error.  It may be embedded in a struct to create another
// NetworkBackend which dispatches to particular implementations of the methods.
type mockNetwork struct{}

func (mockNetwork) Blocks(ctx context.Context, blockHashes []*chainhash.Hash) ([]*wire.MsgBlock, error) {
	return nil, nil
}
func (mockNetwork) CFiltersV2(ctx context.Context, blockHashes []*chainhash.Hash) ([]FilterProof, error) {
	return nil, nil
}
func (mockNetwork) PublishTransactions(ctx context.Context, txs ...*wire.MsgTx) error   { return nil }
func (mockNetwork) PublishMixMessages(ctx context.Context, txs ...mixing.Message) error { return nil }
func (mockNetwork) LoadTxFilter(ctx context.Context, reload bool, addrs []stdaddr.Address, outpoints []wire.OutPoint) error {
	return nil
}
func (mockNetwork) Rescan(ctx context.Context, blocks []chainhash.Hash, save func(*chainhash.Hash, []*wire.MsgTx) error) error {
	return nil
}
func (mockNetwork) StakeDifficulty(ctx context.Context) (dcrutil.Amount, error) { return 0, nil }
func (mockNetwork) Synced(ctx context.Context) (bool, int32)                    { return false, 0 }
func (mockNetwork) Done() <-chan struct{}                                       { return nil }
func (mockNetwork) Err() error                                                  { return nil }
func (mockNetwork) GetFeeEstimatesByCoinType(ctx context.Context, coinType uint8) (*FeeEstimates, error) {
	return &FeeEstimates{
		CoinType:             coinType,
		MinRelayFee:          "10000",  // Atoms as string
		DynamicFeeMultiplier: 1.0,
		NormalFee:            "10000",  // Atoms as string
		FastFee:              "20000",  // Atoms as string
		SlowFee:              "5000",   // Atoms as string
	}, nil
}
