package diagnosis

import (
	"fmt"

	"github.com/bytemaster333/warpscope/internal/bls"
	"github.com/bytemaster333/warpscope/internal/chain"
	"github.com/bytemaster333/warpscope/internal/validator"
	warpparse "github.com/bytemaster333/warpscope/internal/warp"
)

// DiagnosisInput bundles all data gathered by the Engine before running checks.
// Individual check functions receive this struct and return a CheckResult.
type DiagnosisInput struct {
	// Source chain data
	SourceTxHash    interface{ Hex() string } // common.Hash, kept as interface to avoid import cycle
	SendEvent       *chain.WarpMessageEvent
	ParsedMsg       *warpparse.ParsedWarpMessage

	// P-Chain data
	ValidatorsAtHeight *validator.ValidatorSet // at PChainHeight (signing height)
	CurrentValidators  *validator.ValidatorSet // at current P-Chain height
	CurrentPChainHeight uint64
	NetworkID          uint32

	// Destination chain data (nil when not found)
	ReceiveEvent   *chain.ReceiveEvent
	ExecFailedEvent *chain.ExecutionFailedEvent

	// Config
	ExpectedNetworkID uint32
	PChainHeight      uint64
	QuorumNum         uint64
	QuorumDen         uint64
}

// checkNetworkID verifies the Warp message's NetworkID matches the expected network.
func checkNetworkID(in *DiagnosisInput) *CheckResult {
	result := &CheckResult{
		Name:     "NetworkID",
		Category: CategoryInvalidNetworkID,
		Severity: SeverityCritical,
	}
	if in.ParsedMsg == nil || in.ParsedMsg.UnsignedMsg == nil {
		result.Description = "No parsed Warp message available; skipping NetworkID check."
		result.Passed = true
		return result
	}
	err := warpparse.ValidateNetworkID(in.ParsedMsg.UnsignedMsg, in.ExpectedNetworkID)
	if err != nil {
		result.Passed = false
		result.Description = err.Error()
		result.Details = map[string]any{
			"messageNetworkID":  in.ParsedMsg.UnsignedMsg.NetworkID,
			"expectedNetworkID": in.ExpectedNetworkID,
		}
		return result
	}
	result.Passed = true
	result.Description = fmt.Sprintf("NetworkID %d matches expected network.", in.ParsedMsg.UnsignedMsg.NetworkID)
	return result
}

// checkRelayerPickup detects whether the message was ever delivered to the
// destination chain. A missing ReceiveCrossChainMessage event is the most
// common silent failure mode.
func checkRelayerPickup(in *DiagnosisInput) *CheckResult {
	result := &CheckResult{
		Name:     "RelayerPickup",
		Category: CategoryRelayerNeverPickedUp,
		Severity: SeverityCritical,
	}
	if in.ReceiveEvent != nil {
		result.Passed = true
		result.Description = fmt.Sprintf(
			"ReceiveCrossChainMessage found at destination block %d (tx %s).",
			in.ReceiveEvent.BlockNumber, in.ReceiveEvent.TxHash.Hex(),
		)
		result.Details = map[string]any{
			"receiveBlockNumber": in.ReceiveEvent.BlockNumber,
			"deliverer":          in.ReceiveEvent.Deliverer.Hex(),
		}
		return result
	}
	result.Passed = false
	result.Description = "No ReceiveCrossChainMessage event found on the destination chain. " +
		"The relayer never picked up this message. Possible causes: relayer not running, " +
		"misconfigured source chain filter, or validator set too fragmented to aggregate 67% stake weight."
	return result
}

