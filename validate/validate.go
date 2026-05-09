// Copyright (c) 2018 The Decred developers
// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package validate provides context-free consensus validation.
*/
package validate

import (
	"github.com/monetarium/monetarium-wallet/errors"
	blockchain "github.com/monetarium/monetarium-node/blockchain/standalone"
	"github.com/monetarium/monetarium-node/chaincfg/chainhash"
	"github.com/monetarium/monetarium-node/gcs"
	"github.com/monetarium/monetarium-node/wire"
)

// MerkleRoots recreates the merkle roots of regular and stake transactions from
// a block and compares them against the recorded merkle roots in the block
// header.
func MerkleRoots(block *wire.MsgBlock) error {
	const opf = "validate.MerkleRoots(%v)"

	mroot := blockchain.CalcTxTreeMerkleRoot(block.Transactions)
	if block.Header.MerkleRoot != mroot {
		blockHash := block.BlockHash()
		op := errors.Opf(opf, &blockHash)
		return errors.E(op, errors.Consensus, "invalid regular merkle root")
	}
	mroot = blockchain.CalcTxTreeMerkleRoot(block.STransactions)
	if block.Header.StakeRoot != mroot {
		blockHash := block.BlockHash()
		op := errors.Opf(opf, &blockHash)
		return errors.E(op, errors.Consensus, "invalid stake merkle root")
	}

	return nil
}

// DCP0005MerkleRoot recreates the combined regular and stake transaction merkle
// root and compares it against the merkle root in the block header.
//
// DCP0005 (https://github.com/decred/dcps/blob/master/dcp-0005/dcp-0005.mediawiki)
// describes (among other changes) the hard forking change which combined the
// individual regular and stake merkle roots into a single root.
func DCP0005MerkleRoot(block *wire.MsgBlock) error {
	const opf = "validate.DCP0005MerkleRoot(%v)"

	mroot := blockchain.CalcCombinedTxTreeMerkleRoot(block.Transactions, block.STransactions)
	if block.Header.MerkleRoot != mroot {
		blockHash := block.BlockHash()
		op := errors.Opf(opf, &blockHash)
		return errors.E(op, errors.Consensus, "invalid combined merkle root")
	}

	return nil
}

// CFilterV2HeaderCommitment ensures the given v2 committed filter has actually
// been committed to in the header.
//
// Monetarium activates DCP0005-style header commitments from genesis (the
// VoteIDHeaderCommitments agenda is forced active across all networks in
// monetarium-node/chaincfg). There is no pre-activation cfilter window, so
// every header is validated against its StakeRoot inclusion proof — no
// height-based skip is required.
func CFilterV2HeaderCommitment(header *wire.BlockHeader, filter *gcs.FilterV2, leafIndex uint32, proof []chainhash.Hash) error {
	const opf = "validate.CFilterV2HeaderCommitment(%v)"

	// The inclusion proof should verify that the filter hash is included
	// in the stake root of the header (root for header commitments as
	// defined in DCP0005).
	filterHash := filter.Hash()
	root := header.StakeRoot
	if !blockchain.VerifyInclusionProof(&root, &filterHash, leafIndex, proof) {
		blockHash := header.BlockHash()
		op := errors.Opf(opf, &blockHash)
		err := errors.Errorf("invalid header inclusion proof for cfilterv2")
		return errors.E(op, errors.Consensus, err)
	}
	return nil
}
