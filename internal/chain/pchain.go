package chain

import (
	"context"
	"fmt"
	"strings"

	"github.com/ava-labs/avalanchego/api/info"
	"github.com/ava-labs/avalanchego/ids"
	avabls "github.com/ava-labs/avalanchego/utils/crypto/bls"
	"github.com/ava-labs/avalanchego/vms/platformvm"
	platformapi "github.com/ava-labs/avalanchego/vms/platformvm/api"
	"github.com/ava-labs/avalanchego/vms/platformvm/signer"
)

// pchainClient implements PChainClient using avalanchego's platformvm and info
// JSON-RPC clients.
type pchainClient struct {
	platform *platformvm.Client
	info     *info.Client
}

// NewPChainClient creates a PChainClient connected to the given P-Chain node URI.
// The URI should be the base URL of an AvalancheGo node, e.g.:
//
//	https://api.avax.network  (Mainnet)
//	https://api.avax-test.network  (Fuji)
func NewPChainClient(uri string) PChainClient {
	return &pchainClient{
		platform: platformvm.NewClient(uri),
		info:     info.NewClient(uri),
	}
}

// GetCurrentValidators returns all active validators for the given subnet,
// extracting their BLS public keys from the Signer (ProofOfPossession) field.
// Validators without a registered BLS key get nil PublicKeyBytes and are later
// excluded from the canonical Warp validator set.
func (c *pchainClient) GetCurrentValidators(ctx context.Context, subnetID ids.ID) ([]ValidatorInfo, error) {
	permissionless, err := c.platform.GetCurrentValidators(ctx, subnetID, nil)
	if err != nil {
		return nil, fmt.Errorf("platform.getCurrentValidators for subnet %s: %w", subnetID, err)
	}

	infos := make([]ValidatorInfo, 0, len(permissionless))
	for _, v := range permissionless {
		vi := ValidatorInfo{
			NodeID: v.NodeID,
			Weight: v.Weight,
		}
		if v.Signer != nil {
			vi.PublicKeyBytes = extractBLSKeyBytes(v.Signer)
		}
		infos = append(infos, vi)
	}
	return infos, nil
}

// GetValidatorsAt returns the validator set for a subnet at a specific P-Chain
// height. This requires the node to have validator indexing enabled
// (--index-enabled=true). Returns a descriptive error if indexing is disabled.
func (c *pchainClient) GetValidatorsAt(ctx context.Context, subnetID ids.ID, height uint64) ([]ValidatorInfo, error) {
	validatorMap, err := c.platform.GetValidatorsAt(ctx, subnetID, platformapi.Height(height))
	if err != nil {
		if isIndexingDisabledError(err) {
			return nil, fmt.Errorf(
				"the P-Chain node at does not support getValidatorsAt — "+
					"validator indexing is disabled. Use a node with --index-enabled=true "+
					"or the public API endpoint (api.avax.network / api.avax-test.network): %w", err)
		}
		return nil, fmt.Errorf("platform.getValidatorsAt subnet=%s height=%d: %w", subnetID, height, err)
	}

	infos := make([]ValidatorInfo, 0, len(validatorMap))
	for nodeID, v := range validatorMap {
		vi := ValidatorInfo{
			NodeID: nodeID,
			Weight: v.Weight,
		}
		if v.PublicKey != nil {
			// bls.PublicKey is a type alias for blst.P1Affine; call Compress() directly.
			vi.PublicKeyBytes = v.PublicKey.Compress()
		}
		infos = append(infos, vi)
	}
	return infos, nil
}

// GetHeight returns the current P-Chain block height.
func (c *pchainClient) GetHeight(ctx context.Context) (uint64, error) {
	h, err := c.platform.GetHeight(ctx)
	if err != nil {
		return 0, fmt.Errorf("platform.getHeight: %w", err)
	}
	return h, nil
}

// GetNetworkID returns the Avalanche network ID from the connected node's info API.
// 1 = Mainnet, 5 = Fuji.
func (c *pchainClient) GetNetworkID(ctx context.Context) (uint32, error) {
	id, err := c.info.GetNetworkID(ctx)
	if err != nil {
		return 0, fmt.Errorf("info.getNetworkID: %w", err)
	}
	return id, nil
}

// extractBLSKeyBytes returns the 48-byte compressed BLS public key from a
// ProofOfPossession (as returned by platform.getCurrentValidators).
// The PublicKey field is [signer.PublicKeyLen]byte = [48]byte.
func extractBLSKeyBytes(pop *signer.ProofOfPossession) []byte {
	if pop == nil {
		return nil
	}
	// Validate the PoP before returning the key bytes.
	// If this fails (malformed key), return nil so the validator is excluded.
	pk, err := avabls.PublicKeyFromCompressedBytes(pop.PublicKey[:])
	if err != nil {
		return nil
	}
	_ = pk // key is valid; return the raw bytes
	return pop.PublicKey[:]
}

// isIndexingDisabledError checks whether the error message indicates that
// validator indexing is not enabled on the node.
func isIndexingDisabledError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "index") ||
		strings.Contains(msg, "not enabled") ||
		strings.Contains(msg, "API not enabled")
}
