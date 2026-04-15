package diagnosis

import (
	"context"
	"fmt"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/libevm/common"
	"github.com/bytemaster333/warpscope/internal/chain"
	"github.com/bytemaster333/warpscope/internal/validator"
	warpparse "github.com/bytemaster333/warpscope/internal/warp"
)

// DefaultSearchWindow is the number of destination chain blocks to search
// for Teleporter events when no explicit range is given.
const DefaultSearchWindow = 50_000

// Engine orchestrates the WarpScope failure-attribution pipeline.
//
// Usage:
//
//	engine := &Engine{
//	    SourceClient: ..., DestClient: ..., PChain: ...,
//	    SubnetID: ..., ExpectedNetworkID: 1,
//	    QuorumNum: 67, QuorumDen: 100,
//	}
//	report, err := engine.Diagnose(ctx, sourceTxHash)
type Engine struct {
	// SourceClient is the EVM client for the source chain.
	SourceClient chain.EVMClient

	// DestClient is the EVM client for the destination chain.
	// May be nil if destination chain checking is not desired.
	DestClient chain.EVMClient

	// PChain is the P-Chain RPC client.
	PChain chain.PChainClient

	// SubnetID is the subnet whose validator set to query.
	// Use ids.Empty for the Primary Network.
	SubnetID ids.ID

	// ExpectedNetworkID is the Avalanche network ID (1=Mainnet, 5=Fuji).
	ExpectedNetworkID uint32

	// PChainHeight is the P-Chain height to use for the signing-time validator set.
	// When 0, the engine uses the current P-Chain height.
	PChainHeight uint64

	// QuorumNum and QuorumDen define the stake-weight threshold (default 67/100).
	QuorumNum uint64
	QuorumDen uint64

	// DestFromBlock and DestToBlock bound the log search on the destination chain.
	// When both are 0, the engine searches the last DefaultSearchWindow blocks.
	DestFromBlock uint64
	DestToBlock   uint64
}

