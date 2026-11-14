package wallet

import (
	"testing"

	"github.com/monetarium/monetarium-node/blockchain/stake"
	"github.com/monetarium/monetarium-node/chaincfg/chainhash"
	"github.com/monetarium/monetarium-node/wire"
)

// TestCompactVoteRecords covers the nil placements of compactVoteRecords:
// no nils, a nil at the start, in the middle and at the end, and all nil.
// Survivors must keep their relative order and stay parallel to their
// ticket hashes and vote bits.
func TestCompactVoteRecords(t *testing.T) {
	const n = 3

	tests := []struct {
		name string
		nils []int // indices of votes to leave nil
	}{
		{name: "none nil"},
		{name: "nil at start", nils: []int{0}},
		{name: "nil in middle", nils: []int{1}},
		{name: "nil at end", nils: []int{2}},
		{name: "all nil", nils: []int{0, 1, 2}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nilAt := func(i int) bool {
				for _, j := range tc.nils {
					if j == i {
						return true
					}
				}
				return false
			}

			// Snapshot the expected survivors before compaction: the
			// compaction reuses and rewrites the slices it is given.
			type expect struct {
				tx   *wire.MsgTx
				hash *chainhash.Hash
				bits stake.VoteBits
			}
			var want []expect

			votes := make([]*wire.MsgTx, n)
			ticketHashes := make([]*chainhash.Hash, n)
			usedVoteBits := make([]stake.VoteBits, n)
			for i := 0; i < n; i++ {
				ticketHashes[i] = &chainhash.Hash{}
				*ticketHashes[i] = chainhash.HashH([]byte{'t', byte(i)})
				usedVoteBits[i] = stake.VoteBits{Bits: uint16(i + 1)}
				if nilAt(i) {
					continue
				}
				vote := wire.NewMsgTx()
				vote.AddTxIn(&wire.TxIn{
					PreviousOutPoint: wire.OutPoint{Hash: *ticketHashes[i]},
				})
				vote.AddTxOut(wire.NewTxOut(1000, []byte{0x51}))
				votes[i] = vote
				want = append(want, expect{votes[i], ticketHashes[i], usedVoteBits[i]})
			}

			gotVotes, gotHashes, gotBits, gotRecords :=
				compactVoteRecords(votes, ticketHashes, usedVoteBits)

			if len(gotVotes) != len(want) || len(gotHashes) != len(want) ||
				len(gotBits) != len(want) || len(gotRecords) != len(want) {
				t.Fatalf("compacted lengths votes=%d hashes=%d bits=%d records=%d, "+
					"want %d survivors", len(gotVotes), len(gotHashes),
					len(gotBits), len(gotRecords), len(want))
			}

			for k, e := range want {
				if gotVotes[k] != e.tx {
					t.Errorf("survivor %d: vote is not the expected transaction", k)
				}
				if *gotHashes[k] != *e.hash {
					t.Errorf("survivor %d: ticket hash %v does not match %v",
						k, gotHashes[k], e.hash)
				}
				if gotBits[k].Bits != e.bits.Bits {
					t.Errorf("survivor %d: vote bits %d do not match %d",
						k, gotBits[k].Bits, e.bits.Bits)
				}
				if gotRecords[k].Hash != gotVotes[k].TxHash() {
					t.Errorf("survivor %d: record hash %v does not match vote hash %v",
						k, gotRecords[k].Hash, gotVotes[k].TxHash())
				}
			}
		})
	}
}
