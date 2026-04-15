// Package bls provides BLS12-381 bitset decoding, threshold math, and
// aggregate signature verification for Avalanche Warp messages.
//
// CGO is required: this package transitively depends on avalanchego's
// utils/crypto/bls which wraps supranational/blst via CGo. Builds must
// set CGO_ENABLED=1.
package bls

// IsSet reports whether bit i is set in the compact bitset signers.
// Bit i maps to byte i/8, bit position i%8 (LSB-first within each byte).
func IsSet(signers []byte, i int) bool {
	byteIdx := i >> 3
	if byteIdx >= len(signers) {
		return false
	}
	return signers[byteIdx]&(1<<uint(i&7)) != 0
}

// SignerIndices returns the sorted slice of validator indices (0-based)
// whose bits are set in the BitSetSignature.Signers bitset.
// validatorCount is the size of the canonical validator set; bits beyond
// that count are ignored.
func SignerIndices(signers []byte, validatorCount int) []int {
	indices := make([]int, 0, validatorCount)
	for i := range validatorCount {
		if IsSet(signers, i) {
			indices = append(indices, i)
		}
	}
	return indices
}

// SignerCount returns the number of set bits in signers up to validatorCount.
func SignerCount(signers []byte, validatorCount int) int {
	return len(SignerIndices(signers, validatorCount))
}