// checkDestinationExecution checks whether a successfully delivered message
// failed to execute (MessageExecutionFailed event present).
func checkDestinationExecution(in *DiagnosisInput) *CheckResult {
	result := &CheckResult{
		Name:     "DestinationExecution",
		Category: CategoryDestinationExecutionFailed,
		Severity: SeverityCritical,
	}
	if in.ExecFailedEvent != nil {
		result.Passed = false
		result.Description = fmt.Sprintf(
			"MessageExecutionFailed event found at destination block %d (tx %s). "+
				"The message was delivered but the destination contract reverted. "+
				"Call retryMessageExecution() once the destination contract issue is resolved.",
			in.ExecFailedEvent.BlockNumber, in.ExecFailedEvent.TxHash.Hex(),
		)
		result.Details = map[string]any{
			"failedBlockNumber": in.ExecFailedEvent.BlockNumber,
			"failedTxHash":      in.ExecFailedEvent.TxHash.Hex(),
		}
		return result
	}
	// If no receive event was found, this check is not applicable.
	if in.ReceiveEvent == nil {
		result.Passed = true
		result.Description = "No delivery found; execution failure check not applicable."
		return result
	}
	result.Passed = true
	result.Description = "Message was delivered and executed successfully on destination chain."
	return result
}

// checkStakeWeight checks whether the current validator set can theoretically
// provide the 67% stake weight needed for delivery. This is a forward-looking
// check useful when the message has not yet been delivered.
func checkStakeWeight(in *DiagnosisInput) *CheckResult {
	result := &CheckResult{
		Name:     "StakeWeightThreshold",
		Category: CategoryInsufficientStakeWeight,
		Severity: SeverityWarning,
	}
	if in.ValidatorsAtHeight == nil {
		result.Passed = true
		result.Description = "Validator set not available; skipping stake weight check."
		return result
	}

	// Check if BLS-capable validators (those with pubkeys) can collectively meet
	// the quorum. Total weight of BLS-capable validators vs. total weight.
	var blsWeight uint64
	for _, v := range in.ValidatorsAtHeight.Validators {
		blsWeight += v.Weight
	}

	err := bls.CheckThreshold(blsWeight, in.ValidatorsAtHeight.TotalWeight, in.QuorumNum, in.QuorumDen)
	if err != nil {
		result.Passed = false
		result.Description = fmt.Sprintf(
			"BLS-capable validators hold only %.1f%% of total stake weight; "+
				"the %d/%d quorum threshold cannot be met. "+
				"Some validators may not have registered BLS keys.",
			float64(blsWeight)/float64(in.ValidatorsAtHeight.TotalWeight)*100,
			in.QuorumNum, in.QuorumDen,
		)
		result.Details = map[string]any{
			"blsCapableWeight": blsWeight,
			"totalWeight":      in.ValidatorsAtHeight.TotalWeight,
			"quorumNum":        in.QuorumNum,
			"quorumDen":        in.QuorumDen,
		}
		return result
	}

	result.Passed = true
	result.Description = fmt.Sprintf(
		"BLS-capable validators hold %.1f%% of total stake weight (threshold: %.1f%%).",
		float64(blsWeight)/float64(in.ValidatorsAtHeight.TotalWeight)*100,
		float64(in.QuorumNum)/float64(in.QuorumDen)*100,
	)
	return result
}

// checkValidatorSetChanged detects validators who signed at PChainHeight but
// have since exited the validator set. Their signatures would be worthless at
// the current P-Chain height.
func checkValidatorSetChanged(in *DiagnosisInput) *CheckResult {
	result := &CheckResult{
		Name:     "ValidatorSetStability",
		Category: CategoryValidatorSetChanged,
		Severity: SeverityWarning,
	}
	if in.ValidatorsAtHeight == nil || in.CurrentValidators == nil {
		result.Passed = true
		result.Description = "Validator sets not available; skipping churn check."
		return result
	}
	if in.PChainHeight == in.CurrentPChainHeight {
		result.Passed = true
		result.Description = "P-Chain height unchanged; no validator set churn."
		return result
	}

	diff := validator.Diff(in.ValidatorsAtHeight, in.CurrentValidators)
	if diff.IsEmpty() {
		result.Passed = true
		result.Description = fmt.Sprintf(
			"Validator set unchanged between P-Chain heights %d and %d.",
			in.PChainHeight, in.CurrentPChainHeight,
		)
		return result
	}

	result.Passed = false
	result.Description = fmt.Sprintf(
		"%d validator(s) removed since signing height %d (total removed weight: %d). "+
			"Signatures from removed validators are invalid at current height %d. "+
			"Use retrySendCrossChainMessage to collect fresh signatures.",
		len(diff.Removed), in.PChainHeight, diff.RemovedWeight, in.CurrentPChainHeight,
	)
	result.Details = map[string]any{
		"removedCount":        len(diff.Removed),
		"removedWeight":       diff.RemovedWeight,
		"addedCount":          len(diff.Added),
		"signingHeight":       in.PChainHeight,
		"currentHeight":       in.CurrentPChainHeight,
	}
	return result
}

