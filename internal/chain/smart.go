package chain

import (
	"context"
	"fmt"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/vms/platformvm"
)

// ResolveSubnetForChainID calls platform.validatedBy to return the Subnet ID
// that validates the given blockchain. Returns ids.Empty (Primary Network)
// if the call fails so callers get a safe fallback.
func ResolveSubnetForChainID(ctx context.Context, pchainRPC string, chainID ids.ID) (ids.ID, error) {
	client := platformvm.NewClient(pchainRPC)
	subnetID, err := client.ValidatedBy(ctx, chainID)
	if err != nil {
		return ids.Empty, fmt.Errorf("platform.validatedBy(%s): %w", chainID, err)
	}
	return subnetID, nil
}

// EstimatePChainHeight estimates the P-Chain block height that corresponds to
// txBlockTime — i.e., the height the relayer would have used as the signing
// height when it aggregated BLS signatures shortly after txBlockTime.
//
// Algorithm:
//  1. Fetch current P-Chain height H and current P-Chain timestamp T via the API.
//  2. Compute Δt = T − txBlockTime.
//  3. Convert Δt to blocks using avgPChainBlockSecs (~3 s on Fuji/Mainnet in practice,
//     but we use a conservative 3 s).
//  4. Return max(1, H − Δblocks).
//
// The result is an estimate; for perfect accuracy the caller can override with
// --pchain-height.
func EstimatePChainHeight(ctx context.Context, pchainRPC string, txBlockTime time.Time) (uint64, error) {
	client := platformvm.NewClient(pchainRPC)

	currentHeight, err := client.GetHeight(ctx)
	if err != nil {
		return 0, fmt.Errorf("platform.getHeight: %w", err)
	}

	currentTS, err := client.GetTimestamp(ctx)
	if err != nil {
		// GetTimestamp can fail on some nodes; fall back to wall-clock.
		currentTS = time.Now().UTC()
	}

	if !txBlockTime.Before(currentTS) {
		// Tx is at or after current P-Chain time (very fresh / clock skew).
		return currentHeight, nil
	}

	delta := currentTS.Sub(txBlockTime)

	// Conservative average: 3 s/block.
	const avgPChainBlockSecs = 3.0
	blockDelta := uint64(delta.Seconds() / avgPChainBlockSecs)

	if blockDelta >= currentHeight {
		return 1, nil
	}
	estimated := currentHeight - blockDelta

	// Never return 0.
	if estimated == 0 {
		estimated = 1
	}
	return estimated, nil
}