// Diagnose runs the full WarpScope diagnostic pipeline for the given source
// transaction hash and returns a populated DiagnosticReport.
//
// The pipeline:
//  1. Fetch the SendWarpMessage event from the source chain.
//  2. Decode the UnsignedMessage from the event.
//  3. Query validator sets (at PChainHeight and current).
//  4. Search the destination chain for delivery and execution events.
//  5. Run all diagnostic checks in priority order.
//  6. Assemble and return the DiagnosticReport.
func (e *Engine) Diagnose(ctx context.Context, sourceTxHash common.Hash) (*DiagnosticReport, error) {
	if e.QuorumNum == 0 {
		e.QuorumNum = 67
	}
	if e.QuorumDen == 0 {
		e.QuorumDen = 100
	}

	report := &DiagnosticReport{
		TxHash:       sourceTxHash,
		Timestamp:    time.Now().UTC(),
		PChainHeight: e.PChainHeight,
	}

	// ── Step 1: Fetch the SendWarpMessage event ──────────────────────────────
	sendEvent, err := e.SourceClient.GetSendWarpEvent(ctx, sourceTxHash)
	if err != nil {
		return nil, fmt.Errorf("get SendWarpMessage event: %w", err)
	}
	if sendEvent == nil {
		// No Warp event found — wrong tx hash or not a Warp transaction.
		report.RootCause = CategoryRelayerNeverPickedUp
		report.IsHealthy = false
		report.Summary = fmt.Sprintf(
			"No SendWarpMessage event found in transaction %s. "+
				"Ensure the transaction hash is correct and the transaction is on the source chain.",
			sourceTxHash.Hex(),
		)
		return report, nil
	}
	report.MessageID = sendEvent.MessageID

	// ── Step 2: Decode the UnsignedMessage ───────────────────────────────────
	// The UnsignedMsgBytes were already ABI-decoded from the log by GetSendWarpEvent.
	// Parse errors are non-fatal: we continue with a partial ParsedWarpMessage so
	// structural checks (NetworkID, SourceChainID) are skipped but RPC-based checks
	// (delivery detection, validator set) still run.
	parsedMsg, parseErr := warpparse.ParseUnsignedMessageBytes(sendEvent.UnsignedMsgBytes)
	if parseErr != nil || parsedMsg == nil {
		parsedMsg = &warpparse.ParsedWarpMessage{} // empty, nil UnsignedMsg
	}

	if parsedMsg.UnsignedMsg != nil {
		report.NetworkID = parsedMsg.UnsignedMsg.NetworkID
		report.SourceChainID = parsedMsg.UnsignedMsg.SourceChainID
	}

	// ── Step 3: Query P-Chain ─────────────────────────────────────────────────
	networkID, _ := e.PChain.GetNetworkID(ctx)
	currentPChainHeight, _ := e.PChain.GetHeight(ctx)

	pchainHeight := e.PChainHeight
	if pchainHeight == 0 {
		pchainHeight = currentPChainHeight
	}
	report.PChainHeight = pchainHeight

	var validatorsAtHeight, currentValidators *validator.ValidatorSet

	if pchainHeight > 0 {
		infos, err := e.PChain.GetValidatorsAt(ctx, e.SubnetID, pchainHeight)
		if err == nil {
			validatorsAtHeight, _ = validator.FromValidatorInfos(e.SubnetID, pchainHeight, infos)
		}
	}

	if currentPChainHeight > 0 && currentPChainHeight != pchainHeight {
		infos, err := e.PChain.GetCurrentValidators(ctx, e.SubnetID)
		if err == nil {
			currentValidators, _ = validator.FromValidatorInfos(e.SubnetID, currentPChainHeight, infos)
		}
	} else {
		currentValidators = validatorsAtHeight
	}

	// ── Step 4: Search destination chain ─────────────────────────────────────
	var receiveEvent *chain.ReceiveEvent
	var execFailedEvent *chain.ExecutionFailedEvent

	if e.DestClient != nil {
		fromBlock, toBlock, err := e.resolveDestBlocks(ctx)
		if err == nil {
			receiveEvent, _ = e.DestClient.FindReceiveEvent(ctx, sendEvent.MessageID, fromBlock, toBlock)
			if receiveEvent != nil {
				execFailedEvent, _ = e.DestClient.FindExecutionFailedEvent(ctx, sendEvent.MessageID, fromBlock, toBlock)
			}
		}
	}

	// ── Step 5: Run checks ────────────────────────────────────────────────────
	in := &DiagnosisInput{
		SourceTxHash:        sourceTxHash,
		SendEvent:           sendEvent,
		ParsedMsg:           parsedMsg,
		ValidatorsAtHeight:  validatorsAtHeight,
		CurrentValidators:   currentValidators,
		CurrentPChainHeight: currentPChainHeight,
		NetworkID:           networkID,
		ReceiveEvent:        receiveEvent,
		ExecFailedEvent:     execFailedEvent,
		ExpectedNetworkID:   e.ExpectedNetworkID,
		PChainHeight:        pchainHeight,
		QuorumNum:           e.QuorumNum,
		QuorumDen:           e.QuorumDen,
	}

	checks := runChecks(in)
	report.Checks = checks

	// ── Step 6: Determine root cause ──────────────────────────────────────────
	report.RootCause = CategoryHealthy
	report.IsHealthy = true
	for _, c := range checks {
		if !c.Passed && c.Severity == SeverityCritical {
			report.RootCause = c.Category
			report.IsHealthy = false
			break
		}
	}
	// If no critical failures but warnings exist, report is still "healthy" delivery-wise.
	report.Summary = buildSummary(report)
	return report, nil
}

// runChecks executes all diagnostic checks in priority order and returns results.
func runChecks(in *DiagnosisInput) []*CheckResult {
	return []*CheckResult{
		checkNetworkID(in),
		checkRelayerPickup(in),
		checkDestinationExecution(in),
		checkStakeWeight(in),
		checkValidatorSetChanged(in),
		checkBLSSignature(in),
		checkEpochBoundary(in),
	}
}

// resolveDestBlocks determines the block range to search on the destination chain.
func (e *Engine) resolveDestBlocks(ctx context.Context) (fromBlock, toBlock uint64, err error) {
	if e.DestToBlock > 0 {
		return e.DestFromBlock, e.DestToBlock, nil
	}
	current, err := e.DestClient.BlockNumber(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("get destination block number: %w", err)
	}
	if current > DefaultSearchWindow {
		return current - DefaultSearchWindow, current, nil
	}
	return 0, current, nil
}