// checkBLSSignature performs full cryptographic BLS verification of the
// BitSetSignature against the canonical validator set at PChainHeight.
// This check is only run when a signed Warp message is available.
func checkBLSSignature(in *DiagnosisInput) *CheckResult {
	result := &CheckResult{
		Name:     "BLSSignatureValidity",
		Category: CategoryBLSVerificationFailed,
		Severity: SeverityCritical,
	}
	if in.ParsedMsg == nil || in.ParsedMsg.BitSetSig == nil {
		result.Passed = true
		result.Description = "No BitSetSignature available; BLS verification skipped. " +
			"Provide --signed-message-hex or a delivered transaction to enable this check."
		return result
	}
	if in.ValidatorsAtHeight == nil {
		result.Passed = true
		result.Description = "Validator set not available; BLS verification skipped."
		return result
	}

	bitSetSig := in.ParsedMsg.BitSetSig
	vr := bls.Verify(
		in.ParsedMsg.RawMsgBytes,
		bitSetSig.Signers,
		bitSetSig.Signature,
		in.ValidatorsAtHeight.Pubkeys(),
		in.ValidatorsAtHeight.Weights(),
		in.ValidatorsAtHeight.TotalWeight,
		in.QuorumNum,
		in.QuorumDen,
	)

	if !vr.Valid {
		result.Passed = false
		if vr.Error != nil {
			result.Description = vr.Error.Error()
		}
		result.Details = map[string]any{
			"signedWeight": vr.SignedWeight,
			"totalWeight":  vr.TotalWeight,
			"quorumMet":    vr.QuorumMet,
		}
		return result
	}

	result.Passed = true
	result.Description = fmt.Sprintf(
		"BLS aggregate signature is valid. Signed weight: %d / %d (%.1f%%).",
		vr.SignedWeight, vr.TotalWeight,
		float64(vr.SignedWeight)/float64(vr.TotalWeight)*100,
	)
	return result
}

// checkEpochBoundary performs a heuristic check for ACP-181 epoch boundary races.
func checkEpochBoundary(in *DiagnosisInput) *CheckResult {
	result := &CheckResult{
		Name:     "EpochBoundaryRace",
		Category: CategoryEpochBoundaryRace,
		Severity: SeverityWarning,
	}
	if in.PChainHeight == 0 || in.CurrentPChainHeight == 0 {
		result.Passed = true
		result.Description = "P-Chain heights not available; epoch boundary check skipped."
		return result
	}

	// Import validator/epoch heuristic inline to avoid cross-package import.
	delta := in.CurrentPChainHeight - in.PChainHeight
	const epochDuration = 150
	if in.CurrentPChainHeight < in.PChainHeight || delta/epochDuration == 0 {
		result.Passed = true
		result.Description = "No epoch boundary detected between signing height and current height."
		return result
	}

	epochsCrossed := delta / epochDuration
	result.Passed = false
	result.Severity = SeverityWarning
	result.Description = fmt.Sprintf(
		"Approximately %d ACP-181 epoch(s) elapsed since signing height %d (current: %d, delta: %d blocks). "+
			"Validator sets may have changed at epoch boundaries.",
		epochsCrossed, in.PChainHeight, in.CurrentPChainHeight, delta,
	)
	result.Details = map[string]any{
		"signingHeight":  in.PChainHeight,
		"currentHeight":  in.CurrentPChainHeight,
		"epochsCrossed":  epochsCrossed,
	}
	return result
}

