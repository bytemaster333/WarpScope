// Package chain provides interfaces and value types for interacting with
// EVM-compatible chains and the Avalanche P-Chain. All external RPC calls
// go through these interfaces so business logic remains testable.
package chain

import (
	"context"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/libevm/common"
	"github.com/ava-labs/libevm/core/types"
)

// WarpMessageEvent represents a SendWarpMessage event emitted by the Warp
// precompile at 0x0200000000000000000000000000000000000005.
type WarpMessageEvent struct {
	TxHash           common.Hash
	BlockNumber      uint64
	BlockHash        common.Hash
	Sender           common.Address
	// MessageID is Topics[2] — keccak256 of the unsigned message bytes.
	MessageID        common.Hash
	// UnsignedMsgBytes is the raw ABI-decoded bytes from log.Data.
	UnsignedMsgBytes []byte
}

// ReceiveEvent represents a ReceiveCrossChainMessage event emitted by
// TeleporterMessenger at 0x253b2784c75e510dD0fF1da844684a1aC0aa5fcf.
type ReceiveEvent struct {
	TxHash        common.Hash
	BlockNumber   uint64
	MessageID     common.Hash // indexed, from Topics[1]
	SourceChainID common.Hash // indexed, from Topics[2]
	Deliverer     common.Address
}

// ExecutionFailedEvent represents a MessageExecutionFailed event from
// TeleporterMessenger, indicating the destination contract reverted.
type ExecutionFailedEvent struct {
	TxHash        common.Hash
	BlockNumber   uint64
	MessageID     common.Hash // indexed, from Topics[1]
	SourceChainID common.Hash // indexed, from Topics[2]
}

// ValidatorInfo is a lightweight validator representation returned by P-Chain
// queries. Validators with nil PublicKeyBytes have no BLS key registered and
// cannot participate in Warp signing — they are excluded from the canonical set.
type ValidatorInfo struct {
	NodeID         ids.NodeID
	Weight         uint64
	PublicKeyBytes []byte // 48-byte compressed BLS12-381 key; nil if unregistered
}

// ChainConfig holds RPC endpoint configuration for a single chain.
type ChainConfig struct {
	RPCURL   string
	SubnetID ids.ID
}

// EVMClient abstracts EVM chain interactions, enabling the diagnosis engine
// to be tested without live RPC connections.
type EVMClient interface {
	// GetSendWarpEvent returns the SendWarpMessage event from the given source
	// transaction. Returns nil, nil if the transaction contains no Warp event.
	GetSendWarpEvent(ctx context.Context, txHash common.Hash) (*WarpMessageEvent, error)

	// FindReceiveEvent searches [fromBlock, toBlock] on the destination chain
	// for a ReceiveCrossChainMessage event matching messageID.
	// Returns nil, nil if no matching event is found.
	FindReceiveEvent(ctx context.Context, messageID common.Hash, fromBlock, toBlock uint64) (*ReceiveEvent, error)

	// FindExecutionFailedEvent searches for a MessageExecutionFailed event
	// matching messageID within [fromBlock, toBlock].
	// Returns nil, nil if not found.
	FindExecutionFailedEvent(ctx context.Context, messageID common.Hash, fromBlock, toBlock uint64) (*ExecutionFailedEvent, error)

	// BlockNumber returns the current highest block number on this chain.
	BlockNumber(ctx context.Context) (uint64, error)

	// TransactionReceipt returns the receipt for the given transaction hash.
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
}

// PChainClient abstracts Avalanche P-Chain interactions for testability.
type PChainClient interface {
	// GetCurrentValidators returns the active validator set for subnetID.
	GetCurrentValidators(ctx context.Context, subnetID ids.ID) ([]ValidatorInfo, error)

	// GetValidatorsAt returns the validator set for subnetID at a specific
	// P-Chain height. Requires the node to have validator indexing enabled.
	GetValidatorsAt(ctx context.Context, subnetID ids.ID, height uint64) ([]ValidatorInfo, error)

	// GetHeight returns the current P-Chain block height.
	GetHeight(ctx context.Context) (uint64, error)

	// GetNetworkID returns the Avalanche network ID (1=Mainnet, 5=Fuji).
	GetNetworkID(ctx context.Context) (uint32, error)
}
