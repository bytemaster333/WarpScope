package validator

import "fmt"

// EpochDuration is the approximate number of P-Chain blocks in a 5-minute
// ACP-181 epoch. Used for heuristic epoch-boundary detection.
// ACP-181 epochs are time-based (5 minutes), not block-count-based, so this
// is a rough estimate assuming ~2-second P-Chain block times.
const EpochDuration = 150 // ~5 minutes at 2s/block

// EpochGracePeriod is the number of P-Chain blocks near an epoch boundary
// where a message signed against the previous epoch's validator set might
// be affected by the transition.
const EpochGracePeriod = 10

// EpochBoundaryResult holds the result of an epoch-boundary proximity check.
type EpochBoundaryResult struct {
	// NearBoundary is true when the signing height is within EpochGracePeriod
	// blocks of a detected epoch transition.
	NearBoundary bool

	// Description is a human-readable explanation if NearBoundary is true.
	Description string
}

// DetectEpochBoundaryRace checks whether the P-Chain height difference between
// the signing height and the current height spans an epoch boundary, which could
// cause the message's validator set to be invalid at the delivery height.
//
// This is a heuristic check for ACP-181 (P-Chain Epoched Views). Full epoch
// verification requires querying the P-Chain for epoch boundary block heights,
// which is deferred to a future implementation.
//
// Returns a non-empty Description when the height delta crosses an epoch
// boundary multiple, suggesting this may be a contributing factor.
func DetectEpochBoundaryRace(signingHeight, currentHeight uint64) EpochBoundaryResult {
	if currentHeight <= signingHeight {
		return EpochBoundaryResult{}
	}

	delta := currentHeight - signingHeight
	if delta < EpochGracePeriod {
		return EpochBoundaryResult{}
	}

	epochsCrossed := delta / EpochDuration
	if epochsCrossed == 0 {
		return EpochBoundaryResult{}
	}

	return EpochBoundaryResult{
		NearBoundary: true,
		Description: fmt.Sprintf(
			"message was signed at P-Chain height %d; current height is %d "+
				"(delta %d blocks, approximately %d epoch(s) elapsed). "+
				"Under ACP-181, validator sets change at epoch boundaries. "+
				"If validators changed between epochs, the original signatures may no longer be valid.",
			signingHeight, currentHeight, delta, epochsCrossed,
		),
	}
}
