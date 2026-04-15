package bls

import (
	"fmt"

	avabls "github.com/ava-labs/avalanchego/utils/crypto/bls"
)

// VerificationResult holds the outcome of a BLS threshold verification attempt.
type VerificationResult struct {
	// Valid is true only when both the BLS signature is cryptographically valid
	// and the quorum threshold is met.
	Valid bool

	// SignedWeight is the sum of weights for validators whose bits are set.
	SignedWeight uint64

	// TotalWeight is the sum of all validator weights in the canonical set.
	TotalWeight uint64

	// QuorumMet is true when signedWeight * quorumDen >= totalWeight * quorumNum.
	QuorumMet bool

	// Error describes the first failure encountered (nil on success).
	Error error
}

// Verify performs full BLS threshold verification of a Warp BitSetSignature.
//
// Parameters:
//   - msgBytes: UnsignedMessage.Bytes() — the canonical codec-serialized bytes
//     that validators signed. Do NOT re-hash; blst handles domain separation.
//   - signers: BitSetSignature.Signers — compact bitset mapping canonical
//     validator indices to signature presence.
//   - aggSigBytes: BitSetSignature.Signature — 96-byte aggregate BLS signature.
//   - pubkeys: compressed 48-byte BLS public keys in canonical order
//     (sorted lexicographically by key bytes; nil entries are skipped).
//   - weights: stake weights in the same canonical order as pubkeys.
//   - totalWeight: sum of all weights in the canonical validator set.
//   - quorumNum / quorumDen: threshold fraction (default 67/100).
//
// The canonical ordering (lexicographic sort by pubkey bytes) must match
// exactly how the signing validators encoded their bitset.
func Verify(
	msgBytes []byte,
	signers []byte,
	aggSigBytes [96]byte,
	pubkeys [][]byte,
	weights []uint64,
	totalWeight uint64,
	quorumNum, quorumDen uint64,
) VerificationResult {
	if len(pubkeys) != len(weights) {
		return VerificationResult{
			Error: fmt.Errorf("pubkeys length (%d) != weights length (%d)", len(pubkeys), len(weights)),
		}
	}

	// Parse the aggregate BLS signature.
	aggSig, err := avabls.SignatureFromBytes(aggSigBytes[:])
	if err != nil {
		return VerificationResult{
			Error: fmt.Errorf("parse aggregate signature: %w", err),
		}
	}

	// Collect public keys and accumulate weight for each set bit.
	var pks []*avabls.PublicKey
	signedWeight := AccumulateWeight(signers, weights)
	for i, pkBytes := range pubkeys {
		if !IsSet(signers, i) {
			continue
		}
		if len(pkBytes) == 0 {
			return VerificationResult{
				SignedWeight: signedWeight,
				TotalWeight:  totalWeight,
				Error:        fmt.Errorf("validator at index %d has no BLS public key", i),
			}
		}
		pk, err := avabls.PublicKeyFromCompressedBytes(pkBytes)
		if err != nil {
			return VerificationResult{
				SignedWeight: signedWeight,
				TotalWeight:  totalWeight,
				Error:        fmt.Errorf("validator at index %d: invalid compressed public key: %w", i, err),
			}
		}
		pks = append(pks, pk)
	}

	if len(pks) == 0 {
		return VerificationResult{
			SignedWeight: signedWeight,
			TotalWeight:  totalWeight,
			Error:        fmt.Errorf("no signing validators found in bitset"),
		}
	}

	// Check stake-weight threshold before cryptographic verification to provide
	// a clearer failure category (INSUFFICIENT_STAKE_WEIGHT vs BLS_VERIFICATION_FAILED).
	quorumMet := signedWeight*quorumDen >= totalWeight*quorumNum

	// Aggregate public keys into a single key for batch verification.
	aggPK, err := avabls.AggregatePublicKeys(pks)
	if err != nil {
		return VerificationResult{
			SignedWeight: signedWeight,
			TotalWeight:  totalWeight,
			QuorumMet:    quorumMet,
			Error:        fmt.Errorf("aggregate public keys: %w", err),
		}
	}

	// Verify the aggregate signature against the aggregate public key and message.
	// bls.Verify uses the message bytes directly; blst applies its own DST internally.
	if !avabls.Verify(aggPK, aggSig, msgBytes) {
		return VerificationResult{
			SignedWeight: signedWeight,
			TotalWeight:  totalWeight,
			QuorumMet:    quorumMet,
			Valid:         false,
			Error:         fmt.Errorf("BLS aggregate signature verification failed: invalid signature"),
		}
	}

	return VerificationResult{
		Valid:        quorumMet,
		SignedWeight: signedWeight,
		TotalWeight:  totalWeight,
		QuorumMet:    quorumMet,
	}
}
