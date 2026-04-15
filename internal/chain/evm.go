package chain

import (
	"context"
	"fmt"
	"math/big"

	ethereum "github.com/ava-labs/libevm"
	"github.com/ava-labs/libevm/common"
	"github.com/ava-labs/libevm/core/types"
	"github.com/ava-labs/subnet-evm/ethclient"
	"github.com/bytemaster333/warpscope/internal/warp"
)

// evmClient implements EVMClient using a subnet-evm ethclient.
type evmClient struct {
	client ethclient.Client
}

// NewEVMClient creates an EVMClient backed by a subnet-evm JSON-RPC endpoint.
func NewEVMClient(ctx context.Context, rpcURL string) (EVMClient, error) {
	c, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return nil, fmt.Errorf("dial EVM RPC %s: %w", rpcURL, err)
	}
	return &evmClient{client: c}, nil
}

// GetSendWarpEvent returns the SendWarpMessage event from the given transaction.
// It scans the receipt logs for a log emitted by the Warp precompile with
// the SendWarpMessage topic.
func (c *evmClient) GetSendWarpEvent(ctx context.Context, txHash common.Hash) (*WarpMessageEvent, error) {
	receipt, err := c.client.TransactionReceipt(ctx, txHash)
	if err != nil {
		return nil, fmt.Errorf("get receipt for %s: %w", txHash.Hex(), err)
	}

	for _, log := range receipt.Logs {
		if log == nil {
			continue
		}
		if log.Address != warp.WarpPrecompileAddress {
			continue
		}
		if len(log.Topics) < 3 {
			continue
		}
		if log.Topics[0] != warp.SendWarpMessageTopic {
			continue
		}

		// Topics[1] = sender (address, indexed), Topics[2] = messageID (bytes32, indexed)
		sender := common.BytesToAddress(log.Topics[1].Bytes())
		messageID := log.Topics[2]

		// Decode the unsigned Warp message bytes from log.Data.
		parsed, err := warp.DecodeUnsignedMsgFromLog(log)
		if err != nil {
			return nil, fmt.Errorf("decode Warp message from log in tx %s: %w", txHash.Hex(), err)
		}

		return &WarpMessageEvent{
			TxHash:           txHash,
			BlockNumber:      receipt.BlockNumber.Uint64(),
			BlockHash:        receipt.BlockHash,
			Sender:           sender,
			MessageID:        messageID,
			UnsignedMsgBytes: parsed.RawMsgBytes,
		}, nil
	}

	return nil, nil // no Warp event in this transaction
}

// FindReceiveEvent searches [fromBlock, toBlock] on the destination chain for a
// ReceiveCrossChainMessage event matching messageID.
func (c *evmClient) FindReceiveEvent(
	ctx context.Context,
	messageID common.Hash,
	fromBlock, toBlock uint64,
) (*ReceiveEvent, error) {
	query := ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(fromBlock),
		ToBlock:   new(big.Int).SetUint64(toBlock),
		Addresses: []common.Address{warp.TeleporterAddress},
		Topics: [][]common.Hash{
			{warp.ReceiveCrossChainMessageTopic}, // Topics[0]: event sig
			{messageID},                          // Topics[1]: messageID (indexed)
		},
	}

	logs, err := c.client.FilterLogs(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("filter ReceiveCrossChainMessage logs: %w", err)
	}

	for _, log := range logs {
		if len(log.Topics) < 4 {
			continue
		}
		// Topics[1]=messageID, Topics[2]=sourceBlockchainID, Topics[3]=deliverer
		return &ReceiveEvent{
			TxHash:        log.TxHash,
			BlockNumber:   log.BlockNumber,
			MessageID:     log.Topics[1],
			SourceChainID: log.Topics[2],
			Deliverer:     common.BytesToAddress(log.Topics[3].Bytes()),
		}, nil
	}

	return nil, nil // not found
}

// FindExecutionFailedEvent searches [fromBlock, toBlock] for a
// MessageExecutionFailed event matching messageID.
func (c *evmClient) FindExecutionFailedEvent(
	ctx context.Context,
	messageID common.Hash,
	fromBlock, toBlock uint64,
) (*ExecutionFailedEvent, error) {
	query := ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(fromBlock),
		ToBlock:   new(big.Int).SetUint64(toBlock),
		Addresses: []common.Address{warp.TeleporterAddress},
		Topics: [][]common.Hash{
			{warp.MessageExecutionFailedTopic}, // Topics[0]: event sig
			{messageID},                        // Topics[1]: messageID (indexed)
		},
	}

	logs, err := c.client.FilterLogs(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("filter MessageExecutionFailed logs: %w", err)
	}

	for _, log := range logs {
		if len(log.Topics) < 3 {
			continue
		}
		return &ExecutionFailedEvent{
			TxHash:        log.TxHash,
			BlockNumber:   log.BlockNumber,
			MessageID:     log.Topics[1],
			SourceChainID: log.Topics[2],
		}, nil
	}

	return nil, nil // not found
}

// BlockNumber returns the current highest block on this chain.
func (c *evmClient) BlockNumber(ctx context.Context) (uint64, error) {
	n, err := c.client.BlockNumber(ctx)
	if err != nil {
		return 0, fmt.Errorf("get block number: %w", err)
	}
	return n, nil
}

// TransactionReceipt returns the receipt for the given transaction hash.
func (c *evmClient) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	r, err := c.client.TransactionReceipt(ctx, txHash)
	if err != nil {
		return nil, fmt.Errorf("get transaction receipt %s: %w", txHash.Hex(), err)
	}
	return r, nil
}
