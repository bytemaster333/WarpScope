package validator

import (
	"github.com/ava-labs/avalanchego/ids"
)

// DiffResult describes how the validator set changed between two P-Chain heights.
type DiffResult struct {
	// Added contains validators present in `current` but absent at `atSigning`.
	Added []*ValidatorEntry

	// Removed contains validators present at `atSigning` but absent in `current`.
	// These are the dangerous ones: signatures from removed validators are worthless
	// at the current height.
	Removed []*ValidatorEntry

	// RemovedWeight is the total stake weight of removed validators.
	// If RemovedWeight causes the effective signed weight to drop below the quorum
	// threshold, the message cannot be delivered without re-signing.
	RemovedWeight uint64

	// Changed contains validators present in both sets but with different weights.
	Changed []WeightChange
}

// WeightChange records a validator whose weight changed between heights.
type WeightChange struct {
	NodeID        ids.NodeID
	OldWeight     uint64
	NewWeight     uint64
}

// IsEmpty returns true when no validators were added, removed, or changed.
func (d *DiffResult) IsEmpty() bool {
	return len(d.Added) == 0 && len(d.Removed) == 0 && len(d.Changed) == 0
}

// Diff computes the validator-set difference between atSigning (the set used
// when the Warp message was signed) and current (the set today).
//
// Removed validators are the primary concern: any weight they contributed to
// the original BitSetSignature can no longer be counted at the current height.
func Diff(atSigning, current *ValidatorSet) DiffResult {
	signingMap := make(map[ids.NodeID]*ValidatorEntry, len(atSigning.Validators))
	for _, v := range atSigning.Validators {
		signingMap[v.NodeID] = v
	}

	currentMap := make(map[ids.NodeID]*ValidatorEntry, len(current.Validators))
	for _, v := range current.Validators {
		currentMap[v.NodeID] = v
	}

	var result DiffResult

	// Find removed and changed.
	for nodeID, sv := range signingMap {
		cv, inCurrent := currentMap[nodeID]
		if !inCurrent {
			result.Removed = append(result.Removed, sv)
			result.RemovedWeight += sv.Weight
		} else if cv.Weight != sv.Weight {
			result.Changed = append(result.Changed, WeightChange{
				NodeID:    nodeID,
				OldWeight: sv.Weight,
				NewWeight: cv.Weight,
			})
		}
	}

	// Find added.
	for nodeID, cv := range currentMap {
		if _, inSigning := signingMap[nodeID]; !inSigning {
			result.Added = append(result.Added, cv)
		}
	}

	return result
}
