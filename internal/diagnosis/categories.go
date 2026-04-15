// Package bls — diagnosis/categories lives in a separate file; this file
// contains the failure-category constants used throughout WarpScope.
package diagnosis

// FailureCategory is a machine-readable failure classification string.
type FailureCategory string

const (
	// CategoryHealthy indicates all checks passed; message was delivered successfully.
	CategoryHealthy FailureCategory = "HEALTHY"

	// CategoryRelayerNeverPickedUp: SendWarpMessage exists on source but no
	// ReceiveCrossChainMessage on destination and no relayer activity detected.
	// This is the most common silent failure — currently invisible to all tools.
	CategoryRelayerNeverPickedUp FailureCategory = "RELAYER_NEVER_PICKED_UP"

	// CategoryInsufficientStakeWeight: aggregate BLS stake weight of signing
	// validators is below the quorum threshold (default 67/100).
	CategoryInsufficientStakeWeight FailureCategory = "INSUFFICIENT_STAKE_WEIGHT"

	// CategoryValidatorSetChanged: validators who signed the message have since
	// exited the validator set; their weight can no longer be counted.
	CategoryValidatorSetChanged FailureCategory = "VALIDATOR_SET_CHANGED"

	// CategoryBLSVerificationFailed: the aggregate BLS signature is
	// cryptographically invalid (bls.Verify returned false).
	CategoryBLSVerificationFailed FailureCategory = "BLS_VERIFICATION_FAILED"

	// CategoryInvalidSourceChainID: UnsignedMessage.SourceChainID does not match
	// the blockchain ID of the chain where the SendWarpMessage event was emitted.
	CategoryInvalidSourceChainID FailureCategory = "INVALID_SOURCE_CHAIN_ID"

	// CategoryInvalidNetworkID: UnsignedMessage.NetworkID does not match the
	// expected Avalanche network (1=Mainnet, 5=Fuji). Cross-network replay attempt.
	CategoryInvalidNetworkID FailureCategory = "INVALID_NETWORK_ID"

	// CategoryDestinationExecutionFailed: the message was delivered (Warp sig valid)
	// but the TeleporterMessenger.receiveTeleporterMessage call reverted on the
	// destination contract. Indicated by the MessageExecutionFailed event.
	CategoryDestinationExecutionFailed FailureCategory = "DESTINATION_EXECUTION_FAILED"

	// CategoryQuorumConfigMismatch: destination chain's Warp precompile is
	// configured with a quorumNumerator different from what the relayer used.
	CategoryQuorumConfigMismatch FailureCategory = "QUORUM_CONFIG_MISMATCH"

	// CategoryEpochBoundaryRace: message was signed against epoch N's validator
	// set but delivery occurred during epoch N+1 with a different P-Chain height
	// (ACP-181 epoch boundary transition issue).
	CategoryEpochBoundaryRace FailureCategory = "EPOCH_BOUNDARY_RACE"

	// CategoryUnknown is used when no specific failure mode is identified.
	CategoryUnknown FailureCategory = "UNKNOWN"
)

// Severity classifies how critical a check failure is.
type Severity string

const (
	SeverityCritical Severity = "CRITICAL"
	SeverityWarning  Severity = "WARNING"
	SeverityInfo     Severity = "INFO"
)
