package bls

import "fmt"

// DefaultQuorumNumerator is the default quorum percentage numerator (67%).
const DefaultQuorumNumerator = 67

// DefaultQuorumDenominator is the default quorum percentage denominator.
const DefaultQuorumDenominator = 100

// CheckThreshold verifies that signedWeight meets the quorum requirement:
//
//	signedWeight * quorumDen >= totalWeight * quorumNum
//
// This is the exact check performed by avalanchego's warp.VerifyWeight.
// Returns nil when the threshold is met, an ErrInsufficientWeight otherwise.
func CheckThreshold(signedWeight, totalWeight, quorumNum, quorumDen uint64) error {
	if quorumDen == 0 {
		return fmt.Errorf("quorumDen must be non-zero")
	}
	if totalWeight == 0 {
		return fmt.Errorf("totalWeight must be non-zero")
	}
	// Use uint64 arithmetic; multiply before divide to preserve precision.
	// signedWeight * quorumDen >= totalWeight * quorumNum
	lhs := signedWeight * quorumDen
	rhs := totalWeight * quorumNum
	if lhs < rhs {
		pct := float64(signedWeight) / float64(totalWeight) * 100
		need := float64(quorumNum) / float64(quorumDen) * 100
		return fmt.Errorf(
			"insufficient stake weight: collected %.1f%% of total weight, need %.1f%% (%d/%d threshold)",
			pct, need, quorumNum, quorumDen,
		)
	}
	return nil
}

// AccumulateWeight sums the weights at positions marked in signers
// from the provided weights slice (ordered to match the canonical validator set).
func AccumulateWeight(signers []byte, weights []uint64) uint64 {
	var total uint64
	for i, w := range weights {
		if IsSet(signers, i) {
			total += w
		}
	}
	return total
}
