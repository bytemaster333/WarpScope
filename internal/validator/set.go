// Package validator manages Avalanche validator sets for Warp message
// verification, including canonical ordering, weight calculation, and
// snapshot diffing between P-Chain heights.
package validator

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/bytemaster333/warpscope/internal/chain"
)

// ValidatorEntry represents a single validator in the canonical Warp set.
// Only validators with non-nil PublicKeyBytes can participate in Warp signing.
type ValidatorEntry struct {
	NodeID         ids.NodeID
	Weight         uint64
	PublicKeyBytes []byte // 48-byte compressed BLS12-381 key
}

// ValidatorSet holds the canonical validator set at a specific P-Chain height.
// Validators are sorted lexicographically by PublicKeyBytes, which determines
// the bit positions in a BitSetSignature.Signers bitset.
type ValidatorSet struct {
	SubnetID    ids.ID
	Height      uint64
	Validators  []*ValidatorEntry // canonical order: sorted by PublicKeyBytes
	TotalWeight uint64
}

// FromValidatorInfos builds a canonical ValidatorSet from the raw P-Chain
// query response. Validators without BLS keys (PublicKeyBytes == nil) are
// excluded because they cannot participate in Warp signing.
//
// The canonical ordering — lexicographic sort by compressed public key bytes —
// must exactly match the ordering used when the BitSetSignature was constructed.
func FromValidatorInfos(subnetID ids.ID, height uint64, infos []chain.ValidatorInfo) (*ValidatorSet, error) {
	entries := make([]*ValidatorEntry, 0, len(infos))
	for _, info := range infos {
		if len(info.PublicKeyBytes) == 0 {
			// Validator has no BLS key; skip silently (pre-BNF legacy validator).
			continue
		}
		if len(info.PublicKeyBytes) != 48 {
			return nil, fmt.Errorf("validator %s: expected 48-byte BLS pubkey, got %d bytes",
				info.NodeID, len(info.PublicKeyBytes))
		}
		entries = append(entries, &ValidatorEntry{
			NodeID:         info.NodeID,
			Weight:         info.Weight,
			PublicKeyBytes: info.PublicKeyBytes,
		})
	}

	// Sort lexicographically by compressed public key bytes.
	sort.Slice(entries, func(i, j int) bool {
		return bytes.Compare(entries[i].PublicKeyBytes, entries[j].PublicKeyBytes) < 0
	})

	var totalWeight uint64
	for _, e := range entries {
		totalWeight += e.Weight
	}

	return &ValidatorSet{
		SubnetID:    subnetID,
		Height:      height,
		Validators:  entries,
		TotalWeight: totalWeight,
	}, nil
}

// Pubkeys returns the compressed BLS public key bytes in canonical order,
// suitable for passing to bls.Verify.
func (vs *ValidatorSet) Pubkeys() [][]byte {
	out := make([][]byte, len(vs.Validators))
	for i, v := range vs.Validators {
		out[i] = v.PublicKeyBytes
	}
	return out
}

// Weights returns the validator weights in canonical order,
// suitable for passing to bls.AccumulateWeight and bls.Verify.
func (vs *ValidatorSet) Weights() []uint64 {
	out := make([]uint64, len(vs.Validators))
	for i, v := range vs.Validators {
		out[i] = v.Weight
	}
	return out
}
